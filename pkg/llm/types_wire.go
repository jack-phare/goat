package llm

// CompletionRequest maps to OpenAI /v1/chat/completions request body.
type CompletionRequest struct {
	Model         string           `json:"model"`
	Messages      []ChatMessage    `json:"messages"`
	Tools         []ToolDefinition `json:"tools,omitempty"`
	Stream        bool             `json:"stream"`
	MaxTokens     int              `json:"max_tokens,omitempty"`
	Temperature   *float64         `json:"temperature,omitempty"`
	TopP          *float64         `json:"top_p,omitempty"`
	Stop          []string         `json:"stop,omitempty"`
	StreamOptions *StreamOptions   `json:"stream_options,omitempty"`

	// LiteLLM passthrough for Anthropic-specific fields
	ExtraBody map[string]any `json:"extra_body,omitempty"`
}

// StreamOptions requests usage info in the final streaming chunk.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// ChatMessage is an OpenAI-format message for the messages array.
// Content is `any` because it can be:
//   - string (simple text)
//   - []ContentPart (multimodal: text + images)
//   - nil (assistant message with only tool_calls)
type ChatMessage struct {
	Role       string     `json:"role"`                   // "system"|"user"|"assistant"|"tool"
	Content    any        `json:"content"`                // string | []ContentPart | nil
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant messages only
	ToolCallID string     `json:"tool_call_id,omitempty"` // tool result messages only
	Name       string     `json:"name,omitempty"`         // optional sender name
}

// ContentPart for multi-part content arrays (text, images, tool results).
type ContentPart struct {
	Type     string    `json:"type"`                // "text"|"image_url"
	Text     string    `json:"text,omitempty"`      // for type="text"
	ImageURL *ImageURL `json:"image_url,omitempty"` // for type="image_url"
}

// ImageURL holds an image reference for multimodal content.
type ImageURL struct {
	URL    string `json:"url"`              // base64 data URI or HTTPS URL
	Detail string `json:"detail,omitempty"` // "auto"|"low"|"high"
}

// ToolCall represents an assistant's request to invoke a tool.
type ToolCall struct {
	Index    int          `json:"index,omitempty"` // streaming only: identifies which call
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the function name and arguments for a tool call.
type FunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"` // JSON string, accumulated incrementally
}

// ToolDefinition is an OpenAI-format tool for the tools array.
type ToolDefinition struct {
	Type     string      `json:"type"` // "function"
	Function FunctionDef `json:"function"`
}

// FunctionDef describes a function available as a tool.
type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema object
}

// StreamChunk represents a single SSE chunk from LiteLLM.
type StreamChunk struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"` // "chat.completion.chunk"
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`              // final chunk only (stream_options)
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

// Choice represents a single choice in a streaming chunk.
type Choice struct {
	Index        int     `json:"index"`
	Delta        Delta   `json:"delta"`
	FinishReason *string `json:"finish_reason"` // null | "stop" | "tool_calls" | "length"
}

// Delta is the incremental content in a streaming chunk.
type Delta struct {
	Role             string     `json:"role,omitempty"`
	Content          *string    `json:"content,omitempty"`            // text content (nil vs "" matters)
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ReasoningContent *string    `json:"reasoning_content,omitempty"` // LiteLLM thinking passthrough
}

// Usage from the final streaming chunk or non-streaming response.
type Usage struct {
	PromptTokens             int `json:"prompt_tokens"`
	CompletionTokens         int `json:"completion_tokens"`
	TotalTokens              int `json:"total_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}
