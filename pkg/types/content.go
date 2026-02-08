package types

import "encoding/json"

// BetaMessage mirrors the Anthropic Messages API response object.
// This is the canonical internal representation after stream accumulation.
type BetaMessage struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   *string        `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        BetaUsage      `json:"usage"`
}

// ContentBlock is a discriminated union for message content.
// Type field determines which other fields are populated.
//
// Invariants:
//   - type="text":     Text is set
//   - type="tool_use": ID, Name, Input are set
//   - type="thinking": Thinking is set
type ContentBlock struct {
	Type string `json:"type"` // "text" | "tool_use" | "thinking"

	// type="text"
	Text string `json:"text,omitempty"`

	// type="tool_use"
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"` // Parsed JSON (not string)

	// type="thinking"
	Thinking string `json:"thinking,omitempty"`
}

// MarshalJSON produces a clean JSON representation with only fields relevant to the block type.
func (cb ContentBlock) MarshalJSON() ([]byte, error) {
	switch cb.Type {
	case "text":
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: cb.Text})

	case "tool_use":
		return json.Marshal(struct {
			Type  string         `json:"type"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		}{Type: "tool_use", ID: cb.ID, Name: cb.Name, Input: cb.Input})

	case "thinking":
		return json.Marshal(struct {
			Type     string `json:"type"`
			Thinking string `json:"thinking"`
		}{Type: "thinking", Thinking: cb.Thinking})

	default:
		// Fallback: marshal all fields
		type Alias ContentBlock
		return json.Marshal(Alias(cb))
	}
}

// BetaUsage mirrors Anthropic's usage object with cache token fields.
// All fields are non-optional (zero-valued if absent).
type BetaUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}
