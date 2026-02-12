package prompt

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxMemoryLines = 200

// LoadFirstNLines reads the first n lines of a file. Returns empty string
// if the file doesn't exist. This is the shared helper used by both
// main agent auto-memory and subagent memory.
func LoadFirstNLines(path string, maxLines int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() && len(lines) < maxLines {
		lines = append(lines, scanner.Text())
	}

	return strings.Join(lines, "\n"), scanner.Err()
}

// ProjectHash returns a stable hash for the current project directory.
// If in a git repo, it hashes the repo root path. Otherwise, it uses
// sanitizePath on the given cwd.
func ProjectHash(cwd string) string {
	root := gitRepoRoot(cwd)
	if root != "" {
		h := sha256.Sum256([]byte(root))
		return fmt.Sprintf("%x", h[:8]) // 16-char hex prefix
	}
	return sanitizePathForMemory(cwd)
}

// ResolveAutoMemoryDir returns the auto-memory directory path for the main agent.
// Path: ~/.claude/projects/<hash>/memory/
// Creates the directory if missing. Returns empty string on error.
func ResolveAutoMemoryDir(cwd string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	hash := ProjectHash(cwd)
	dir := filepath.Join(home, ".claude", "projects", hash, "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return dir
}

// LoadAutoMemory reads the first 200 lines of MEMORY.md from the
// auto-memory directory for the given cwd. Returns empty string if
// no file exists or on error.
func LoadAutoMemory(cwd string) string {
	dir := ResolveAutoMemoryDir(cwd)
	if dir == "" {
		return ""
	}
	content, err := LoadFirstNLines(filepath.Join(dir, "MEMORY.md"), maxMemoryLines)
	if err != nil {
		return ""
	}
	return content
}

// gitRepoRoot runs `git rev-parse --show-toplevel` to find the repo root.
// Returns empty string if not in a git repo or on error.
func gitRepoRoot(cwd string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// sanitizePathForMemory converts a path into a safe directory name
// by replacing separators and special characters with dashes.
func sanitizePathForMemory(path string) string {
	result := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', ' ':
			return '-'
		default:
			return r
		}
	}, path)
	return strings.TrimLeft(result, "-")
}
