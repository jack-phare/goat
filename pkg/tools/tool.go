package tools

import "context"

// SideEffectType classifies a tool's impact on system state.
type SideEffectType int

const (
	SideEffectNone     SideEffectType = iota // FileRead, Glob, Grep
	SideEffectReadOnly                       // WebSearch, WebFetch
	SideEffectMutating                       // Bash, FileWrite, FileEdit
	SideEffectNetwork                        // WebFetch, WebSearch
	SideEffectBlocking                       // AskUserQuestion
	SideEffectSpawns                         // Agent/Task tool
)

// ToolOutput is the result of a tool execution.
type ToolOutput struct {
	Content string // text content for the tool_result
	IsError bool   // when true, content is an error message
}

// Tool is the interface every tool must implement.
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any // JSON Schema object for the tools array
	SideEffect() SideEffectType
	Execute(ctx context.Context, input map[string]any) (ToolOutput, error)
}
