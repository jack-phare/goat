package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
)

// CheckpointManifest records which files were snapshotted at a checkpoint.
type CheckpointManifest struct {
	UserMessageUUID string         `json:"user_message_uuid"`
	CreatedAt       time.Time      `json:"created_at"`
	Files           []FileSnapshot `json:"files"`
}

// FileSnapshot records the state of a single file at checkpoint time.
type FileSnapshot struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Hash   string `json:"hash,omitempty"`
	Size   int    `json:"size,omitempty"`
}

// CheckpointManager handles file snapshotting and rewind for a session.
type CheckpointManager struct {
	sessionDir string
}

func newCheckpointManager(sessionDir string) *CheckpointManager {
	return &CheckpointManager{sessionDir: sessionDir}
}

func (cm *CheckpointManager) checkpointsDir() string {
	return filepath.Join(cm.sessionDir, "checkpoints")
}

func (cm *CheckpointManager) checkpointDir(userMsgUUID string) string {
	return filepath.Join(cm.checkpointsDir(), userMsgUUID)
}

func (cm *CheckpointManager) filesDir(userMsgUUID string) string {
	return filepath.Join(cm.checkpointDir(userMsgUUID), "files")
}

func (cm *CheckpointManager) manifestPath(userMsgUUID string) string {
	return filepath.Join(cm.checkpointDir(userMsgUUID), "manifest.json")
}

// CreateCheckpoint snapshots the given files, storing content-addressed copies.
func (cm *CheckpointManager) CreateCheckpoint(userMsgUUID string, filePaths []string) error {
	filesDir := cm.filesDir(userMsgUUID)
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return fmt.Errorf("create checkpoint dir: %w", err)
	}

	manifest := CheckpointManifest{
		UserMessageUUID: userMsgUUID,
		CreatedAt:       time.Now(),
	}

	for _, path := range filePaths {
		snapshot := FileSnapshot{Path: path}

		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				snapshot.Exists = false
				manifest.Files = append(manifest.Files, snapshot)
				continue
			}
			return fmt.Errorf("read file %q: %w", path, err)
		}

		snapshot.Exists = true
		snapshot.Size = len(data)

		// Content-addressed storage via SHA256
		hash := sha256.Sum256(data)
		snapshot.Hash = hex.EncodeToString(hash[:])

		hashPath := filepath.Join(filesDir, snapshot.Hash)
		// Only write if not already present (deduplication)
		if _, err := os.Stat(hashPath); os.IsNotExist(err) {
			if err := os.WriteFile(hashPath, data, 0644); err != nil {
				return fmt.Errorf("write checkpoint file %q: %w", path, err)
			}
		}

		manifest.Files = append(manifest.Files, snapshot)
	}

	// Write manifest
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(cm.manifestPath(userMsgUUID), data, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

// RewindFiles restores files to the state captured at the given checkpoint.
// If dryRun is true, it returns stats without modifying files.
func (cm *CheckpointManager) RewindFiles(userMsgUUID string, dryRun bool) (*agent.RewindFilesResult, error) {
	manifestData, err := os.ReadFile(cm.manifestPath(userMsgUUID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCheckpointMissing
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest CheckpointManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	result := &agent.RewindFilesResult{CanRewind: true}
	filesDir := cm.filesDir(userMsgUUID)

	for _, snap := range manifest.Files {
		if !snap.Exists {
			// File didn't exist at checkpoint — delete it if it now exists
			if _, err := os.Stat(snap.Path); err == nil {
				result.FilesChanged = append(result.FilesChanged, snap.Path)
				result.Deletions++
				if !dryRun {
					if err := os.Remove(snap.Path); err != nil {
						result.Error = fmt.Sprintf("failed to delete %q: %s", snap.Path, err)
						result.CanRewind = false
					}
				}
			}
			continue
		}

		// File existed at checkpoint — restore it
		currentData, err := os.ReadFile(snap.Path)
		currentExists := err == nil

		// Check if restoration is needed
		if currentExists {
			currentHash := sha256.Sum256(currentData)
			if hex.EncodeToString(currentHash[:]) == snap.Hash {
				continue // file unchanged, skip
			}
		}

		result.FilesChanged = append(result.FilesChanged, snap.Path)
		if currentExists {
			result.Insertions++ // file modified — counting as insertion (overwrite)
		} else {
			result.Insertions++ // file was deleted — re-creating
		}

		if !dryRun {
			// Read snapshot content from content-addressed store
			snapData, err := os.ReadFile(filepath.Join(filesDir, snap.Hash))
			if err != nil {
				result.Error = fmt.Sprintf("failed to read checkpoint content for %q: %s", snap.Path, err)
				result.CanRewind = false
				continue
			}

			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(snap.Path), 0755); err != nil {
				result.Error = fmt.Sprintf("failed to create dir for %q: %s", snap.Path, err)
				result.CanRewind = false
				continue
			}

			if err := os.WriteFile(snap.Path, snapData, 0644); err != nil {
				result.Error = fmt.Sprintf("failed to restore %q: %s", snap.Path, err)
				result.CanRewind = false
			}
		}
	}

	return result, nil
}
