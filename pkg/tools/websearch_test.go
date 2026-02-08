package tools

import (
	"context"
	"strings"
	"testing"
)

type mockSearchProvider struct {
	results []SearchResult
	err     error
	opts    SearchOptions // captured for verification
}

func (m *mockSearchProvider) Search(_ context.Context, _ string, opts SearchOptions) ([]SearchResult, error) {
	m.opts = opts
	return m.results, m.err
}

func TestWebSearch_WithResults(t *testing.T) {
	provider := &mockSearchProvider{
		results: []SearchResult{
			{Title: "Go Docs", URL: "https://go.dev", Snippet: "The Go programming language"},
			{Title: "Go Wiki", URL: "https://go.dev/wiki", Snippet: "Community wiki"},
		},
	}
	tool := &WebSearchTool{Provider: provider}
	out, err := tool.Execute(context.Background(), map[string]any{
		"query": "golang",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Go Docs") {
		t.Errorf("expected 'Go Docs' in output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "https://go.dev") {
		t.Errorf("expected URL in output, got %q", out.Content)
	}
}

func TestWebSearch_StubProvider(t *testing.T) {
	tool := &WebSearchTool{} // no provider
	out, err := tool.Execute(context.Background(), map[string]any{
		"query": "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error from stub provider")
	}
	if !strings.Contains(out.Content, "not configured") {
		t.Errorf("expected 'not configured' message, got %q", out.Content)
	}
}

func TestWebSearch_AllowedDomains(t *testing.T) {
	provider := &mockSearchProvider{results: []SearchResult{}}
	tool := &WebSearchTool{Provider: provider}
	tool.Execute(context.Background(), map[string]any{
		"query":           "test",
		"allowed_domains": []any{"example.com", "go.dev"},
	})
	if len(provider.opts.AllowedDomains) != 2 {
		t.Errorf("expected 2 allowed domains, got %d", len(provider.opts.AllowedDomains))
	}
}

func TestWebSearch_BlockedDomains(t *testing.T) {
	provider := &mockSearchProvider{results: []SearchResult{}}
	tool := &WebSearchTool{Provider: provider}
	tool.Execute(context.Background(), map[string]any{
		"query":           "test",
		"blocked_domains": []any{"spam.com"},
	})
	if len(provider.opts.BlockedDomains) != 1 {
		t.Errorf("expected 1 blocked domain, got %d", len(provider.opts.BlockedDomains))
	}
}

func TestWebSearch_EmptyResults(t *testing.T) {
	provider := &mockSearchProvider{results: []SearchResult{}}
	tool := &WebSearchTool{Provider: provider}
	out, err := tool.Execute(context.Background(), map[string]any{
		"query": "obscure query",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "No results found") {
		t.Errorf("expected no results message, got %q", out.Content)
	}
}

func TestWebSearch_MissingQuery(t *testing.T) {
	tool := &WebSearchTool{}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing query")
	}
}
