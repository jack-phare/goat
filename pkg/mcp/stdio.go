package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// StdioTransport communicates with an MCP server via stdin/stdout of a spawned process.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *bytes.Buffer

	writeMu sync.Mutex // serializes writes to stdin

	pending map[int]chan JSONRPCResponse
	pendMu  sync.Mutex

	done chan struct{} // closed when reader goroutine exits
}

// NewStdioTransport spawns a child process and returns a transport that communicates
// via JSON-RPC over its stdin/stdout. The process inherits the parent environment
// plus any additional env vars specified.
func NewStdioTransport(command string, args []string, env map[string]string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)

	// Inherit parent env + user overrides
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	t := &StdioTransport{
		cmd:     cmd,
		stdin:   stdinPipe,
		stdout:  stdoutPipe,
		stderr:  &stderrBuf,
		pending: make(map[int]chan JSONRPCResponse),
		done:    make(chan struct{}),
	}

	go t.readLoop()

	return t, nil
}

// readLoop reads lines from stdout and dispatches JSON-RPC responses to pending channels.
func (t *StdioTransport) readLoop() {
	defer close(t.done)

	scanner := bufio.NewScanner(t.stdout)
	// Allow large JSON payloads (1 MB)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// Skip unparseable lines (could be log output from the server)
			continue
		}

		t.pendMu.Lock()
		ch, ok := t.pending[resp.ID]
		if ok {
			delete(t.pending, resp.ID)
		}
		t.pendMu.Unlock()

		if ok {
			ch <- resp
		}
	}
}

// Send writes a JSON-RPC request to stdin and waits for the correlated response.
func (t *StdioTransport) Send(ctx context.Context, req JSONRPCRequest) (JSONRPCResponse, error) {
	if req.ID == nil {
		return JSONRPCResponse{}, fmt.Errorf("Send requires a request with an ID; use Notify for notifications")
	}
	id := *req.ID

	// Register pending channel before writing to avoid race
	ch := make(chan JSONRPCResponse, 1)
	t.pendMu.Lock()
	t.pending[id] = ch
	t.pendMu.Unlock()

	// Write request to stdin
	data, err := json.Marshal(req)
	if err != nil {
		t.pendMu.Lock()
		delete(t.pending, id)
		t.pendMu.Unlock()
		return JSONRPCResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	t.writeMu.Lock()
	_, writeErr := t.stdin.Write(append(data, '\n'))
	t.writeMu.Unlock()

	if writeErr != nil {
		t.pendMu.Lock()
		delete(t.pending, id)
		t.pendMu.Unlock()
		return JSONRPCResponse{}, fmt.Errorf("write to stdin: %w", writeErr)
	}

	// Wait for response or context cancellation
	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		t.pendMu.Lock()
		delete(t.pending, id)
		t.pendMu.Unlock()
		return JSONRPCResponse{}, ctx.Err()
	case <-t.done:
		t.pendMu.Lock()
		delete(t.pending, id)
		t.pendMu.Unlock()
		return JSONRPCResponse{}, fmt.Errorf("transport closed: %s", t.stderr.String())
	}
}

// Notify writes a JSON-RPC notification (no ID, no response expected).
func (t *StdioTransport) Notify(_ context.Context, method string, params any) error {
	n := newNotification(method, params)
	data, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	_, err = t.stdin.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("write notification: %w", err)
	}
	return nil
}

// Close terminates the child process: close stdin, SIGTERM, wait with timeout, SIGKILL.
func (t *StdioTransport) Close() error {
	// Close stdin to signal the child process
	t.stdin.Close()

	// Try graceful shutdown with SIGTERM
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Signal(syscall.SIGTERM)
	}

	// Wait with a 5-second timeout
	done := make(chan error, 1)
	go func() {
		done <- t.cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited
	case <-time.After(5 * time.Second):
		// Force kill
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		<-done
	}

	// Wait for reader to finish
	<-t.done

	return nil
}
