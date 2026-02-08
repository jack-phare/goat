package llm

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

var update = flag.Bool("update", false, "update golden files")

func loadSSETestData(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "sse", name+".txt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read SSE test data %s: %v", name, err)
	}
	return string(data)
}

func loadGoldenFile(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", name, err)
	}
	return data
}

func writeGoldenFile(t *testing.T, name string, data []byte) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", name+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write golden file %s: %v", name, err)
	}
}

func accumulateSSE(t *testing.T, sseData string) *CompletionResponse {
	t.Helper()
	body := io.NopCloser(strings.NewReader(sseData))
	ctx := context.Background()
	events := ParseSSEStream(ctx, body)
	stream := newStream(events, body, func() {})
	resp, err := stream.Accumulate()
	if err != nil {
		t.Fatalf("Accumulate error: %v", err)
	}
	return resp
}

// goldenCompletionResponse is a JSON-serializable version of CompletionResponse.
type goldenCompletionResponse struct {
	ID           string               `json:"id"`
	Model        string               `json:"model"`
	Content      []types.ContentBlock `json:"content"`
	FinishReason string               `json:"finish_reason"`
	StopReason   string               `json:"stop_reason"`
	Usage        types.BetaUsage      `json:"usage"`
}

func toGolden(resp *CompletionResponse) goldenCompletionResponse {
	return goldenCompletionResponse{
		ID:           resp.ID,
		Model:        resp.Model,
		Content:      resp.Content,
		FinishReason: resp.FinishReason,
		StopReason:   resp.StopReason,
		Usage:        resp.Usage,
	}
}

func TestGolden(t *testing.T) {
	tests := []struct {
		sseName    string
		goldenName string
	}{
		{"text_only", "text_only"},
		{"tool_calls", "tool_calls"},
		{"thinking_text_tools", "thinking_text_tools"},
	}

	for _, tt := range tests {
		t.Run(tt.sseName, func(t *testing.T) {
			sseData := loadSSETestData(t, tt.sseName)
			resp := accumulateSSE(t, sseData)

			golden := toGolden(resp)
			actual, err := json.MarshalIndent(golden, "", "  ")
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}

			if *update {
				writeGoldenFile(t, tt.goldenName, actual)
				t.Logf("updated golden file: %s", tt.goldenName)
				return
			}

			expected := loadGoldenFile(t, tt.goldenName)

			// Normalize: unmarshal both and compare
			var actualMap, expectedMap any
			json.Unmarshal(actual, &actualMap)
			json.Unmarshal(expected, &expectedMap)

			actualNorm, _ := json.Marshal(actualMap)
			expectedNorm, _ := json.Marshal(expectedMap)

			if string(actualNorm) != string(expectedNorm) {
				t.Errorf("golden mismatch for %s\n--- expected ---\n%s\n--- actual ---\n%s",
					tt.goldenName, string(expected), string(actual))
			}
		})
	}
}

func TestGoldenBetaMessage(t *testing.T) {
	tests := []struct {
		sseName    string
		goldenName string
	}{
		{"text_only", "beta_message_text"},
		{"tool_calls", "beta_message_tools"},
	}

	for _, tt := range tests {
		t.Run(tt.goldenName, func(t *testing.T) {
			sseData := loadSSETestData(t, tt.sseName)
			resp := accumulateSSE(t, sseData)
			betaMsg := resp.ToBetaMessage()

			actual, err := json.MarshalIndent(betaMsg, "", "  ")
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}

			if *update {
				writeGoldenFile(t, tt.goldenName, actual)
				t.Logf("updated golden file: %s", tt.goldenName)
				return
			}

			expected := loadGoldenFile(t, tt.goldenName)

			var actualMap, expectedMap any
			json.Unmarshal(actual, &actualMap)
			json.Unmarshal(expected, &expectedMap)

			actualNorm, _ := json.Marshal(actualMap)
			expectedNorm, _ := json.Marshal(expectedMap)

			if string(actualNorm) != string(expectedNorm) {
				t.Errorf("golden mismatch for %s\n--- expected ---\n%s\n--- actual ---\n%s",
					tt.goldenName, string(expected), string(actual))
			}
		})
	}
}

func TestGoldenMalformed(t *testing.T) {
	sseData := loadSSETestData(t, "malformed")
	resp := accumulateSSE(t, sseData)

	// Should have accumulated text despite malformed JSON lines
	if len(resp.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(resp.Content))
	}
	if resp.Content[0].Text != "before after" {
		t.Errorf("Content[0].Text = %q, want 'before after'", resp.Content[0].Text)
	}
}

func TestGoldenKeepalive(t *testing.T) {
	sseData := loadSSETestData(t, "keepalive")
	resp := accumulateSSE(t, sseData)

	if len(resp.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(resp.Content))
	}
	if resp.Content[0].Text != "alive" {
		t.Errorf("Content[0].Text = %q, want 'alive'", resp.Content[0].Text)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens = %d, want 10", resp.Usage.InputTokens)
	}
}
