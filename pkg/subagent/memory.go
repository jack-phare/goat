package subagent

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const maxMemoryLines = 200

// resolveMemoryDir returns the memory directory path for an agent.
// If scope is "auto", it creates a per-agent subdirectory under the project memory dir.
// If scope is a path, it uses that directly.
func resolveMemoryDir(agentName, scope, cwd string) string {
	if scope == "" {
		return ""
	}

	if scope == "auto" {
		// Default: ~/.claude/projects/<project-hash>/agents/<agent-name>/memory/
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".claude", "projects", sanitizePath(cwd), "agents", agentName, "memory")
	}

	// Treat scope as a direct path
	return scope
}

// ensureMemoryDir creates the memory directory if it doesn't exist.
func ensureMemoryDir(dir string) error {
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// loadMemoryContent reads the first maxMemoryLines lines of MEMORY.md from the given dir.
// Returns empty string if the file doesn't exist.
func loadMemoryContent(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}

	path := filepath.Join(dir, "MEMORY.md")
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
	for scanner.Scan() && len(lines) < maxMemoryLines {
		lines = append(lines, scanner.Text())
	}

	return strings.Join(lines, "\n"), scanner.Err()
}

// sanitizePath converts a path into a safe directory name.
func sanitizePath(path string) string {
	// Replace path separators and special chars with dashes
	result := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', ' ':
			return '-'
		default:
			return r
		}
	}, path)

	// Trim leading dashes
	return strings.TrimLeft(result, "-")
}
