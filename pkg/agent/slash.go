package agent

import "strings"

// builtinCommands are CLI commands that should NOT be intercepted as skills.
var builtinCommands = map[string]bool{
	"help":    true,
	"clear":   true,
	"compact": true,
	"config":  true,
	"cost":    true,
	"doctor":  true,
	"fast":    true,
	"login":   true,
	"logout":  true,
	"mcp":     true,
	"model":   true,
	"review":  true,
	"status":  true,
	"vim":     true,
}

// ParseSlashCommand detects if input is a slash command and extracts the skill name and args.
// Returns skillName, args, and whether the input is a slash command.
// Built-in CLI commands (e.g., /help, /clear) are excluded and return isSlash=false.
func ParseSlashCommand(input string, knownCommands []string) (skillName, args string, isSlash bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}

	// Strip the leading "/"
	rest := input[1:]
	if rest == "" {
		return "", "", false
	}

	// Split on first space
	var name, remainder string
	if idx := strings.IndexByte(rest, ' '); idx >= 0 {
		name = rest[:idx]
		remainder = strings.TrimSpace(rest[idx+1:])
	} else {
		name = rest
	}

	// Exclude built-in CLI commands
	if builtinCommands[name] {
		return "", "", false
	}

	return name, remainder, true
}
