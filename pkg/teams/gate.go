package teams

import "os"

// IsEnabled returns true if the agent teams feature is enabled.
func IsEnabled() bool {
	return os.Getenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS") == "1"
}
