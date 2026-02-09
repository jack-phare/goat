package types

import (
	"encoding/json"
	"testing"
)

func TestContentBlock_MarshalJSON_Text(t *testing.T) {
	cb := ContentBlock{Type: "text", Text: "hello world"}
	data, err := json.Marshal(cb)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	json.Unmarshal(data, &m)

	if m["type"] != "text" {
		t.Errorf("type = %v", m["type"])
	}
	if m["text"] != "hello world" {
		t.Errorf("text = %v", m["text"])
	}
	// Verify no extra fields leak
	if _, ok := m["id"]; ok {
		t.Error("id should not be present for text block")
	}
	if _, ok := m["name"]; ok {
		t.Error("name should not be present for text block")
	}
	if _, ok := m["input"]; ok {
		t.Error("input should not be present for text block")
	}
	if _, ok := m["thinking"]; ok {
		t.Error("thinking should not be present for text block")
	}
}

func TestContentBlock_MarshalJSON_ToolUse(t *testing.T) {
	cb := ContentBlock{
		Type:  "tool_use",
		ID:    "call_1",
		Name:  "Bash",
		Input: map[string]any{"command": "ls"},
	}
	data, err := json.Marshal(cb)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	json.Unmarshal(data, &m)

	if m["type"] != "tool_use" {
		t.Errorf("type = %v", m["type"])
	}
	if m["id"] != "call_1" {
		t.Errorf("id = %v", m["id"])
	}
	if m["name"] != "Bash" {
		t.Errorf("name = %v", m["name"])
	}
	input, ok := m["input"].(map[string]any)
	if !ok {
		t.Fatal("input should be a map")
	}
	if input["command"] != "ls" {
		t.Errorf("input.command = %v", input["command"])
	}
	// No text or thinking fields
	if _, ok := m["text"]; ok {
		t.Error("text should not be present for tool_use block")
	}
	if _, ok := m["thinking"]; ok {
		t.Error("thinking should not be present for tool_use block")
	}
}

func TestContentBlock_MarshalJSON_Thinking(t *testing.T) {
	cb := ContentBlock{Type: "thinking", Thinking: "let me think..."}
	data, err := json.Marshal(cb)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	json.Unmarshal(data, &m)

	if m["type"] != "thinking" {
		t.Errorf("type = %v", m["type"])
	}
	if m["thinking"] != "let me think..." {
		t.Errorf("thinking = %v", m["thinking"])
	}
	// No text, id, name, input fields
	if _, ok := m["text"]; ok {
		t.Error("text should not be present for thinking block")
	}
	if _, ok := m["id"]; ok {
		t.Error("id should not be present for thinking block")
	}
}

func TestContentBlock_MarshalJSON_Unknown(t *testing.T) {
	cb := ContentBlock{Type: "custom", Text: "fallback"}
	data, err := json.Marshal(cb)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	json.Unmarshal(data, &m)

	if m["type"] != "custom" {
		t.Errorf("type = %v", m["type"])
	}
	// Fallback should include all non-empty fields
	if m["text"] != "fallback" {
		t.Errorf("text = %v", m["text"])
	}
}

func TestContentBlock_MarshalJSON_EmptyText(t *testing.T) {
	// Even with empty text, text type should include the text field
	cb := ContentBlock{Type: "text", Text: ""}
	data, err := json.Marshal(cb)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	json.Unmarshal(data, &m)

	if _, ok := m["text"]; !ok {
		t.Error("text field should be present even when empty for text type")
	}
}

func TestContentBlock_MarshalJSON_ToolUseNilInput(t *testing.T) {
	cb := ContentBlock{Type: "tool_use", ID: "call_1", Name: "Read", Input: nil}
	data, err := json.Marshal(cb)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	json.Unmarshal(data, &m)

	// Input should be null (present but nil)
	if _, ok := m["input"]; !ok {
		t.Error("input field should be present for tool_use even when nil")
	}
}

func TestContentBlock_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		orig ContentBlock
	}{
		{
			name: "text",
			orig: ContentBlock{Type: "text", Text: "hello world"},
		},
		{
			name: "text_empty",
			orig: ContentBlock{Type: "text", Text: ""},
		},
		{
			name: "thinking",
			orig: ContentBlock{Type: "thinking", Thinking: "let me consider..."},
		},
		{
			name: "tool_use",
			orig: ContentBlock{
				Type:  "tool_use",
				ID:    "call_42",
				Name:  "Bash",
				Input: map[string]any{"command": "ls -la"},
			},
		},
		{
			name: "tool_use_nil_input",
			orig: ContentBlock{Type: "tool_use", ID: "call_99", Name: "Read"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.orig)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var got ContentBlock
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if got.Type != tt.orig.Type {
				t.Errorf("Type = %q, want %q", got.Type, tt.orig.Type)
			}

			switch tt.orig.Type {
			case "text":
				if got.Text != tt.orig.Text {
					t.Errorf("Text = %q, want %q", got.Text, tt.orig.Text)
				}
			case "thinking":
				if got.Thinking != tt.orig.Thinking {
					t.Errorf("Thinking = %q, want %q", got.Thinking, tt.orig.Thinking)
				}
			case "tool_use":
				if got.ID != tt.orig.ID {
					t.Errorf("ID = %q, want %q", got.ID, tt.orig.ID)
				}
				if got.Name != tt.orig.Name {
					t.Errorf("Name = %q, want %q", got.Name, tt.orig.Name)
				}
				if tt.orig.Input != nil {
					if got.Input == nil {
						t.Fatal("Input should not be nil")
					}
					if got.Input["command"] != tt.orig.Input["command"] {
						t.Errorf("Input[command] = %v, want %v", got.Input["command"], tt.orig.Input["command"])
					}
				}
			}
		})
	}
}
