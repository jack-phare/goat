package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// flusherRecorder wraps httptest.ResponseRecorder with Flusher support.
type flusherRecorder struct {
	*httptest.ResponseRecorder
	flushCount int
}

func (f *flusherRecorder) Flush() {
	f.flushCount++
}

func newFlusherRecorder() *flusherRecorder {
	return &flusherRecorder{
		ResponseRecorder: httptest.NewRecorder(),
	}
}

func TestSSETransport_Headers(t *testing.T) {
	w := newFlusherRecorder()
	tr, err := NewSSETransport(w)
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}
	defer tr.Close()

	// Check SSE headers
	headers := w.Header()
	if got := headers.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if got := headers.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
	if got := headers.Get("Connection"); got != "keep-alive" {
		t.Errorf("Connection = %q, want keep-alive", got)
	}
}

func TestSSETransport_EventFormat(t *testing.T) {
	w := newFlusherRecorder()
	tr, err := NewSSETransport(w)
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}
	defer tr.Close()

	// Write an SSE event
	data := []byte(`{"result":"hello"}`)
	if err := tr.EmitSSE("custom_event", data); err != nil {
		t.Fatalf("EmitSSE: %v", err)
	}

	output := w.Body.String()
	expected := "event: custom_event\ndata: {\"result\":\"hello\"}\n\n"
	if output != expected {
		t.Errorf("SSE output = %q, want %q", output, expected)
	}

	if w.flushCount != 1 {
		t.Errorf("flush count = %d, want 1", w.flushCount)
	}
}

func TestSSETransport_WriteUsesMessageEvent(t *testing.T) {
	w := newFlusherRecorder()
	tr, err := NewSSETransport(w)
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}
	defer tr.Close()

	data := []byte(`{"type":"output"}`)
	if err := tr.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	output := w.Body.String()
	if !strings.HasPrefix(output, "event: message\n") {
		t.Errorf("expected 'event: message', got %q", output)
	}
}

func TestSSETransport_NonFlusherError(t *testing.T) {
	// Plain ResponseRecorder does NOT implement Flusher in our test
	// (we need an interface that doesn't have Flush)
	w := &nonFlusherWriter{}
	_, err := NewSSETransport(w)
	if err == nil {
		t.Error("expected error for non-Flusher ResponseWriter")
	}
}

// nonFlusherWriter is an http.ResponseWriter that does NOT implement http.Flusher.
type nonFlusherWriter struct {
	header http.Header
}

func (n *nonFlusherWriter) Header() http.Header {
	if n.header == nil {
		n.header = make(http.Header)
	}
	return n.header
}
func (n *nonFlusherWriter) Write(b []byte) (int, error) { return len(b), nil }
func (n *nonFlusherWriter) WriteHeader(int)             {}

func TestSSETransport_WriteAfterClose(t *testing.T) {
	w := newFlusherRecorder()
	tr, err := NewSSETransport(w)
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}

	tr.Close()

	if err := tr.Write([]byte("data")); err != ErrTransportClosed {
		t.Errorf("Write after close: err = %v, want ErrTransportClosed", err)
	}
}

func TestSSETransport_IsReady(t *testing.T) {
	w := newFlusherRecorder()
	tr, err := NewSSETransport(w)
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}

	if !tr.IsReady() {
		t.Error("expected IsReady() = true")
	}

	tr.Close()

	if tr.IsReady() {
		t.Error("expected IsReady() = false after Close()")
	}
}

func TestSSETransport_SendInput(t *testing.T) {
	w := newFlusherRecorder()
	tr, err := NewSSETransport(w)
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}
	defer tr.Close()

	msg := TransportMessage{Type: TMsgOutput}
	if err := tr.SendInput(msg); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	// Read from the input channel
	got := <-tr.ReadMessages()
	if got.Type != TMsgOutput {
		t.Errorf("Type = %q, want output", got.Type)
	}
}

func TestSSETransport_SendInputAfterClose(t *testing.T) {
	w := newFlusherRecorder()
	tr, err := NewSSETransport(w)
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}

	tr.Close()

	if err := tr.SendInput(TransportMessage{}); err != ErrTransportClosed {
		t.Errorf("SendInput after close: err = %v, want ErrTransportClosed", err)
	}
}

func TestSSETransport_MultipleEvents(t *testing.T) {
	w := newFlusherRecorder()
	tr, err := NewSSETransport(w)
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}
	defer tr.Close()

	tr.Write([]byte(`{"msg":1}`))
	tr.Write([]byte(`{"msg":2}`))
	tr.Write([]byte(`{"msg":3}`))

	output := w.Body.String()

	// Should have 3 events
	count := strings.Count(output, "event: message\n")
	if count != 3 {
		t.Errorf("event count = %d, want 3; output:\n%s", count, output)
	}
}

func TestSSETransport_EndInputIdempotent(t *testing.T) {
	w := newFlusherRecorder()
	tr, err := NewSSETransport(w)
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}
	defer tr.Close()

	// Should not panic on double EndInput
	tr.EndInput()
	tr.EndInput()
}

func TestSSETransport_InterfaceCompliance(t *testing.T) {
	var _ Transport = (*SSETransport)(nil)
}
