package transport

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"nhooyr.io/websocket"
)

// WebSocketTransport communicates via a WebSocket connection.
// Messages are sent as text frames containing JSON.
type WebSocketTransport struct {
	conn  *websocket.Conn
	ctx   context.Context

	inputCh   chan TransportMessage
	doneCh    chan struct{}
	ready     atomic.Bool
	writeMu   sync.Mutex
	closeOnce sync.Once
}

// NewWebSocketTransport wraps an existing WebSocket connection as a Transport.
// The ctx parameter is used for read/write operations on the connection.
func NewWebSocketTransport(ctx context.Context, conn *websocket.Conn) *WebSocketTransport {
	t := &WebSocketTransport{
		conn:    conn,
		ctx:     ctx,
		inputCh: make(chan TransportMessage, 64),
		doneCh:  make(chan struct{}),
	}
	t.ready.Store(true)

	go t.readLoop()

	return t
}

// readLoop reads WebSocket messages and sends them on inputCh.
func (t *WebSocketTransport) readLoop() {
	defer close(t.inputCh)

	for {
		_, data, err := t.conn.Read(t.ctx)
		if err != nil {
			// Check for normal closure
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway {
				return
			}
			// Unexpected error — signal it
			select {
			case t.inputCh <- TransportMessage{Type: TMsgError, Error: err}:
			case <-t.doneCh:
			}
			return
		}

		var msg TransportMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			select {
			case t.inputCh <- TransportMessage{Type: TMsgError, Error: err}:
			case <-t.doneCh:
				return
			}
			continue
		}

		select {
		case t.inputCh <- msg:
		case <-t.doneCh:
			return
		}
	}
}

// Write sends data as a text WebSocket message.
// Thread-safe via mutex.
func (t *WebSocketTransport) Write(data []byte) error {
	if !t.ready.Load() {
		return ErrTransportClosed
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	return t.conn.Write(t.ctx, websocket.MessageText, data)
}

// Close sends a close frame and shuts down the transport.
// Safe to call multiple times.
func (t *WebSocketTransport) Close() error {
	t.closeOnce.Do(func() {
		t.ready.Store(false)
		close(t.doneCh)
		t.conn.Close(websocket.StatusNormalClosure, "")
	})
	return nil
}

// IsReady returns true if the transport is accepting writes.
func (t *WebSocketTransport) IsReady() bool {
	return t.ready.Load()
}

// ReadMessages returns a channel of messages received from the WebSocket.
func (t *WebSocketTransport) ReadMessages() <-chan TransportMessage {
	return t.inputCh
}

// EndInput is a no-op for WebSocket — the connection handles its own lifecycle.
func (t *WebSocketTransport) EndInput() {}
