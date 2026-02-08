package tools

import (
	"context"
	"testing"
)

// stubTool is a minimal Tool for testing the registry.
type stubTool struct {
	name        string
	description string
	sideEffect  SideEffectType
}

func (s *stubTool) Name() string                { return s.name }
func (s *stubTool) Description() string          { return s.description }
func (s *stubTool) InputSchema() map[string]any  { return map[string]any{"type": "object"} }
func (s *stubTool) SideEffect() SideEffectType   { return s.sideEffect }
func (s *stubTool) Execute(_ context.Context, _ map[string]any) (ToolOutput, error) {
	return ToolOutput{Content: "ok"}, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := &stubTool{name: "Bash", description: "Execute commands"}
	r.Register(tool)

	got, ok := r.Get("Bash")
	if !ok {
		t.Fatal("expected to find Bash tool")
	}
	if got.Name() != "Bash" {
		t.Errorf("got name %q, want %q", got.Name(), "Bash")
	}

	_, ok = r.Get("NotExist")
	if ok {
		t.Error("expected NotExist to not be found")
	}
}

func TestRegistry_Names(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "Grep"})
	r.Register(&stubTool{name: "Bash"})
	r.Register(&stubTool{name: "FileRead"})

	names := r.Names()
	want := []string{"Bash", "FileRead", "Grep"}
	if len(names) != len(want) {
		t.Fatalf("got %d names, want %d", len(names), len(want))
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestRegistry_Disabled(t *testing.T) {
	r := NewRegistry(WithDisabled("Bash"))
	r.Register(&stubTool{name: "Bash"})
	r.Register(&stubTool{name: "Grep"})

	if !r.IsDisabled("Bash") {
		t.Error("expected Bash to be disabled")
	}

	// Disabled tools are excluded from Names()
	names := r.Names()
	if len(names) != 1 || names[0] != "Grep" {
		t.Errorf("expected only Grep, got %v", names)
	}

	// But can still be retrieved via Get()
	_, ok := r.Get("Bash")
	if !ok {
		t.Error("expected disabled tool to still be retrievable via Get()")
	}
}

func TestRegistry_Allowed(t *testing.T) {
	r := NewRegistry(WithAllowed("FileRead", "Glob"))

	if !r.IsAllowed("FileRead") {
		t.Error("expected FileRead to be allowed")
	}
	if !r.IsAllowed("Glob") {
		t.Error("expected Glob to be allowed")
	}
	if r.IsAllowed("Bash") {
		t.Error("expected Bash to not be auto-allowed")
	}
}

func TestRegistry_ToolDefinitions(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "Bash", description: "Execute commands"})
	r.Register(&stubTool{name: "Grep", description: "Search files"})

	defs := r.ToolDefinitions()
	if len(defs) != 2 {
		t.Fatalf("got %d definitions, want 2", len(defs))
	}

	// Should be sorted by name
	if defs[0].Function.Name != "Bash" {
		t.Errorf("first tool = %q, want Bash", defs[0].Function.Name)
	}
	if defs[1].Function.Name != "Grep" {
		t.Errorf("second tool = %q, want Grep", defs[1].Function.Name)
	}

	// All should be type "function"
	for _, d := range defs {
		if d.Type != "function" {
			t.Errorf("tool %s type = %q, want function", d.Function.Name, d.Type)
		}
	}
}

func TestRegistry_LLMTools(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "Bash", description: "Execute commands"})

	adapted := r.LLMTools()
	if len(adapted) != 1 {
		t.Fatalf("got %d LLM tools, want 1", len(adapted))
	}
	if adapted[0].ToolName() != "Bash" {
		t.Errorf("ToolName() = %q, want Bash", adapted[0].ToolName())
	}
	if adapted[0].Description() != "Execute commands" {
		t.Errorf("Description() = %q, want 'Execute commands'", adapted[0].Description())
	}
}

func TestRegistry_DisabledExcludedFromDefinitions(t *testing.T) {
	r := NewRegistry(WithDisabled("Bash"))
	r.Register(&stubTool{name: "Bash", description: "Execute commands"})
	r.Register(&stubTool{name: "Grep", description: "Search files"})

	defs := r.ToolDefinitions()
	if len(defs) != 1 {
		t.Fatalf("got %d definitions, want 1 (Bash should be excluded)", len(defs))
	}
	if defs[0].Function.Name != "Grep" {
		t.Errorf("expected only Grep, got %s", defs[0].Function.Name)
	}
}
