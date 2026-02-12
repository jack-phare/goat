package subagent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jg-phare/goat/pkg/prompt"
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
// Delegates to the shared prompt.LoadFirstNLines helper.
func loadMemoryContent(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	return prompt.LoadFirstNLines(filepath.Join(dir, "MEMORY.md"), maxMemoryLines)
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
