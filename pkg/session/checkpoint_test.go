package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newTestCheckpointManager(t *testing.T) (*CheckpointManager, string) {
	t.Helper()
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "session-1")
	os.MkdirAll(sessionDir, 0755)
	return newCheckpointManager(sessionDir), dir
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeTestFile: %v", err)
	}
}

func TestCheckpoint_CreateStoresCorrectContent(t *testing.T) {
	cm, baseDir := newTestCheckpointManager(t)

	filePath := filepath.Join(baseDir, "src", "main.go")
	writeTestFile(t, filePath, "package main\nfunc main() {}\n")

	err := cm.CreateCheckpoint("msg-uuid-1", []string{filePath})
	if err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	// Verify manifest exists and is correct
	manifestData, err := os.ReadFile(cm.manifestPath("msg-uuid-1"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest CheckpointManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	if manifest.UserMessageUUID != "msg-uuid-1" {
		t.Errorf("manifest UUID = %q, want msg-uuid-1", manifest.UserMessageUUID)
	}
	if len(manifest.Files) != 1 {
		t.Fatalf("manifest files = %d, want 1", len(manifest.Files))
	}
	if !manifest.Files[0].Exists {
		t.Error("file should be marked as Exists=true")
	}
	if manifest.Files[0].Hash == "" {
		t.Error("file should have a hash")
	}

	// Verify content-addressed file
	snapContent, err := os.ReadFile(filepath.Join(cm.filesDir("msg-uuid-1"), manifest.Files[0].Hash))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(snapContent) != "package main\nfunc main() {}\n" {
		t.Errorf("snapshot content = %q, want source content", snapContent)
	}
}

func TestCheckpoint_NonexistentFile_RecordsExistsFalse(t *testing.T) {
	cm, _ := newTestCheckpointManager(t)

	err := cm.CreateCheckpoint("msg-uuid-2", []string{"/nonexistent/file.go"})
	if err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	manifestData, _ := os.ReadFile(cm.manifestPath("msg-uuid-2"))
	var manifest CheckpointManifest
	json.Unmarshal(manifestData, &manifest)

	if len(manifest.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(manifest.Files))
	}
	if manifest.Files[0].Exists {
		t.Error("nonexistent file should have Exists=false")
	}
	if manifest.Files[0].Hash != "" {
		t.Error("nonexistent file should have empty hash")
	}
}

func TestCheckpoint_RewindRestoresFiles(t *testing.T) {
	cm, baseDir := newTestCheckpointManager(t)

	filePath := filepath.Join(baseDir, "data.txt")
	writeTestFile(t, filePath, "original content")

	// Create checkpoint
	cm.CreateCheckpoint("msg-uuid-3", []string{filePath})

	// Modify file
	writeTestFile(t, filePath, "modified content")

	// Rewind
	result, err := cm.RewindFiles("msg-uuid-3", false)
	if err != nil {
		t.Fatalf("RewindFiles: %v", err)
	}
	if !result.CanRewind {
		t.Errorf("CanRewind = false, want true; error: %s", result.Error)
	}
	if len(result.FilesChanged) != 1 {
		t.Errorf("FilesChanged = %d, want 1", len(result.FilesChanged))
	}

	// Verify file restored
	data, _ := os.ReadFile(filePath)
	if string(data) != "original content" {
		t.Errorf("file content after rewind = %q, want 'original content'", data)
	}
}

func TestCheckpoint_RewindDeletesNewFiles(t *testing.T) {
	cm, baseDir := newTestCheckpointManager(t)

	filePath := filepath.Join(baseDir, "new.txt")

	// Create checkpoint when file doesn't exist
	cm.CreateCheckpoint("msg-uuid-4", []string{filePath})

	// Now create the file
	writeTestFile(t, filePath, "new content")

	// Rewind should delete it
	result, err := cm.RewindFiles("msg-uuid-4", false)
	if err != nil {
		t.Fatalf("RewindFiles: %v", err)
	}
	if !result.CanRewind {
		t.Errorf("CanRewind = false; error: %s", result.Error)
	}
	if result.Deletions != 1 {
		t.Errorf("Deletions = %d, want 1", result.Deletions)
	}

	// File should be gone
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("file should have been deleted by rewind")
	}
}

func TestCheckpoint_RewindDryRun(t *testing.T) {
	cm, baseDir := newTestCheckpointManager(t)

	filePath := filepath.Join(baseDir, "dryrun.txt")
	writeTestFile(t, filePath, "original")

	cm.CreateCheckpoint("msg-uuid-5", []string{filePath})

	// Modify file
	writeTestFile(t, filePath, "changed")

	// Dry run should report changes but not modify
	result, err := cm.RewindFiles("msg-uuid-5", true)
	if err != nil {
		t.Fatalf("RewindFiles dry run: %v", err)
	}
	if !result.CanRewind {
		t.Error("CanRewind should be true for dry run")
	}
	if len(result.FilesChanged) != 1 {
		t.Errorf("FilesChanged = %d, want 1", len(result.FilesChanged))
	}

	// File should still be modified
	data, _ := os.ReadFile(filePath)
	if string(data) != "changed" {
		t.Errorf("file content after dry run = %q, want 'changed' (should not have been modified)", data)
	}
}

func TestCheckpoint_SHA256Deduplication(t *testing.T) {
	cm, baseDir := newTestCheckpointManager(t)

	file1 := filepath.Join(baseDir, "file1.txt")
	file2 := filepath.Join(baseDir, "file2.txt")
	sameContent := "identical content"

	writeTestFile(t, file1, sameContent)
	writeTestFile(t, file2, sameContent)

	err := cm.CreateCheckpoint("msg-uuid-6", []string{file1, file2})
	if err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	// Both files should share the same hash file
	hash := sha256.Sum256([]byte(sameContent))
	hashStr := hex.EncodeToString(hash[:])

	filesDir := cm.filesDir("msg-uuid-6")
	entries, _ := os.ReadDir(filesDir)
	if len(entries) != 1 {
		t.Errorf("content-addressed files = %d, want 1 (deduplicated)", len(entries))
	}
	if entries[0].Name() != hashStr {
		t.Errorf("hash file name = %q, want %q", entries[0].Name(), hashStr)
	}
}

func TestCheckpoint_ManifestSerialization(t *testing.T) {
	cm, baseDir := newTestCheckpointManager(t)

	filePath := filepath.Join(baseDir, "test.txt")
	writeTestFile(t, filePath, "hello")

	cm.CreateCheckpoint("msg-uuid-7", []string{filePath, "/nonexistent"})

	// Load and verify manifest
	data, _ := os.ReadFile(cm.manifestPath("msg-uuid-7"))
	var manifest CheckpointManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if manifest.UserMessageUUID != "msg-uuid-7" {
		t.Errorf("UUID = %q, want msg-uuid-7", manifest.UserMessageUUID)
	}
	if manifest.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(manifest.Files))
	}
	if !manifest.Files[0].Exists {
		t.Error("first file should exist")
	}
	if manifest.Files[1].Exists {
		t.Error("second file should not exist")
	}
}

func TestCheckpoint_RewindMissing(t *testing.T) {
	cm, _ := newTestCheckpointManager(t)

	_, err := cm.RewindFiles("nonexistent-uuid", false)
	if err != ErrCheckpointMissing {
		t.Errorf("err = %v, want ErrCheckpointMissing", err)
	}
}

func TestCheckpoint_UnchangedFileSkipped(t *testing.T) {
	cm, baseDir := newTestCheckpointManager(t)

	filePath := filepath.Join(baseDir, "unchanged.txt")
	writeTestFile(t, filePath, "same content")

	cm.CreateCheckpoint("msg-uuid-8", []string{filePath})

	// Don't modify the file — rewind should report no changes
	result, err := cm.RewindFiles("msg-uuid-8", false)
	if err != nil {
		t.Fatalf("RewindFiles: %v", err)
	}
	if len(result.FilesChanged) != 0 {
		t.Errorf("FilesChanged = %d, want 0 (file unchanged)", len(result.FilesChanged))
	}
}

// --- Store-level checkpoint tests ---

func TestStore_CreateCheckpointAndRewind(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	meta := testMetadata("sess-cp", "/tmp")
	s.Create(meta)

	// Create a test file in a temp location
	dir := t.TempDir()
	filePath := filepath.Join(dir, "myfile.txt")
	writeTestFile(t, filePath, "before checkpoint")

	// Create checkpoint
	if err := s.CreateCheckpoint("sess-cp", "user-msg-1", []string{filePath}); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	// Modify the file
	writeTestFile(t, filePath, "after checkpoint")

	// Rewind via store
	result, err := s.RewindFiles("sess-cp", "user-msg-1", false)
	if err != nil {
		t.Fatalf("RewindFiles: %v", err)
	}
	if !result.CanRewind {
		t.Errorf("CanRewind = false; error: %s", result.Error)
	}

	// Verify restored
	data, _ := os.ReadFile(filePath)
	if string(data) != "before checkpoint" {
		t.Errorf("restored content = %q, want 'before checkpoint'", data)
	}
}
