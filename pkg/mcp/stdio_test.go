package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// testHelperScript creates a small Go program that acts as an MCP echo server.
// It reads JSON-RPC requests from stdin and echoes them back as responses.
func testHelperScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "echo_server.go")
	os.WriteFile(script, []byte(`package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type Request struct {
	JSONRPC string          `+"`"+`json:"jsonrpc"`+"`"+`
	ID      *int            `+"`"+`json:"id,omitempty"`+"`"+`
	Method  string          `+"`"+`json:"method"`+"`"+`
	Params  json.RawMessage `+"`"+`json:"params,omitempty"`+"`"+`
}

type Response struct {
	JSONRPC string          `+"`"+`json:"jsonrpc"`+"`"+`
	ID      int             `+"`"+`json:"id"`+"`"+`
	Result  json.RawMessage `+"`"+`json:"result,omitempty"`+"`"+`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		// Notifications have no ID, don't respond
		if req.ID == nil {
			continue
		}

		var result json.RawMessage
		switch req.Method {
		case "initialize":
			result = json.RawMessage(` + "`" + `{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"echo","version":"1.0"}}` + "`" + `)
		case "tools/list":
			result = json.RawMessage(` + "`" + `{"tools":[{"name":"echo","description":"Echoes input"}]}` + "`" + `)
		case "tools/call":
			result = json.RawMessage(` + "`" + `{"content":[{"type":"text","text":"echoed"}],"isError":false}` + "`" + `)
		default:
			result = json.RawMessage(` + "`" + `{}` + "`" + `)
		}

		resp := Response{JSONRPC: "2.0", ID: *req.ID, Result: result}
		data, _ := json.Marshal(resp)
		fmt.Fprintln(os.Stdout, string(data))
	}
}
`), 0644)
	return script
}

func TestStdioTransport_SendReceive(t *testing.T) {
	script := testHelperScript(t)

	transport, err := NewStdioTransport("go", []string{"run", script}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := newRequest(1, "initialize", InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    ClientCapabilities{},
		ClientInfo:      ClientInfo{Name: "test", Version: "0.1"},
	})

	resp, err := transport.Send(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != 1 {
		t.Errorf("expected id 1, got %d", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.ServerInfo.Name != "echo" {
		t.Errorf("expected server name 'echo', got %q", result.ServerInfo.Name)
	}
}

func TestStdioTransport_ConcurrentSends(t *testing.T) {
	script := testHelperScript(t)

	transport, err := NewStdioTransport("go", []string{"run", script}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	const n = 5
	var wg sync.WaitGroup
	errors := make([]error, n)

	for i := range n {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			req := newRequest(id+100, "tools/list", nil)
			resp, err := transport.Send(ctx, req)
			if err != nil {
				errors[id] = err
				return
			}
			if resp.ID != id+100 {
				errors[id] = fmt.Errorf("expected id %d, got %d", id+100, resp.ID)
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

func TestStdioTransport_Notify(t *testing.T) {
	script := testHelperScript(t)

	transport, err := NewStdioTransport("go", []string{"run", script}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	ctx := context.Background()
	// Notification should succeed without waiting for response
	if err := transport.Notify(ctx, "notifications/initialized", nil); err != nil {
		t.Fatal(err)
	}
}

func TestStdioTransport_ContextCancellation(t *testing.T) {
	script := testHelperScript(t)

	transport, err := NewStdioTransport("go", []string{"run", script}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Use a high ID unlikely to be answered before cancel
	req := newRequest(9999, "initialize", nil)
	_, err = transport.Send(ctx, req)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestStdioTransport_Close(t *testing.T) {
	script := testHelperScript(t)

	transport, err := NewStdioTransport("go", []string{"run", script}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Do a round trip first to confirm it works
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := transport.Send(ctx, newRequest(1, "tools/list", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatal("unexpected error")
	}

	// Now close
	if err := transport.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}
}

func TestStdioTransport_ProcessCrash(t *testing.T) {
	// Use a command that exits immediately
	transport, err := NewStdioTransport("echo", []string{""}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	// Wait a bit for process to exit
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = transport.Send(ctx, newRequest(1, "initialize", nil))
	if err == nil {
		t.Error("expected error from crashed process")
	}
}

func TestStdioTransport_EnvVars(t *testing.T) {
	// Create a script that outputs the env var
	dir := t.TempDir()
	script := filepath.Join(dir, "env_check.go")
	os.WriteFile(script, []byte(`package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var raw map[string]json.RawMessage
		json.Unmarshal(scanner.Bytes(), &raw)

		idRaw := raw["id"]
		var id int
		json.Unmarshal(idRaw, &id)

		val := os.Getenv("MCP_TEST_VAR")
		result, _ := json.Marshal(map[string]string{"value": val})
		resp := map[string]any{"jsonrpc": "2.0", "id": id, "result": json.RawMessage(result)}
		data, _ := json.Marshal(resp)
		fmt.Fprintln(os.Stdout, string(data))
	}
}
`), 0644)

	transport, err := NewStdioTransport("go", []string{"run", script}, map[string]string{
		"MCP_TEST_VAR": "hello_mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := transport.Send(ctx, newRequest(1, "test", nil))
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result["value"] != "hello_mcp" {
		t.Errorf("expected 'hello_mcp', got %q", result["value"])
	}
}

func TestStdioTransport_SendRequiresID(t *testing.T) {
	script := testHelperScript(t)

	transport, err := NewStdioTransport("go", []string{"run", script}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	req := JSONRPCRequest{JSONRPC: "2.0", Method: "test"} // no ID
	_, err = transport.Send(context.Background(), req)
	if err == nil {
		t.Error("expected error for request without ID")
	}
}

func TestStdioTransport_MultipleRequestMethods(t *testing.T) {
	script := testHelperScript(t)

	transport, err := NewStdioTransport("go", []string{"run", script}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// initialize
	resp, err := transport.Send(ctx, newRequest(1, "initialize", nil))
	if err != nil {
		t.Fatal(err)
	}
	var initResult InitializeResult
	json.Unmarshal(resp.Result, &initResult)
	if initResult.ServerInfo.Name != "echo" {
		t.Errorf("init: got server %q", initResult.ServerInfo.Name)
	}

	// tools/list
	resp, err = transport.Send(ctx, newRequest(2, "tools/list", nil))
	if err != nil {
		t.Fatal(err)
	}
	var toolsResult ToolsListResult
	json.Unmarshal(resp.Result, &toolsResult)
	if len(toolsResult.Tools) != 1 {
		t.Errorf("tools/list: got %d tools", len(toolsResult.Tools))
	}

	// tools/call
	resp, err = transport.Send(ctx, newRequest(3, "tools/call", ToolCallParams{Name: "echo"}))
	if err != nil {
		t.Fatal(err)
	}
	var toolResult ToolResult
	json.Unmarshal(resp.Result, &toolResult)
	if len(toolResult.Content) == 0 || toolResult.Content[0].Text != "echoed" {
		t.Errorf("tools/call: unexpected result %+v", toolResult)
	}
}
