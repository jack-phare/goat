package llm

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestParseSSEStream(t *testing.T) {
	t.Run("simple text chunks", func(t *testing.T) {
		sseData := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
		body := io.NopCloser(strings.NewReader(sseData))
		ctx := context.Background()
		ch := ParseSSEStream(ctx, body)

		var chunks []*StreamChunk
		var done bool
		for event := range ch {
			if event.Err != nil {
				t.Fatalf("unexpected error: %v", event.Err)
			}
			if event.Done {
				done = true
				continue
			}
			chunks = append(chunks, event.Chunk)
		}

		if !done {
			t.Error("expected Done event")
		}
		if len(chunks) != 4 {
			t.Fatalf("got %d chunks, want 4", len(chunks))
		}
		if chunks[0].ID != "chatcmpl-1" {
			t.Errorf("chunk[0].ID = %q", chunks[0].ID)
		}
	})

	t.Run("tool calls", func(t *testing.T) {
		sseData := `data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"role":"assistant","content":null},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Bash","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":" \"ls\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
		body := io.NopCloser(strings.NewReader(sseData))
		ctx := context.Background()
		ch := ParseSSEStream(ctx, body)

		var chunks []*StreamChunk
		for event := range ch {
			if event.Err != nil {
				t.Fatalf("unexpected error: %v", event.Err)
			}
			if event.Chunk != nil {
				chunks = append(chunks, event.Chunk)
			}
		}

		if len(chunks) != 5 {
			t.Fatalf("got %d chunks, want 5", len(chunks))
		}

		// Verify tool call deltas present
		if len(chunks[1].Choices[0].Delta.ToolCalls) != 1 {
			t.Error("expected tool call delta in chunk[1]")
		}
		if chunks[1].Choices[0].Delta.ToolCalls[0].ID != "call_1" {
			t.Errorf("tool call ID = %q, want call_1", chunks[1].Choices[0].Delta.ToolCalls[0].ID)
		}
	})

	t.Run("comment lines and keep-alive", func(t *testing.T) {
		sseData := `: keep-alive

: another comment

data: {"id":"chatcmpl-3","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}

: ping

data: [DONE]
`
		body := io.NopCloser(strings.NewReader(sseData))
		ctx := context.Background()
		ch := ParseSSEStream(ctx, body)

		var chunks []*StreamChunk
		for event := range ch {
			if event.Err != nil {
				t.Fatalf("unexpected error: %v", event.Err)
			}
			if event.Chunk != nil {
				chunks = append(chunks, event.Chunk)
			}
		}

		if len(chunks) != 1 {
			t.Fatalf("got %d chunks, want 1 (comments should be skipped)", len(chunks))
		}
	})

	t.Run("malformed JSON skipped", func(t *testing.T) {
		sseData := `data: {"id":"chatcmpl-4","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}

data: {this is not valid json}

data: {"id":"chatcmpl-4","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"still ok"},"finish_reason":null}]}

data: [DONE]
`
		body := io.NopCloser(strings.NewReader(sseData))
		ctx := context.Background()
		ch := ParseSSEStream(ctx, body)

		var chunks []*StreamChunk
		for event := range ch {
			if event.Err != nil {
				t.Fatalf("unexpected error: %v", event.Err)
			}
			if event.Chunk != nil {
				chunks = append(chunks, event.Chunk)
			}
		}

		if len(chunks) != 2 {
			t.Fatalf("got %d chunks, want 2 (malformed JSON should be skipped)", len(chunks))
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		// Create a reader that blocks after one line
		pr, pw := io.Pipe()

		ctx, cancel := context.WithCancel(context.Background())
		ch := ParseSSEStream(ctx, pr)

		// Write one chunk then cancel context and close pipe
		go func() {
			pw.Write([]byte(`data: {"id":"chatcmpl-5","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}` + "\n"))
			// Wait a bit for the scanner to block on next read
			time.Sleep(50 * time.Millisecond)
			cancel()
			// Close the pipe to unblock the scanner after cancellation
			time.Sleep(20 * time.Millisecond)
			pw.Close()
		}()

		var gotCanceled bool
		timeout := time.After(2 * time.Second)
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					goto done
				}
				if event.Err == context.Canceled {
					gotCanceled = true
				}
			case <-timeout:
				t.Fatal("test timed out waiting for channel close")
			}
		}
	done:

		if !gotCanceled {
			t.Error("expected context.Canceled error")
		}
	})

	t.Run("thinking blocks via reasoning_content", func(t *testing.T) {
		sseData := `data: {"id":"chatcmpl-6","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me think..."},"finish_reason":null}]}

data: {"id":"chatcmpl-6","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"Here is my answer"},"finish_reason":null}]}

data: {"id":"chatcmpl-6","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
		body := io.NopCloser(strings.NewReader(sseData))
		ctx := context.Background()
		ch := ParseSSEStream(ctx, body)

		var chunks []*StreamChunk
		for event := range ch {
			if event.Err != nil {
				t.Fatalf("unexpected error: %v", event.Err)
			}
			if event.Chunk != nil {
				chunks = append(chunks, event.Chunk)
			}
		}

		if len(chunks) != 3 {
			t.Fatalf("got %d chunks, want 3", len(chunks))
		}
		rc := chunks[0].Choices[0].Delta.ReasoningContent
		if rc == nil || *rc != "Let me think..." {
			t.Errorf("reasoning_content = %v, want 'Let me think...'", rc)
		}
	})
}
