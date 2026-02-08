package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
)

// StreamEvent wraps a parsed chunk or an error.
type StreamEvent struct {
	Chunk *StreamChunk // Non-nil on successful parse
	Err   error        // Non-nil on parse error or stream error
	Done  bool         // True when "data: [DONE]" received
}

// ParseSSEStream reads an HTTP response body line-by-line and yields StreamEvents.
// The returned channel is closed when the stream ends (either [DONE] or error).
func ParseSSEStream(ctx context.Context, body io.ReadCloser) <-chan StreamEvent {
	ch := make(chan StreamEvent)

	go func() {
		defer close(ch)
		defer body.Close()

		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				ch <- StreamEvent{Err: ctx.Err()}
				return
			default:
			}

			line := scanner.Text()

			// Skip SSE comments (keep-alive pings)
			if strings.HasPrefix(line, ":") {
				continue
			}

			// Skip empty lines (event boundaries)
			if line == "" {
				continue
			}

			// Only process data lines
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// Check for stream termination
			if data == "[DONE]" {
				ch <- StreamEvent{Done: true}
				return
			}

			// Unmarshal JSON chunk
			var chunk StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				// Malformed JSON: skip, not fatal
				continue
			}

			ch <- StreamEvent{Chunk: &chunk}
		}

		// After scanner exits, check for errors or context cancellation
		if err := scanner.Err(); err != nil {
			select {
			case <-ctx.Done():
				ch <- StreamEvent{Err: ctx.Err()}
			default:
				ch <- StreamEvent{Err: err}
			}
			return
		}

		// Scanner may have exited cleanly (EOF) due to body.Close() from context cancel
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Err: ctx.Err()}
		default:
			// Normal EOF without [DONE] — stream ended unexpectedly
		}
	}()

	return ch
}
