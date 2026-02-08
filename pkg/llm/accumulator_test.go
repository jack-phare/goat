package llm

import "testing"

func TestToolCallAccumulator(t *testing.T) {
	t.Run("single tool call", func(t *testing.T) {
		acc := NewToolCallAccumulator()
		acc.AddDelta(ToolCall{Index: 0, ID: "call_1", Type: "function", Function: FunctionCall{Name: "Bash", Arguments: ""}})
		acc.AddDelta(ToolCall{Index: 0, Function: FunctionCall{Arguments: `{"command":`}})
		acc.AddDelta(ToolCall{Index: 0, Function: FunctionCall{Arguments: ` "ls"}`}})

		calls := acc.Complete()
		if len(calls) != 1 {
			t.Fatalf("got %d calls, want 1", len(calls))
		}
		if calls[0].ID != "call_1" {
			t.Errorf("ID = %q, want %q", calls[0].ID, "call_1")
		}
		if calls[0].Function.Name != "Bash" {
			t.Errorf("Name = %q, want %q", calls[0].Function.Name, "Bash")
		}
		if calls[0].Function.Arguments != `{"command": "ls"}` {
			t.Errorf("Arguments = %q, want %q", calls[0].Function.Arguments, `{"command": "ls"}`)
		}
	})

	t.Run("multiple tool calls", func(t *testing.T) {
		acc := NewToolCallAccumulator()
		acc.AddDelta(ToolCall{Index: 0, ID: "call_1", Type: "function", Function: FunctionCall{Name: "Bash"}})
		acc.AddDelta(ToolCall{Index: 1, ID: "call_2", Type: "function", Function: FunctionCall{Name: "Read"}})
		acc.AddDelta(ToolCall{Index: 2, ID: "call_3", Type: "function", Function: FunctionCall{Name: "Write"}})
		acc.AddDelta(ToolCall{Index: 0, Function: FunctionCall{Arguments: `{"command":"ls"}`}})
		acc.AddDelta(ToolCall{Index: 1, Function: FunctionCall{Arguments: `{"path":"foo.go"}`}})
		acc.AddDelta(ToolCall{Index: 2, Function: FunctionCall{Arguments: `{"path":"bar.go"}`}})

		calls := acc.Complete()
		if len(calls) != 3 {
			t.Fatalf("got %d calls, want 3", len(calls))
		}
		if calls[0].Function.Name != "Bash" {
			t.Errorf("calls[0].Name = %q, want Bash", calls[0].Function.Name)
		}
		if calls[1].Function.Name != "Read" {
			t.Errorf("calls[1].Name = %q, want Read", calls[1].Function.Name)
		}
		if calls[2].Function.Name != "Write" {
			t.Errorf("calls[2].Name = %q, want Write", calls[2].Function.Name)
		}
	})

	t.Run("interleaved deltas", func(t *testing.T) {
		acc := NewToolCallAccumulator()
		acc.AddDelta(ToolCall{Index: 0, ID: "call_1", Type: "function", Function: FunctionCall{Name: "Bash"}})
		acc.AddDelta(ToolCall{Index: 1, ID: "call_2", Type: "function", Function: FunctionCall{Name: "Read"}})
		acc.AddDelta(ToolCall{Index: 0, Function: FunctionCall{Arguments: `{"cmd":`}})
		acc.AddDelta(ToolCall{Index: 1, Function: FunctionCall{Arguments: `{"path":`}})
		acc.AddDelta(ToolCall{Index: 0, Function: FunctionCall{Arguments: `"ls"}`}})
		acc.AddDelta(ToolCall{Index: 1, Function: FunctionCall{Arguments: `"f.go"}`}})

		calls := acc.Complete()
		if len(calls) != 2 {
			t.Fatalf("got %d calls, want 2", len(calls))
		}
		if calls[0].Function.Arguments != `{"cmd":"ls"}` {
			t.Errorf("calls[0].Arguments = %q", calls[0].Function.Arguments)
		}
		if calls[1].Function.Arguments != `{"path":"f.go"}` {
			t.Errorf("calls[1].Arguments = %q", calls[1].Function.Arguments)
		}
	})

	t.Run("sparse indices", func(t *testing.T) {
		acc := NewToolCallAccumulator()
		acc.AddDelta(ToolCall{Index: 0, ID: "call_1", Type: "function", Function: FunctionCall{Name: "A", Arguments: `{}`}})
		acc.AddDelta(ToolCall{Index: 3, ID: "call_4", Type: "function", Function: FunctionCall{Name: "D", Arguments: `{}`}})

		calls := acc.Complete()
		if len(calls) != 2 {
			t.Fatalf("got %d calls, want 2", len(calls))
		}
		if calls[0].Function.Name != "A" {
			t.Errorf("calls[0].Name = %q, want A", calls[0].Function.Name)
		}
		if calls[1].Function.Name != "D" {
			t.Errorf("calls[1].Name = %q, want D", calls[1].Function.Name)
		}
	})
}
