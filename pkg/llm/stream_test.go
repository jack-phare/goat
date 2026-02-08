package llm

import (
	"context"
	"io"
	"strings"
	"testing"
)

func makeTestStream(sseData string) *Stream {
	body := io.NopCloser(strings.NewReader(sseData))
	ctx, cancel := context.WithCancel(context.Background())
	events := ParseSSEStream(ctx, body)
	return NewStream(events, body, cancel)
}

func TestStreamAccumulate(t *testing.T) {
	t.Run("text only", func(t *testing.T) {
		sseData := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"anthropic/claude-opus-4-5-20250514","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"anthropic/claude-opus-4-5-20250514","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"anthropic/claude-opus-4-5-20250514","choices":[{"index":0,"delta":{"content":" world!"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"anthropic/claude-opus-4-5-20250514","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"anthropic/claude-opus-4-5-20250514","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}

data: [DONE]
`
		stream := makeTestStream(sseData)
		resp, err := stream.Accumulate()
		if err != nil {
			t.Fatalf("Accumulate error: %v", err)
		}

		if resp.ID != "chatcmpl-1" {
			t.Errorf("ID = %q, want chatcmpl-1", resp.ID)
		}
		if resp.Model != "anthropic/claude-opus-4-5-20250514" {
			t.Errorf("Model = %q", resp.Model)
		}
		if resp.FinishReason != "stop" {
			t.Errorf("FinishReason = %q, want stop", resp.FinishReason)
		}
		if resp.StopReason != "end_turn" {
			t.Errorf("StopReason = %q, want end_turn", resp.StopReason)
		}
		if len(resp.Content) != 1 {
			t.Fatalf("len(Content) = %d, want 1", len(resp.Content))
		}
		if resp.Content[0].Type != "text" {
			t.Errorf("Content[0].Type = %q, want text", resp.Content[0].Type)
		}
		if resp.Content[0].Text != "Hello world!" {
			t.Errorf("Content[0].Text = %q, want 'Hello world!'", resp.Content[0].Text)
		}
		if resp.Usage.InputTokens != 100 {
			t.Errorf("Usage.InputTokens = %d, want 100", resp.Usage.InputTokens)
		}
	})

	t.Run("tool calls only", func(t *testing.T) {
		sseData := `data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"role":"assistant","content":null},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Bash","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\": \"ls\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
		stream := makeTestStream(sseData)
		resp, err := stream.Accumulate()
		if err != nil {
			t.Fatalf("Accumulate error: %v", err)
		}

		if resp.FinishReason != "tool_calls" {
			t.Errorf("FinishReason = %q, want tool_calls", resp.FinishReason)
		}
		if resp.StopReason != "tool_use" {
			t.Errorf("StopReason = %q, want tool_use", resp.StopReason)
		}
		if len(resp.Content) != 1 {
			t.Fatalf("len(Content) = %d, want 1", len(resp.Content))
		}
		if resp.Content[0].Type != "tool_use" {
			t.Errorf("Content[0].Type = %q, want tool_use", resp.Content[0].Type)
		}
		if resp.Content[0].Name != "Bash" {
			t.Errorf("Content[0].Name = %q, want Bash", resp.Content[0].Name)
		}
		if resp.Content[0].Input["command"] != "ls" {
			t.Errorf("Content[0].Input[command] = %v, want ls", resp.Content[0].Input["command"])
		}
	})

	t.Run("thinking + text + tool calls", func(t *testing.T) {
		sseData := `data: {"id":"chatcmpl-3","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me think..."},"finish_reason":null}]}

data: {"id":"chatcmpl-3","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"reasoning_content":" about this."},"finish_reason":null}]}

data: {"id":"chatcmpl-3","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"I'll run a command."},"finish_reason":null}]}

data: {"id":"chatcmpl-3","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Bash","arguments":"{\"command\": \"ls\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-3","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
		stream := makeTestStream(sseData)
		resp, err := stream.Accumulate()
		if err != nil {
			t.Fatalf("Accumulate error: %v", err)
		}

		// Content blocks ordered: thinking → text → tool_use
		if len(resp.Content) != 3 {
			t.Fatalf("len(Content) = %d, want 3", len(resp.Content))
		}
		if resp.Content[0].Type != "thinking" {
			t.Errorf("Content[0].Type = %q, want thinking", resp.Content[0].Type)
		}
		if resp.Content[0].Thinking != "Let me think... about this." {
			t.Errorf("Content[0].Thinking = %q", resp.Content[0].Thinking)
		}
		if resp.Content[1].Type != "text" {
			t.Errorf("Content[1].Type = %q, want text", resp.Content[1].Type)
		}
		if resp.Content[1].Text != "I'll run a command." {
			t.Errorf("Content[1].Text = %q", resp.Content[1].Text)
		}
		if resp.Content[2].Type != "tool_use" {
			t.Errorf("Content[2].Type = %q, want tool_use", resp.Content[2].Type)
		}
	})

	t.Run("empty response", func(t *testing.T) {
		sseData := `data: {"id":"chatcmpl-4","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-4","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
		stream := makeTestStream(sseData)
		resp, err := stream.Accumulate()
		if err != nil {
			t.Fatalf("Accumulate error: %v", err)
		}

		if len(resp.Content) != 0 {
			t.Errorf("len(Content) = %d, want 0 for empty response", len(resp.Content))
		}
	})

	t.Run("arguments with nested objects and unicode", func(t *testing.T) {
		sseData := `data: {"id":"chatcmpl-5","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Write","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-5","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\": \"test.go\", \"content\": \"Hello \\u4e16\\u754c\", \"nested\": {\"key\": \"value\"}}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-5","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
		stream := makeTestStream(sseData)
		resp, err := stream.Accumulate()
		if err != nil {
			t.Fatalf("Accumulate error: %v", err)
		}

		if len(resp.Content) != 1 {
			t.Fatalf("len(Content) = %d, want 1", len(resp.Content))
		}
		tc := resp.Content[0]
		if tc.Input["path"] != "test.go" {
			t.Errorf("Input[path] = %v", tc.Input["path"])
		}
		if tc.Input["content"] != "Hello 世界" {
			t.Errorf("Input[content] = %v", tc.Input["content"])
		}
		nested, ok := tc.Input["nested"].(map[string]any)
		if !ok {
			t.Fatalf("Input[nested] not a map: %T", tc.Input["nested"])
		}
		if nested["key"] != "value" {
			t.Errorf("Input[nested][key] = %v", nested["key"])
		}
	})

	t.Run("callback receives each chunk", func(t *testing.T) {
		sseData := `data: {"id":"chatcmpl-6","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"a"},"finish_reason":null}]}

data: {"id":"chatcmpl-6","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"b"},"finish_reason":null}]}

data: {"id":"chatcmpl-6","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
		stream := makeTestStream(sseData)
		var callbackCount int
		resp, err := stream.AccumulateWithCallback(func(chunk *StreamChunk) {
			callbackCount++
		})
		if err != nil {
			t.Fatalf("Accumulate error: %v", err)
		}

		if callbackCount != 3 {
			t.Errorf("callback called %d times, want 3", callbackCount)
		}
		if resp.Content[0].Text != "ab" {
			t.Errorf("Content[0].Text = %q, want ab", resp.Content[0].Text)
		}
	})
}
