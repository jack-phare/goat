package transport

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

// SSETransport implements Server-Sent Events transport for HTTP streaming.
// SSE is inherently write-only (server → client). Input is received via a
// separate mechanism (e.g., POST endpoint) and injected through the input channel.
type SSETransport struct {
	writer  http.ResponseWriter
	flusher http.Flusher

	inputCh      chan TransportMessage
	doneCh       chan struct{}
	ready        atomic.Bool
	writeMu      sync.Mutex
	closeOnce    sync.Once
	endInputOnce sync.Once
}

// NewSSETransport creates an SSE transport from an http.ResponseWriter.
// Returns an error if the ResponseWriter does not implement http.Flusher.
// Sets the appropriate SSE headers on the response.
func NewSSETransport(w http.ResponseWriter) (*SSETransport, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("ResponseWriter does not implement http.Flusher")
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	t := &SSETransport{
		writer:  w,
		flusher: flusher,
		inputCh: make(chan TransportMessage, 64),
		doneCh:  make(chan struct{}),
	}
	t.ready.Store(true)

	return t, nil
}

// EmitSSE writes an SSE event in the standard format:
//
//	event: <event>\ndata: <data>\n\n
//
// Each call flushes the response writer.
func (t *SSETransport) EmitSSE(event string, data []byte) error {
	if !t.ready.Load() {
		return ErrTransportClosed
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	if _, err := fmt.Fprintf(t.writer, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	t.flusher.Flush()
	return nil
}

// Write sends data as an SSE "message" event.
func (t *SSETransport) Write(data []byte) error {
	return t.EmitSSE("message", data)
}

// Close shuts down the SSE transport. Safe to call multiple times.
func (t *SSETransport) Close() error {
	t.closeOnce.Do(func() {
		t.ready.Store(false)
		close(t.doneCh)
	})
	return nil
}

// IsReady returns true if the transport is accepting writes.
func (t *SSETransport) IsReady() bool {
	return t.ready.Load()
}

// ReadMessages returns the input channel. Since SSE is write-only,
// this channel must be fed externally (e.g., from a POST endpoint handler).
func (t *SSETransport) ReadMessages() <-chan TransportMessage {
	return t.inputCh
}

// SendInput injects a message into the input channel from an external source
// (e.g., a companion POST endpoint). This is the primary way to feed input
// to an SSE-based transport.
func (t *SSETransport) SendInput(msg TransportMessage) error {
	if !t.ready.Load() {
		return ErrTransportClosed
	}
	select {
	case t.inputCh <- msg:
		return nil
	case <-t.doneCh:
		return ErrTransportClosed
	}
}

// EndInput closes the input channel. Call this when the client signals
// no more input will be sent. Safe to call multiple times.
func (t *SSETransport) EndInput() {
	t.endInputOnce.Do(func() {
		close(t.inputCh)
	})
}
