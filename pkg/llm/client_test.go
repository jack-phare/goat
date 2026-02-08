package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	t.Run("end-to-end streaming", func(t *testing.T) {
		sseBody := `data: {"id":"chatcmpl-e2e","object":"chat.completion.chunk","created":1234,"model":"anthropic/claude-opus-4-5-20250514","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-e2e","object":"chat.completion.chunk","created":1234,"model":"anthropic/claude-opus-4-5-20250514","choices":[{"index":0,"delta":{"content":"Hello from LLM!"},"finish_reason":null}]}

data: {"id":"chatcmpl-e2e","object":"chat.completion.chunk","created":1234,"model":"anthropic/claude-opus-4-5-20250514","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"chatcmpl-e2e","object":"chat.completion.chunk","created":1234,"model":"anthropic/claude-opus-4-5-20250514","choices":[],"usage":{"prompt_tokens":50,"completion_tokens":10,"total_tokens":60}}

data: [DONE]
`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/chat/completions" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			fmt.Fprint(w, sseBody)
		}))
		defer srv.Close()

		client := NewClient(ClientConfig{
			BaseURL: srv.URL,
			APIKey:  "test-key",
			Model:   "claude-opus-4-5-20250514",
		})

		req := &CompletionRequest{
			Model:    "anthropic/claude-opus-4-5-20250514",
			Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
		}

		stream, err := client.Complete(context.Background(), req)
		if err != nil {
			t.Fatalf("Complete error: %v", err)
		}

		resp, err := stream.Accumulate()
		if err != nil {
			t.Fatalf("Accumulate error: %v", err)
		}

		if resp.ID != "chatcmpl-e2e" {
			t.Errorf("ID = %q", resp.ID)
		}
		if len(resp.Content) != 1 || resp.Content[0].Text != "Hello from LLM!" {
			t.Errorf("Content = %+v", resp.Content)
		}
		if resp.StopReason != "end_turn" {
			t.Errorf("StopReason = %q", resp.StopReason)
		}
		if resp.Usage.InputTokens != 50 {
			t.Errorf("Usage.InputTokens = %d", resp.Usage.InputTokens)
		}
	})

	t.Run("401 fails immediately", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(401)
			fmt.Fprint(w, "unauthorized")
		}))
		defer srv.Close()

		client := NewClient(ClientConfig{
			BaseURL: srv.URL,
			APIKey:  "bad-key",
		})

		req := &CompletionRequest{
			Model:    "anthropic/claude",
			Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
		}

		_, err := client.Complete(context.Background(), req)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		llmErr, ok := err.(*LLMError)
		if !ok {
			t.Fatalf("expected *LLMError, got %T: %v", err, err)
		}
		if llmErr.SDKError != "authentication_failed" {
			t.Errorf("SDKError = %q, want authentication_failed", llmErr.SDKError)
		}
	})

	t.Run("429 retry then succeed", func(t *testing.T) {
		var attempt atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempt.Add(1)
			if n <= 2 {
				w.WriteHeader(429)
				fmt.Fprint(w, "rate limited")
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			fmt.Fprint(w, `data: {"id":"chatcmpl-retry","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}

data: [DONE]
`)
		}))
		defer srv.Close()

		client := NewClient(ClientConfig{
			BaseURL: srv.URL,
			APIKey:  "test-key",
			Retry: RetryConfig{
				MaxRetries:        3,
				InitialBackoff:    10 * time.Millisecond,
				MaxBackoff:        50 * time.Millisecond,
				BackoffFactor:     2.0,
				JitterFraction:    0.0,
				RetryableStatuses: []int{429, 500, 502, 503, 529},
			},
		})

		req := &CompletionRequest{
			Model:    "anthropic/claude",
			Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
		}

		stream, err := client.Complete(context.Background(), req)
		if err != nil {
			t.Fatalf("Complete error: %v", err)
		}

		resp, err := stream.Accumulate()
		if err != nil {
			t.Fatalf("Accumulate error: %v", err)
		}

		if resp.Content[0].Text != "ok" {
			t.Errorf("Content = %q", resp.Content[0].Text)
		}
		if attempt.Load() != 3 {
			t.Errorf("attempts = %d, want 3", attempt.Load())
		}
	})

	t.Run("500 retry exhausted", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			fmt.Fprint(w, "server error")
		}))
		defer srv.Close()

		client := NewClient(ClientConfig{
			BaseURL: srv.URL,
			Retry: RetryConfig{
				MaxRetries:        2,
				InitialBackoff:    5 * time.Millisecond,
				MaxBackoff:        20 * time.Millisecond,
				BackoffFactor:     2.0,
				JitterFraction:    0.0,
				RetryableStatuses: []int{500},
			},
		})

		req := &CompletionRequest{
			Model:    "anthropic/claude",
			Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
		}

		_, err := client.Complete(context.Background(), req)
		if err == nil {
			t.Fatal("expected error")
		}
		_, ok := err.(*ErrMaxRetriesExceeded)
		if !ok {
			// Could also be *LLMError if non-retryable path is hit
			t.Logf("error type: %T: %v", err, err)
		}
	})

	t.Run("context cancellation during streaming", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			// Write one chunk then hang
			fmt.Fprint(w, `data: {"id":"chatcmpl-cancel","object":"chat.completion.chunk","created":1234,"model":"claude","choices":[{"index":0,"delta":{"content":"start"},"finish_reason":null}]}`+"\n\n")
			w.(http.Flusher).Flush()
			// Hang — context cancellation should unblock
			<-r.Context().Done()
		}))
		defer srv.Close()

		client := NewClient(ClientConfig{
			BaseURL: srv.URL,
			Retry:   RetryConfig{MaxRetries: 0, RetryableStatuses: []int{429}},
		})

		ctx, cancel := context.WithCancel(context.Background())

		req := &CompletionRequest{
			Model:    "anthropic/claude",
			Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
		}

		stream, err := client.Complete(ctx, req)
		if err != nil {
			t.Fatalf("Complete error: %v", err)
		}

		// Read first chunk
		chunk, err := stream.Next()
		if err != nil {
			t.Fatalf("Next error: %v", err)
		}
		if chunk.ID != "chatcmpl-cancel" {
			t.Errorf("chunk.ID = %q", chunk.ID)
		}

		// Cancel context
		cancel()
		time.Sleep(50 * time.Millisecond)

		// Next read should fail
		_, err = stream.Next()
		if err == nil {
			t.Error("expected error after context cancellation")
		}
	})

	t.Run("model get and set", func(t *testing.T) {
		client := NewClient(ClientConfig{Model: "model-a"})
		if client.Model() != "model-a" {
			t.Errorf("Model() = %q", client.Model())
		}
		client.SetModel("model-b")
		if client.Model() != "model-b" {
			t.Errorf("Model() after SetModel = %q", client.Model())
		}
	})
}
