package prompt

import (
	"os"
	"path/filepath"
	"runtime"
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
// It walks up parent directories looking for additional CLAUDE.md files,
// checking all three file patterns at each level.
// Returns "" if no files are found.
func LoadClaudeMD(cwd string) string {
	var sections []string

	// Load files at CWD level
	sections = appendClaudeMDFiles(sections, cwd)

	// Walk up parent directories
	parent := filepath.Dir(cwd)
	for parent != cwd {
		sections = appendClaudeMDFiles(sections, parent)
		cwd = parent
		parent = filepath.Dir(parent)
	}

	return strings.Join(sections, "\n\n---\n\n")
}

// appendClaudeMDFiles checks for all three CLAUDE.md file patterns in a directory.
func appendClaudeMDFiles(sections []string, dir string) []string {
	sections = appendIfExists(sections, filepath.Join(dir, "CLAUDE.md"))
	sections = appendIfExists(sections, filepath.Join(dir, ".claude", "CLAUDE.md"))
	sections = appendIfExists(sections, filepath.Join(dir, "CLAUDE.local.md"))
	return sections
}

// LoadManagedPolicy loads the OS-level managed CLAUDE.md policy file.
// Returns empty string if no managed policy exists or on error.
//
// Paths:
//   - macOS: /Library/Application Support/ClaudeCode/CLAUDE.md
//   - Linux: /etc/claude-code/CLAUDE.md
func LoadManagedPolicy() string {
	var path string
	switch runtime.GOOS {
	case "darwin":
		path = "/Library/Application Support/ClaudeCode/CLAUDE.md"
	case "linux":
		path = "/etc/claude-code/CLAUDE.md"
	default:
		return ""
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// appendIfExists reads a file and appends its content to the slice if the file exists
// and has non-empty content. It also resolves @import directives.
func appendIfExists(sections []string, path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return sections
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return sections
	}
	// Resolve @import directives relative to the file's directory
	resolved, err := ResolveImports(content, filepath.Dir(path))
	if err == nil {
		content = resolved
	}
	return append(sections, content)
}
