package transport

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStdioTransport_RoundTrip(t *testing.T) {
	// Use io.Pipe to simulate stdin/stdout
	inR, inW := io.Pipe()
	var outBuf bytes.Buffer

	tr := NewStdioTransport(inR, &outBuf)
	defer tr.Close()

	// Write a JSONL message to "stdin"
	msg := TransportMessage{
		Type:    TMsgOutput,
		Payload: json.RawMessage(`"hello world"`),
	}
	data, _ := json.Marshal(msg)
	inW.Write(append(data, '\n'))
	inW.Close() // signal EOF

	// Read from transport's input channel
	select {
	case got := <-tr.ReadMessages():
		if got.Type != TMsgOutput {
			t.Errorf("Type = %q, want output", got.Type)
		}
		if string(got.Payload) != `"hello world"` {
			t.Errorf("Payload = %q, want \"hello world\"", got.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	// Write from transport to "stdout"
	outMsg := []byte(`{"type":"output","payload":"response"}`)
	if err := tr.Write(outMsg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if !strings.Contains(outBuf.String(), `{"type":"output","payload":"response"}`) {
		t.Errorf("stdout output = %q, does not contain expected data", outBuf.String())
	}
}

func TestStdioTransport_LargeMessage(t *testing.T) {
	// Test a message > 1MB but within 10MB
	inR, inW := io.Pipe()
	var outBuf bytes.Buffer

	tr := NewStdioTransport(inR, &outBuf)
	defer tr.Close()

	// Build a large payload (~2MB)
	largePart := strings.Repeat("x", 2*1024*1024)
	msg := TransportMessage{
		Type:    TMsgOutput,
		Payload: json.RawMessage(`"` + largePart + `"`),
	}
	data, _ := json.Marshal(msg)
	inW.Write(append(data, '\n'))
	inW.Close()

	select {
	case got := <-tr.ReadMessages():
		if got.Type != TMsgOutput {
			t.Errorf("Type = %q, want output", got.Type)
		}
		if len(got.Payload) < 2*1024*1024 {
			t.Errorf("Payload len = %d, want >= 2MB", len(got.Payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for large message")
	}
}

func TestStdioTransport_EmptyLines(t *testing.T) {
	// Empty lines should be skipped
	input := "\n\n" + `{"type":"output","payload":"hello"}` + "\n\n"
	tr := NewStdioTransport(strings.NewReader(input), io.Discard)
	defer tr.Close()

	select {
	case got := <-tr.ReadMessages():
		if got.Type != TMsgOutput {
			t.Errorf("Type = %q, want output", got.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: empty lines should be skipped")
	}
}

func TestStdioTransport_MalformedJSON(t *testing.T) {
	// Malformed JSON should produce TMsgError
	input := `not valid json` + "\n"
	tr := NewStdioTransport(strings.NewReader(input), io.Discard)
	defer tr.Close()

	select {
	case got := <-tr.ReadMessages():
		if got.Type != TMsgError {
			t.Errorf("Type = %q, want error", got.Type)
		}
		if got.Error == nil {
			t.Error("expected non-nil Error for malformed JSON")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for error message")
	}
}

func TestStdioTransport_ScannerEOF(t *testing.T) {
	// When reader hits EOF, the input channel should close
	tr := NewStdioTransport(strings.NewReader(""), io.Discard)
	defer tr.Close()

	select {
	case _, ok := <-tr.ReadMessages():
		if ok {
			t.Error("expected channel to be closed on EOF")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: channel should close on EOF")
	}
}

func TestStdioTransport_ConcurrentWrites(t *testing.T) {
	var outBuf bytes.Buffer
	var outMu sync.Mutex

	// Use a mutex-protected writer to detect data races
	safeWriter := &syncWriter{w: &outBuf, mu: &outMu}

	tr := NewStdioTransport(strings.NewReader(""), safeWriter)
	defer tr.Close()

	const numWriters = 10
	const msgsPerWriter = 20

	var wg sync.WaitGroup
	wg.Add(numWriters)

	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < msgsPerWriter; j++ {
				data := []byte(`{"writer":` + string(rune('0'+id)) + `}`)
				tr.Write(data)
			}
		}(i)
	}

	wg.Wait()

	outMu.Lock()
	output := outBuf.String()
	outMu.Unlock()

	// Each write should produce a line
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != numWriters*msgsPerWriter {
		t.Errorf("output lines = %d, want %d", len(lines), numWriters*msgsPerWriter)
	}
}

func TestStdioTransport_WriteAfterClose(t *testing.T) {
	tr := NewStdioTransport(strings.NewReader(""), io.Discard)
	tr.Close()

	err := tr.Write([]byte("data"))
	if err != ErrTransportClosed {
		t.Errorf("Write after close: err = %v, want ErrTransportClosed", err)
	}
}

func TestStdioTransport_IsReady(t *testing.T) {
	tr := NewStdioTransport(strings.NewReader(""), io.Discard)

	if !tr.IsReady() {
		t.Error("expected IsReady() = true")
	}

	tr.Close()

	if tr.IsReady() {
		t.Error("expected IsReady() = false after Close()")
	}
}

func TestStdioTransport_EndInputIsNoOp(t *testing.T) {
	tr := NewStdioTransport(strings.NewReader(""), io.Discard)
	defer tr.Close()

	// EndInput should not panic
	tr.EndInput()
	tr.EndInput()
}

func TestStdioTransport_InterfaceCompliance(t *testing.T) {
	var _ Transport = (*StdioTransport)(nil)
}

func TestStdioTransport_MultipleMessages(t *testing.T) {
	lines := []string{
		`{"type":"output","payload":"msg1"}`,
		`{"type":"output","payload":"msg2"}`,
		`{"type":"output","payload":"msg3"}`,
	}
	input := strings.Join(lines, "\n") + "\n"

	tr := NewStdioTransport(strings.NewReader(input), io.Discard)
	defer tr.Close()

	received := 0
	for range tr.ReadMessages() {
		received++
	}

	if received != 3 {
		t.Errorf("received %d messages, want 3", received)
	}
}

// syncWriter is a thread-safe writer for testing.
type syncWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (s *syncWriter) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
