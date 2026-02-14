package main

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

func TestPrintTurnMeta(t *testing.T) {
	// Capture stderr output
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	m := types.ResultMessage{
		NumTurns:     3,
		TotalCostUSD: 0.001234,
	}
	printTurnMeta(m)

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	got := buf.String()

	expected := `{"turn":3,"cost_usd":0.001234}` + "\n"
	if got != expected {
		t.Errorf("printTurnMeta output:\n  got:  %q\n  want: %q", got, expected)
	}
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		name    string
		content []types.ContentBlock
		want    string
	}{
		{
			name: "text block",
			content: []types.ContentBlock{
				{Type: "text", Text: "hello world"},
			},
			want: "hello world",
		},
		{
			name: "first text block wins",
			content: []types.ContentBlock{
				{Type: "thinking", Text: "let me think"},
				{Type: "text", Text: "first"},
				{Type: "text", Text: "second"},
			},
			want: "first",
		},
		{
			name:    "no text blocks",
			content: []types.ContentBlock{{Type: "tool_use"}},
			want:    "",
		},
		{
			name:    "empty content",
			content: nil,
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := types.AssistantMessage{
				Message: types.BetaMessage{Content: tt.content},
			}
			got := extractText(m)
			if got != tt.want {
				t.Errorf("extractText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintTurnMeta_ZeroCost(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printTurnMeta(types.ResultMessage{NumTurns: 1, TotalCostUSD: 0})

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	got := buf.String()

	expected := fmt.Sprintf(`{"turn":1,"cost_usd":0.000000}` + "\n")
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}
