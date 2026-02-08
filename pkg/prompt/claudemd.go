package prompt

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadClaudeMD discovers and loads CLAUDE.md files from the directory hierarchy.
// It searches CWD and parent directories, returning the combined content
// with files separated by "\n\n---\n\n".
//
// Loading order at each directory level:
//  1. CLAUDE.md
//  2. .claude/CLAUDE.md
//  3. CLAUDE.local.md
//
// It walks up parent directories looking for additional CLAUDE.md files.
// Returns "" if no files are found.
func LoadClaudeMD(cwd string) string {
	var sections []string

	// Load files at CWD level
	sections = appendIfExists(sections, filepath.Join(cwd, "CLAUDE.md"))
	sections = appendIfExists(sections, filepath.Join(cwd, ".claude", "CLAUDE.md"))
	sections = appendIfExists(sections, filepath.Join(cwd, "CLAUDE.local.md"))

	// Walk up parent directories
	parent := filepath.Dir(cwd)
	for parent != cwd {
		sections = appendIfExists(sections, filepath.Join(parent, "CLAUDE.md"))
		cwd = parent
		parent = filepath.Dir(parent)
	}

	return strings.Join(sections, "\n\n---\n\n")
}

// appendIfExists reads a file and appends its content to the slice if the file exists
// and has non-empty content.
func appendIfExists(sections []string, path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return sections
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return sections
	}
	return append(sections, content)
}
