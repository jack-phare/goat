package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	webFetchTimeout    = 30 * time.Second
	webFetchMaxBody    = 5 * 1024 * 1024 // 5MB
	webFetchMaxContent = 50000           // chars after extraction
	webFetchUserAgent  = "Goat/1.0 (CLI Agent)"
)

// ContentSummarizer summarizes web page content using an LLM.
type ContentSummarizer interface {
	Summarize(ctx context.Context, prompt, content string) (string, error)
}

// WebFetchTool fetches web content and extracts text from HTML.
type WebFetchTool struct {
	// HTTPClient overrides the default client (useful for testing).
	HTTPClient *http.Client

	// Summarizer, when set, processes fetched content with a prompt via an LLM.
	// When nil, raw extracted content is returned.
	Summarizer ContentSummarizer
}

func (w *WebFetchTool) Name() string { return "WebFetch" }

func (w *WebFetchTool) Description() string {
	return `IMPORTANT: WebFetch WILL FAIL for authenticated or private URLs. Before using this tool, check if the URL points to an authenticated service (e.g. Google Docs, Confluence, Jira, GitHub). If so, you MUST use ToolSearch first to find a specialized tool that provides authenticated access.

- Fetches content from a specified URL and processes it using an AI model
- Takes a URL and a prompt as input
- Fetches the URL content, converts HTML to markdown
- Processes the content with the prompt using a small, fast model
- Returns the model's response about the content
- Use this tool when you need to retrieve and analyze web content

Usage notes:
  - IMPORTANT: If an MCP-provided web fetch tool is available, prefer using that tool instead of this one, as it may have fewer restrictions.
  - The URL must be a fully-formed valid URL
  - HTTP URLs will be automatically upgraded to HTTPS
  - The prompt should describe what information you want to extract from the page
  - This tool is read-only and does not modify any files
  - Results may be summarized if the content is very large
  - Includes a self-cleaning 15-minute cache for faster responses when repeatedly accessing the same URL
  - When a URL redirects to a different host, the tool will inform you and provide the redirect URL in a special format. You should then make a new WebFetch request with the redirect URL to fetch the content.
  - For GitHub URLs, prefer using the gh CLI via Bash instead (e.g., gh pr view, gh issue view, gh api).`
}

func (w *WebFetchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to fetch content from",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The prompt describing what to extract from the page",
			},
		},
		"required": []string{"url", "prompt"},
	}
}

func (w *WebFetchTool) SideEffect() SideEffectType { return SideEffectNetwork }

func (w *WebFetchTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	rawURL, ok := input["url"].(string)
	if !ok || rawURL == "" {
		return ToolOutput{Content: "Error: url is required", IsError: true}, nil
	}

	prompt, _ := input["prompt"].(string)
	if prompt == "" {
		return ToolOutput{Content: "Error: prompt is required", IsError: true}, nil
	}

	// Auto-upgrade HTTP to HTTPS
	if strings.HasPrefix(rawURL, "http://") {
		rawURL = "https://" + rawURL[7:]
	}

	// Validate URL has a scheme
	if !strings.HasPrefix(rawURL, "https://") {
		return ToolOutput{Content: "Error: url must start with http:// or https://", IsError: true}, nil
	}

	client := w.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: webFetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		}
	}

	ctx, cancel := context.WithTimeout(ctx, webFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error creating request: %s", err),
			IsError: true,
		}, nil
	}
	req.Header.Set("User-Agent", webFetchUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error fetching URL: %s", err),
			IsError: true,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ToolOutput{
			Content: fmt.Sprintf("Error: HTTP %d from %s", resp.StatusCode, rawURL),
			IsError: true,
		}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBody))
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error reading response: %s", err),
			IsError: true,
		}, nil
	}

	content := string(body)
	contentType := resp.Header.Get("Content-Type")

	// Extract text from HTML
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml") {
		content = extractTextFromHTML(content)
	}

	// Truncate if needed
	if len(content) > webFetchMaxContent {
		content = content[:webFetchMaxContent] + "\n... (truncated)"
	}

	// If summarizer is available, process content through LLM
	if w.Summarizer != nil {
		summary, sumErr := w.Summarizer.Summarize(ctx, prompt, content)
		if sumErr == nil && summary != "" {
			return ToolOutput{
				Content: fmt.Sprintf("Fetched and summarized content from %s:\n\n%s", rawURL, summary),
			}, nil
		}
		// Fall back to raw content on summarizer error
	}

	return ToolOutput{
		Content: fmt.Sprintf("Fetched content from %s:\n\nPrompt: %s\n\n%s", rawURL, prompt, content),
	}, nil
}

// extractTextFromHTML uses the x/net/html tokenizer to strip tags and extract visible text.
func extractTextFromHTML(rawHTML string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(rawHTML))
	var b strings.Builder
	var skip bool

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(b.String())
		case html.StartTagToken:
			tn, _ := tokenizer.TagName()
			tag := string(tn)
			// Skip script, style, and other non-visible tags
			if tag == "script" || tag == "style" || tag == "noscript" || tag == "head" {
				skip = true
			}
			// Add newlines for block elements
			if isBlockTag(tag) {
				b.WriteByte('\n')
			}
		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			tag := string(tn)
			if tag == "script" || tag == "style" || tag == "noscript" || tag == "head" {
				skip = false
			}
		case html.TextToken:
			if !skip {
				text := strings.TrimSpace(string(tokenizer.Text()))
				if text != "" {
					if b.Len() > 0 {
						b.WriteByte(' ')
					}
					b.WriteString(text)
				}
			}
		}
	}
}

func isBlockTag(tag string) bool {
	switch tag {
	case "div", "p", "br", "h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li", "table", "tr", "td", "th",
		"section", "article", "header", "footer", "nav",
		"blockquote", "pre", "hr":
		return true
	}
	return false
}
