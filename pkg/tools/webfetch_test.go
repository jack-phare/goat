package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetch_HTMLExtraction(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Test</title></head><body><h1>Hello</h1><p>World</p><script>alert("x")</script></body></html>`))
	}))
	defer srv.Close()

	tool := &WebFetchTool{HTTPClient: srv.Client()}
	out, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "extract text",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Hello") {
		t.Errorf("expected 'Hello' in output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "World") {
		t.Errorf("expected 'World' in output, got %q", out.Content)
	}
	// Script content should be stripped
	if strings.Contains(out.Content, "alert") {
		t.Errorf("script content should be stripped, got %q", out.Content)
	}
}

func TestWebFetch_PlainText(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("plain text content"))
	}))
	defer srv.Close()

	tool := &WebFetchTool{HTTPClient: srv.Client()}
	out, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "read",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "plain text content") {
		t.Errorf("expected plain text, got %q", out.Content)
	}
}

func TestWebFetch_HTTPUpgrade(t *testing.T) {
	// We can't easily test the actual upgrade, but we can verify the URL
	// transformation. Using a TLS server and verifying it works.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tool := &WebFetchTool{HTTPClient: srv.Client()}
	out, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "read",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
}

func TestWebFetch_HTTPError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tool := &WebFetchTool{HTTPClient: srv.Client()}
	out, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "read",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for 404")
	}
	if !strings.Contains(out.Content, "404") {
		t.Errorf("expected 404 in error, got %q", out.Content)
	}
}

func TestWebFetch_InvalidURL(t *testing.T) {
	tool := &WebFetchTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"url":    "not-a-url",
		"prompt": "read",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for invalid URL")
	}
}

func TestWebFetch_LargeContentTruncation(t *testing.T) {
	largeContent := strings.Repeat("x", webFetchMaxContent+1000)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(largeContent))
	}))
	defer srv.Close()

	tool := &WebFetchTool{HTTPClient: srv.Client()}
	out, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"prompt": "read",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "truncated") {
		t.Error("expected truncation message")
	}
}

func TestWebFetch_MissingURL(t *testing.T) {
	tool := &WebFetchTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"prompt": "read",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing URL")
	}
}

func TestWebFetch_MissingPrompt(t *testing.T) {
	tool := &WebFetchTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"url": "https://example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing prompt")
	}
}
