package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// HTTPTransport communicates with an MCP server via Streamable HTTP.
// Each JSON-RPC request is sent as an HTTP POST; the response may be
// immediate JSON or an SSE stream.
type HTTPTransport struct {
	url       string
	headers   map[string]string
	client    *http.Client
	sessionID string // Mcp-Session-Id from server
	mu        sync.Mutex
}

// NewHTTPTransport creates an HTTP transport for the given URL with optional custom headers.
func NewHTTPTransport(url string, headers map[string]string) *HTTPTransport {
	return &HTTPTransport{
		url:     url,
		headers: headers,
		client:  &http.Client{},
	}
}

// Send sends a JSON-RPC request via HTTP POST and returns the response.
// The response may come as immediate JSON or via an SSE stream.
func (t *HTTPTransport) Send(ctx context.Context, req JSONRPCRequest) (JSONRPCResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	// Add custom headers
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}

	// Add session ID if we have one
	t.mu.Lock()
	if t.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", t.sessionID)
	}
	t.mu.Unlock()

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	// Track session ID
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.mu.Lock()
		t.sessionID = sid
		t.mu.Unlock()
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return JSONRPCResponse{}, fmt.Errorf("http %d: %s", resp.StatusCode, string(bodyBytes))
	}

	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return t.parseSSEResponse(ctx, resp.Body, req.ID)
	}

	// Default: JSON response
	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return rpcResp, nil
}

// parseSSEResponse reads an SSE stream and extracts the JSON-RPC response matching the request ID.
func (t *HTTPTransport) parseSSEResponse(ctx context.Context, body io.Reader, reqID *int) (JSONRPCResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return JSONRPCResponse{}, ctx.Err()
		default:
		}

		line := scanner.Text()

		// Skip SSE comments and empty lines
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue // skip unparseable
		}

		// Match by ID if we have one
		if reqID != nil && resp.ID == *reqID {
			return resp, nil
		}
		// If no ID matching needed or first valid response
		if reqID == nil {
			return resp, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("sse stream: %w", err)
	}

	return JSONRPCResponse{}, fmt.Errorf("sse stream ended without matching response")
}

// Notify sends a JSON-RPC notification via HTTP POST (no response body expected).
func (t *HTTPTransport) Notify(ctx context.Context, method string, params any) error {
	n := newNotification(method, params)
	body, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}

	t.mu.Lock()
	if t.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", t.sessionID)
	}
	t.mu.Unlock()

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("http %d for notification", resp.StatusCode)
	}

	return nil
}

// Close is a no-op for HTTP transport (stateless per-request).
func (t *HTTPTransport) Close() error {
	return nil
}
