package tools

import (
	"context"
	"fmt"
	"strings"
)

// SearchOptions configures domain filtering for web search.
type SearchOptions struct {
	AllowedDomains []string
	BlockedDomains []string
}

// SearchResult represents a single web search result.
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// SearchProvider executes web searches.
type SearchProvider interface {
	Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
}

// StubSearchProvider returns a helpful message when no real provider is configured.
type StubSearchProvider struct{}

func (s *StubSearchProvider) Search(_ context.Context, _ string, _ SearchOptions) ([]SearchResult, error) {
	return nil, fmt.Errorf("web search not configured. Set a SearchProvider on the WebSearchTool")
}

// WebSearchTool performs web searches via a configurable provider.
type WebSearchTool struct {
	Provider SearchProvider
}

func (w *WebSearchTool) Name() string { return "WebSearch" }

func (w *WebSearchTool) Description() string {
	return "Searches the web and returns results to inform responses."
}

func (w *WebSearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query",
			},
			"allowed_domains": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Only include results from these domains",
			},
			"blocked_domains": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Exclude results from these domains",
			},
		},
		"required": []string{"query"},
	}
}

func (w *WebSearchTool) SideEffect() SideEffectType { return SideEffectNetwork }

func (w *WebSearchTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return ToolOutput{Content: "Error: query is required", IsError: true}, nil
	}

	provider := w.Provider
	if provider == nil {
		provider = &StubSearchProvider{}
	}

	opts := SearchOptions{}
	if domains, ok := input["allowed_domains"].([]any); ok {
		for _, d := range domains {
			if s, ok := d.(string); ok {
				opts.AllowedDomains = append(opts.AllowedDomains, s)
			}
		}
	}
	if domains, ok := input["blocked_domains"].([]any); ok {
		for _, d := range domains {
			if s, ok := d.(string); ok {
				opts.BlockedDomains = append(opts.BlockedDomains, s)
			}
		}
	}

	results, err := provider.Search(ctx, query, opts)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error: %s", err),
			IsError: true,
		}, nil
	}

	if len(results) == 0 {
		return ToolOutput{Content: "No results found."}, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Search results for %q:\n\n", query)
	for i, r := range results {
		fmt.Fprintf(&b, "%d. **%s**\n   %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet)
		if i < len(results)-1 {
			b.WriteByte('\n')
		}
	}

	return ToolOutput{Content: b.String()}, nil
}
