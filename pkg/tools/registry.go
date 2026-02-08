package tools

import (
	"sort"

	"github.com/jg-phare/goat/pkg/llm"
)

// Registry holds available tools and resolves them by name.
type Registry struct {
	tools    map[string]Tool
	allowed  map[string]bool // auto-allowed tools (no permission prompt)
	disabled map[string]bool // explicitly disallowed
}

// RegistryOption configures a Registry.
type RegistryOption func(*Registry)

// WithAllowed marks tool names as auto-allowed.
func WithAllowed(names ...string) RegistryOption {
	return func(r *Registry) {
		for _, n := range names {
			r.allowed[n] = true
		}
	}
}

// WithDisabled marks tool names as disabled.
func WithDisabled(names ...string) RegistryOption {
	return func(r *Registry) {
		for _, n := range names {
			r.disabled[n] = true
		}
	}
}

// NewRegistry creates a new tool registry.
func NewRegistry(opts ...RegistryOption) *Registry {
	r := &Registry{
		tools:    make(map[string]Tool),
		allowed:  make(map[string]bool),
		disabled: make(map[string]bool),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// IsAllowed returns true if the tool is auto-allowed (no permission prompt needed).
func (r *Registry) IsAllowed(name string) bool {
	return r.allowed[name]
}

// IsDisabled returns true if the tool is explicitly disallowed.
func (r *Registry) IsDisabled(name string) bool {
	return r.disabled[name]
}

// Names returns all registered tool names in sorted order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		if !r.disabled[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// ToolDefinitions returns OpenAI-format tool definitions for all enabled tools.
func (r *Registry) ToolDefinitions() []llm.ToolDefinition {
	names := r.Names()
	defs := make([]llm.ToolDefinition, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		defs = append(defs, llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.InputSchema(),
			},
		})
	}
	return defs
}

// LLMTools returns adapters that satisfy the llm.Tool interface,
// for use with llm.buildCompletionRequest.
func (r *Registry) LLMTools() []llm.Tool {
	names := r.Names()
	adapted := make([]llm.Tool, 0, len(names))
	for _, name := range names {
		adapted = append(adapted, &llmToolAdapter{tool: r.tools[name]})
	}
	return adapted
}
