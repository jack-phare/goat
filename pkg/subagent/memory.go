package subagent

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const maxMemoryLines = 200

// resolveMemoryDir returns the memory directory path for an agent.
// Named scopes: "user", "project", "local" map to well-known directories.
// "auto" is an alias for "user".
// Any other non-empty value is treated as a direct path.
func resolveMemoryDir(agentName, scope, cwd string) string {
	if scope == "" {
		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	switch scope {
	case "user", "auto":
		// ~/.claude/agent-memory/<agent-name>/
		return filepath.Join(home, ".claude", "agent-memory", agentName)
	case "project":
		// .claude/agent-memory/<agent-name>/ (relative to CWD, shareable via VCS)
		return filepath.Join(cwd, ".claude", "agent-memory", agentName)
	case "local":
		// .claude/agent-memory-local/<agent-name>/ (relative to CWD, not in VCS)
		return filepath.Join(cwd, ".claude", "agent-memory-local", agentName)
	default:
		// Treat as a direct path
		return scope
	}
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
