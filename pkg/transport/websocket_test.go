package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func TestWebSocketTransport_RoundTrip(t *testing.T) {
	// Set up a WebSocket server using httptest
	var serverTransport *WebSocketTransport
	serverReady := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket accept: %v", err)
			return
		}
		serverTransport = NewWebSocketTransport(r.Context(), conn)
		close(serverReady)

		// Keep the handler alive while the test runs
		<-serverTransport.doneCh
	}))
	defer srv.Close()

	// Connect as client
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}

	// Wait for server transport to be ready
	select {
	case <-serverReady:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server transport")
	}

	// Client → Server: send a message
	msg := TransportMessage{
		Type:    TMsgOutput,
		Payload: json.RawMessage(`"hello from client"`),
	}
	data, _ := json.Marshal(msg)
	if err := clientConn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("client write: %v", err)
	}

	// Read on server side
	select {
	case got := <-serverTransport.ReadMessages():
		if got.Type != TMsgOutput {
			t.Errorf("server got Type = %q, want output", got.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout reading on server")
	}

	// Server → Client: send a response
	resp := []byte(`{"type":"output","payload":"hello from server"}`)
	if err := serverTransport.Write(resp); err != nil {
		t.Fatalf("server write: %v", err)
	}

	// Read on client side
	_, clientData, err := clientConn.Read(ctx)
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if !strings.Contains(string(clientData), "hello from server") {
		t.Errorf("client got %q, want 'hello from server'", clientData)
	}

	serverTransport.Close()
	clientConn.Close(websocket.StatusNormalClosure, "")
}

func TestWebSocketTransport_ClientDisconnect(t *testing.T) {
	var serverTransport *WebSocketTransport
	serverReady := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		serverTransport = NewWebSocketTransport(r.Context(), conn)
		close(serverReady)
		<-serverTransport.doneCh
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx := context.Background()
	clientConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	<-serverReady

	// Close client abruptly
	clientConn.Close(websocket.StatusGoingAway, "bye")

	// Server should detect the disconnect (input channel closes)
	select {
	case _, ok := <-serverTransport.ReadMessages():
		// We might get an error message or closed channel — both are fine
		_ = ok
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: server should detect client disconnect")
	}

	serverTransport.Close()
}

func TestWebSocketTransport_ServerClose(t *testing.T) {
	var serverTransport *WebSocketTransport
	serverReady := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		serverTransport = NewWebSocketTransport(r.Context(), conn)
		close(serverReady)
		<-serverTransport.doneCh
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx := context.Background()
	clientConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	<-serverReady

	// Close server transport — should send close frame
	serverTransport.Close()

	if serverTransport.IsReady() {
		t.Error("expected IsReady() = false after Close()")
	}

	// Write after close should fail
	err = serverTransport.Write([]byte("data"))
	if err != ErrTransportClosed {
		t.Errorf("Write after close: err = %v, want ErrTransportClosed", err)
	}

	clientConn.Close(websocket.StatusNormalClosure, "")
}

func TestWebSocketTransport_ConcurrentWrites(t *testing.T) {
	var serverTransport *WebSocketTransport
	serverReady := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		serverTransport = NewWebSocketTransport(r.Context(), conn)
		close(serverReady)
		<-serverTransport.doneCh
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx := context.Background()
	clientConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	<-serverReady

	// Read all messages on client side
	received := 0
	var mu sync.Mutex
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		for {
			_, _, err := clientConn.Read(ctx)
			if err != nil {
				return
			}
			mu.Lock()
			received++
			mu.Unlock()
		}
	}()

	// Concurrent writes from server
	const numWriters = 5
	const msgsPerWriter = 20

	var wg sync.WaitGroup
	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < msgsPerWriter; j++ {
				data := []byte(`{"type":"output","payload":"msg"}`)
				serverTransport.Write(data)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	serverTransport.Close()
	clientConn.Close(websocket.StatusNormalClosure, "")

	<-clientDone

	mu.Lock()
	total := received
	mu.Unlock()

	if total != numWriters*msgsPerWriter {
		t.Errorf("received %d messages, want %d", total, numWriters*msgsPerWriter)
	}
}

func TestWebSocketTransport_InterfaceCompliance(t *testing.T) {
	var _ Transport = (*WebSocketTransport)(nil)
}
