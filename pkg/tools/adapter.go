package tools

// llmToolAdapter wraps a tools.Tool to satisfy the llm.Tool interface.
type llmToolAdapter struct {
	tool Tool
}

func (a *llmToolAdapter) ToolName() string        { return a.tool.Name() }
func (a *llmToolAdapter) Description() string      { return a.tool.Description() }
func (a *llmToolAdapter) InputSchema() map[string]any { return a.tool.InputSchema() }
