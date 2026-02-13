package tools

// llmToolAdapter wraps a tools.Tool to satisfy the llm.Tool interface.
type llmToolAdapter struct {
	tool            Tool
	compactDesc     string // if non-empty, used instead of tool.Description()
}

func (a *llmToolAdapter) ToolName() string { return a.tool.Name() }
func (a *llmToolAdapter) Description() string {
	if a.compactDesc != "" {
		return a.compactDesc
	}
	return a.tool.Description()
}
func (a *llmToolAdapter) InputSchema() map[string]any { return a.tool.InputSchema() }

// compactDescriptions maps tool names to short descriptions (~1-2 sentences)
// optimized for models with limited instruction-following capacity (e.g., Llama via Groq).
// Groq recommends "clear and concise" tool descriptions for reliable function calling.
var compactDescriptions = map[string]string{
	"Bash":  "Execute a bash command. Use for system commands like git, npm, docker. Do not use for file operations — use dedicated tools instead.",
	"Read":  "Read a file from the filesystem. Returns file contents with line numbers.",
	"Write": "Write content to a file, creating or overwriting it.",
	"Edit":  "Replace an exact string in a file with new text. Requires reading the file first.",
	"Glob":  "Find files matching a glob pattern (e.g. \"**/*.go\"). Returns matching file paths.",
	"Grep":  "Search file contents using regex via ripgrep. Returns matching lines or file paths.",
}
