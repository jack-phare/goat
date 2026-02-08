package session

import (
	"os"
	"path/filepath"
	"strings"
)

// SanitizePath converts an absolute path to a directory-safe name.
// e.g. "/Users/foo/bar" → "Users-foo-bar"
func SanitizePath(cwd string) string {
	s := strings.ReplaceAll(cwd, string(filepath.Separator), "-")
	return strings.TrimLeft(s, "-")
}

// DefaultBaseDir returns the default session storage root: ~/.claude/projects/
func DefaultBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".claude", "projects")
	}
	return filepath.Join(home, ".claude", "projects")
}
