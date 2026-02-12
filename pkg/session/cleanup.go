package session

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CleanupConfig configures session cleanup behavior.
type CleanupConfig struct {
	RetentionDays int // sessions older than this are deleted (default: 30)
}

// CleanupStats reports the outcome of a cleanup run.
type CleanupStats struct {
	SessionsDeleted int
	BytesFreed      int64
}

// protectedDirNames are directory names that must never be deleted during cleanup.
// These contain persistent data that outlives individual sessions.
var protectedDirNames = map[string]bool{
	"memory":       true, // auto-memory (project-scoped MEMORY.md)
	"agent-memory": true, // per-agent memory directories
}

// Cleanup walks the sessions under baseDir and deletes those whose metadata
// indicates they haven't been updated within the retention window.
// Auto-memory and agent-memory directories are always preserved.
func Cleanup(baseDir string, config CleanupConfig) (CleanupStats, error) {
	retentionDays := config.RetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	var stats CleanupStats

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}
		return stats, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Never delete protected directories
		if isProtectedDir(name) {
			continue
		}

		dir := filepath.Join(baseDir, name)

		// Try to load metadata to check UpdatedAt
		meta, err := loadMetadata(dir)
		if err != nil {
			// No valid metadata — check dir modification time as fallback
			info, statErr := entry.Info()
			if statErr != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				size := dirSize(dir)
				if rmErr := os.RemoveAll(dir); rmErr == nil {
					stats.SessionsDeleted++
					stats.BytesFreed += size
				}
			}
			continue
		}

		// Check against retention cutoff
		lastActive := meta.UpdatedAt
		if lastActive.IsZero() {
			lastActive = meta.CreatedAt
		}
		if lastActive.Before(cutoff) {
			size := dirSize(dir)
			if rmErr := os.RemoveAll(dir); rmErr == nil {
				stats.SessionsDeleted++
				stats.BytesFreed += size
			}
		}
	}

	return stats, nil
}

// isProtectedDir returns true if the directory name matches a protected pattern.
func isProtectedDir(name string) bool {
	if protectedDirNames[name] {
		return true
	}
	// Also protect any dir ending with "-memory" (e.g., custom agent memory dirs)
	if strings.HasSuffix(name, "-memory") {
		return true
	}
	return false
}

// dirSize calculates the total size of all files under dir.
func dirSize(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
