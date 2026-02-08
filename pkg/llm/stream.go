package llm

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/jg-phare/goat/pkg/types"
)

// Stream represents an active SSE streaming response.
type Stream struct {
	events <-chan StreamEvent
	body   io.ReadCloser
	cancel context.CancelFunc
}

// NewStream creates a Stream from an SSE event channel and HTTP response body.
func NewStream(events <-chan StreamEvent, body io.ReadCloser, cancel context.CancelFunc) *Stream {
	return &Stream{
		events: events,
		body:   body,
		cancel: cancel,
	}
}

// Next returns the next parsed StreamChunk, or io.EOF when done.
// Returns context.Canceled if the parent context was cancelled.
func (s *Stream) Next() (*StreamChunk, error) {
	event, ok := <-s.events
	if !ok {
		return nil, io.EOF
	}
	if event.Done {
		return nil, io.EOF
	}
	if event.Err != nil {
		return nil, event.Err
	}
	return event.Chunk, nil
}

// Accumulate reads all remaining chunks and returns the fully assembled CompletionResponse.
func (s *Stream) Accumulate() (*CompletionResponse, error) {
	return s.AccumulateWithCallback(nil)
}

// AccumulateWithCallback reads all chunks, calling cb for each chunk before accumulation.
func (s *Stream) AccumulateWithCallback(cb func(*StreamChunk)) (*CompletionResponse, error) {
	defer s.Close()

	var textAccum strings.Builder
	var thinkAccum strings.Builder
	toolAccum := NewToolCallAccumulator()
	var response CompletionResponse
	var usage *Usage

	for event := range s.events {
		if event.Err != nil {
			return nil, event.Err
		}
		if event.Done {
			break
		}

		chunk := event.Chunk
		if cb != nil {
			cb(chunk)
		}

		// Extract response metadata from first chunk
		if response.ID == "" {
			response.ID = chunk.ID
			response.Model = chunk.Model
		}

		// Process usage (arrives in final chunk with stream_options)
		if chunk.Usage != nil {
			usage = chunk.Usage
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			// Accumulate text content
			if delta.Content != nil {
				textAccum.WriteString(*delta.Content)
			}

			// Accumulate thinking content (LiteLLM passthrough)
			if delta.ReasoningContent != nil {
				thinkAccum.WriteString(*delta.ReasoningContent)
			}

			// Accumulate tool calls
			for _, tc := range delta.ToolCalls {
				toolAccum.AddDelta(tc)
			}

			// Capture finish reason
			if choice.FinishReason != nil {
				response.FinishReason = *choice.FinishReason
			}
		}
	}

	// Build content blocks in order: thinking → text → tool_use
	if thinkAccum.Len() > 0 {
		response.Content = append(response.Content, types.ContentBlock{
			Type:     "thinking",
			Thinking: thinkAccum.String(),
		})
	}
	if textAccum.Len() > 0 {
		response.Content = append(response.Content, types.ContentBlock{
			Type: "text",
			Text: textAccum.String(),
		})
	}
	for _, tc := range toolAccum.Complete() {
		var input map[string]any
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				// If arguments aren't valid JSON, store as raw string
				input = map[string]any{"_raw": tc.Function.Arguments}
			}
		}
		response.Content = append(response.Content, types.ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	response.ToolCalls = toolAccum.Complete()
	response.StopReason = translateFinishReason(response.FinishReason)
	response.Usage = translateUsage(usage)

	return &response, nil
}

// Close terminates the stream early and releases the HTTP connection.
func (s *Stream) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.body != nil {
		return s.body.Close()
	}
	return nil
}
