package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPTransport_JSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      *req.ID,
			Result:  json.RawMessage(`{"tools":[{"name":"search"}]}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL, nil)

	ctx := context.Background()
	resp, err := transport.Send(ctx, newRequest(1, "tools/list", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != 1 {
		t.Errorf("expected id 1, got %d", resp.ID)
	}

	var result ToolsListResult
	json.Unmarshal(resp.Result, &result)
	if len(result.Tools) != 1 || result.Tools[0].Name != "search" {
		t.Errorf("unexpected tools: %+v", result)
	}
}

func TestHTTPTransport_SSEResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send a comment line first (keep-alive)
		fmt.Fprintln(w, ": keep-alive")
		fmt.Fprintln(w)

		// Send the actual response
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      *req.ID,
			Result:  json.RawMessage(`{"content":[{"type":"text","text":"sse result"}]}`),
		}
		data, _ := json.Marshal(resp)
		fmt.Fprintf(w, "data: %s\n\n", string(data))
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL, nil)

	ctx := context.Background()
	resp, err := transport.Send(ctx, newRequest(42, "tools/call", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != 42 {
		t.Errorf("expected id 42, got %d", resp.ID)
	}

	var result ToolResult
	json.Unmarshal(resp.Result, &result)
	if len(result.Content) != 1 || result.Content[0].Text != "sse result" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestHTTPTransport_SessionID(t *testing.T) {
	var receivedSessionID string
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		receivedSessionID = r.Header.Get("Mcp-Session-Id")

		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Set session ID on first call
		w.Header().Set("Mcp-Session-Id", "test-session-123")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      *req.ID,
			Result:  json.RawMessage(`{}`),
		})
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL, nil)
	ctx := context.Background()

	// First call: no session ID sent
	transport.Send(ctx, newRequest(1, "initialize", nil))
	if receivedSessionID != "" {
		t.Error("first call should not have session ID")
	}

	// Second call: session ID should be sent
	transport.Send(ctx, newRequest(2, "tools/list", nil))
	if receivedSessionID != "test-session-123" {
		t.Errorf("expected session ID 'test-session-123', got %q", receivedSessionID)
	}
}

func TestHTTPTransport_CustomHeaders(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")

		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      *req.ID,
			Result:  json.RawMessage(`{}`),
		})
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL, map[string]string{
		"Authorization": "Bearer test-token",
	})

	ctx := context.Background()
	transport.Send(ctx, newRequest(1, "initialize", nil))

	if receivedAuth != "Bearer test-token" {
		t.Errorf("expected auth header, got %q", receivedAuth)
	}
}

func TestHTTPTransport_HTTPError(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"401 Unauthorized", http.StatusUnauthorized},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "error", tt.status)
			}))
			defer server.Close()

			transport := NewHTTPTransport(server.URL, nil)
			_, err := transport.Send(context.Background(), newRequest(1, "test", nil))
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestHTTPTransport_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Block until the server-side request context is done
		// (httptest closes connections when we call CloseClientConnections)
		<-r.Context().Done()
	}))

	transport := NewHTTPTransport(server.URL, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := transport.Send(ctx, newRequest(1, "test", nil))
	if err == nil {
		t.Error("expected error from context cancellation")
	}

	// Clean up: close client connections first so handler unblocks, then close server
	server.CloseClientConnections()
	server.Close()
}

func TestHTTPTransport_Notify(t *testing.T) {
	var receivedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedMethod = req.Method
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL, nil)

	err := transport.Notify(context.Background(), "notifications/initialized", nil)
	if err != nil {
		t.Fatal(err)
	}
	if receivedMethod != "notifications/initialized" {
		t.Errorf("expected method, got %q", receivedMethod)
	}
}

func TestHTTPTransport_Close(t *testing.T) {
	transport := NewHTTPTransport("http://localhost:0", nil)
	if err := transport.Close(); err != nil {
		t.Errorf("Close should be no-op, got %v", err)
	}
}

func TestHTTPTransport_LargeResponse(t *testing.T) {
	// Generate a large result
	largeText := make([]byte, 100*1024) // 100KB
	for i := range largeText {
		largeText[i] = 'x'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		result, _ := json.Marshal(map[string]string{"data": string(largeText)})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      *req.ID,
			Result:  json.RawMessage(result),
		})
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL, nil)
	resp, err := transport.Send(context.Background(), newRequest(1, "test", nil))
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]string
	json.Unmarshal(resp.Result, &result)
	if len(result["data"]) != 100*1024 {
		t.Errorf("expected 100KB data, got %d bytes", len(result["data"]))
	}
}

func TestHTTPTransport_SSEMultipleEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send an unrelated event first (different ID)
		other := JSONRPCResponse{JSONRPC: "2.0", ID: 999, Result: json.RawMessage(`{"ignored":true}`)}
		otherData, _ := json.Marshal(other)
		fmt.Fprintf(w, "data: %s\n\n", string(otherData))

		// Then the matching response
		resp := JSONRPCResponse{JSONRPC: "2.0", ID: *req.ID, Result: json.RawMessage(`{"matched":true}`)}
		data, _ := json.Marshal(resp)
		fmt.Fprintf(w, "data: %s\n\n", string(data))
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL, nil)
	resp, err := transport.Send(context.Background(), newRequest(7, "test", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != 7 {
		t.Errorf("expected matched response with id 7, got %d", resp.ID)
	}

	var result map[string]bool
	json.Unmarshal(resp.Result, &result)
	if !result["matched"] {
		t.Error("expected matched=true")
	}
}
