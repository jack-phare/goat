package tools

import (
	"context"
	"strings"
	"testing"
)

func TestConfig_SetThenGet(t *testing.T) {
	store := NewInMemoryConfigStore()
	tool := &ConfigTool{Store: store}

	// Set a value
	out, err := tool.Execute(context.Background(), map[string]any{
		"setting": "theme",
		"value":   "dark",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "theme set to dark") {
		t.Errorf("expected set message, got %q", out.Content)
	}

	// Get it back
	out, err = tool.Execute(context.Background(), map[string]any{
		"setting": "theme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "theme = dark") {
		t.Errorf("expected get result, got %q", out.Content)
	}
}

func TestConfig_GetNonexistent(t *testing.T) {
	store := NewInMemoryConfigStore()
	tool := &ConfigTool{Store: store}
	out, err := tool.Execute(context.Background(), map[string]any{
		"setting": "nonexistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for nonexistent key")
	}
}

func TestConfig_SetTypes(t *testing.T) {
	store := NewInMemoryConfigStore()
	tool := &ConfigTool{Store: store}

	tests := []struct {
		key   string
		value any
	}{
		{"str_key", "hello"},
		{"bool_key", true},
		{"num_key", float64(42)},
	}

	for _, tt := range tests {
		out, err := tool.Execute(context.Background(), map[string]any{
			"setting": tt.key,
			"value":   tt.value,
		})
		if err != nil {
			t.Fatal(err)
		}
		if out.IsError {
			t.Errorf("unexpected error setting %s: %s", tt.key, out.Content)
		}

		// Verify the value can be retrieved
		out, err = tool.Execute(context.Background(), map[string]any{
			"setting": tt.key,
		})
		if err != nil {
			t.Fatal(err)
		}
		if out.IsError {
			t.Errorf("unexpected error getting %s: %s", tt.key, out.Content)
		}
	}
}

func TestConfig_MissingSetting(t *testing.T) {
	tool := &ConfigTool{Store: NewInMemoryConfigStore()}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing setting")
	}
}

func TestConfig_NilStore(t *testing.T) {
	tool := &ConfigTool{}
	out, err := tool.Execute(context.Background(), map[string]any{"setting": "foo"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for nil store")
	}
}
