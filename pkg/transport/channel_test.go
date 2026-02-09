package transport

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestChannelTransport_WriteRead(t *testing.T) {
	tr := NewChannelTransport(8)
	defer tr.Close()

	// Write from agent side
	data := []byte(`{"type":"output","payload":"hello"}`)
	if err := tr.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Read from consumer side
	got, ok := tr.Receive()
	if !ok {
		t.Fatal("Receive returned false")
	}
	if string(got) != string(data) {
		t.Errorf("Receive = %q, want %q", got, data)
	}
}

func TestChannelTransport_SendReadMessages(t *testing.T) {
	tr := NewChannelTransport(8)
	defer tr.Close()

	// Send from consumer side
	msg := TransportMessage{
		Type:    TMsgOutput,
		Payload: json.RawMessage(`"hello from consumer"`),
	}
	if err := tr.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Read from agent side
	ch := tr.ReadMessages()
	select {
	case got := <-ch:
		if got.Type != TMsgOutput {
			t.Errorf("Type = %q, want output", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestChannelTransport_IsReady(t *testing.T) {
	tr := NewChannelTransport(8)

	if !tr.IsReady() {
		t.Error("expected IsReady() = true after creation")
	}

	tr.Close()

	if tr.IsReady() {
		t.Error("expected IsReady() = false after Close()")
	}
}

func TestChannelTransport_WriteAfterClose(t *testing.T) {
	tr := NewChannelTransport(8)
	tr.Close()

	err := tr.Write([]byte("data"))
	if err != ErrTransportClosed {
		t.Errorf("Write after close: err = %v, want ErrTransportClosed", err)
	}
}

func TestChannelTransport_SendAfterClose(t *testing.T) {
	tr := NewChannelTransport(8)
	tr.Close()

	err := tr.Send(TransportMessage{Type: TMsgOutput})
	if err != ErrTransportClosed {
		t.Errorf("Send after close: err = %v, want ErrTransportClosed", err)
	}
}

func TestChannelTransport_EndInput(t *testing.T) {
	tr := NewChannelTransport(8)
	defer tr.Close()

	// End input
	tr.EndInput()

	// ReadMessages channel should be closed
	ch := tr.ReadMessages()
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed after EndInput()")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: channel should be closed")
	}
}

func TestChannelTransport_EndInputIdempotent(t *testing.T) {
	tr := NewChannelTransport(8)
	defer tr.Close()

	// Should not panic on double close
	tr.EndInput()
	tr.EndInput()
}

func TestChannelTransport_CloseIdempotent(t *testing.T) {
	tr := NewChannelTransport(8)

	if err := tr.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestChannelTransport_ConcurrentWrites(t *testing.T) {
	tr := NewChannelTransport(256)
	defer tr.Close()

	const numWriters = 10
	const msgsPerWriter = 50

	var wg sync.WaitGroup
	wg.Add(numWriters)

	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < msgsPerWriter; j++ {
				data := []byte(`{"writer":` + string(rune('0'+id)) + `}`)
				_ = tr.Write(data)
			}
		}(i)
	}

	// Drain output in background
	received := 0
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for range tr.Output() {
			received++
			if received >= numWriters*msgsPerWriter {
				return
			}
		}
	}()

	wg.Wait()

	// Wait for drain to complete
	select {
	case <-drainDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout draining: only received %d/%d", received, numWriters*msgsPerWriter)
	}

	if received != numWriters*msgsPerWriter {
		t.Errorf("received %d messages, want %d", received, numWriters*msgsPerWriter)
	}
}

func TestChannelTransport_ConcurrentSends(t *testing.T) {
	tr := NewChannelTransport(256)
	defer tr.Close()

	const numSenders = 10
	const msgsPerSender = 50

	var wg sync.WaitGroup
	wg.Add(numSenders)

	for i := 0; i < numSenders; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < msgsPerSender; j++ {
				_ = tr.Send(TransportMessage{Type: TMsgOutput})
			}
		}()
	}

	// Drain input
	received := 0
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for range tr.ReadMessages() {
			received++
			if received >= numSenders*msgsPerSender {
				return
			}
		}
	}()

	wg.Wait()

	select {
	case <-drainDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout draining: only received %d/%d", received, numSenders*msgsPerSender)
	}

	if received != numSenders*msgsPerSender {
		t.Errorf("received %d messages, want %d", received, numSenders*msgsPerSender)
	}
}

func TestChannelTransport_DefaultBufferSize(t *testing.T) {
	// Zero buffer size should default to 64
	tr := NewChannelTransport(0)
	defer tr.Close()

	if !tr.IsReady() {
		t.Error("expected IsReady() = true")
	}
}

func TestChannelTransport_ReceiveAfterClose(t *testing.T) {
	tr := NewChannelTransport(8)

	// Write data before close
	tr.Write([]byte("data"))

	tr.Close()

	// Should still be able to receive buffered data from output channel
	// (but doneCh is also closed, so Receive may return false)
	// This tests the close path doesn't panic
	_, _ = tr.Receive()
}

func TestChannelTransport_InterfaceCompliance(t *testing.T) {
	// Verify ChannelTransport implements Transport
	var _ Transport = (*ChannelTransport)(nil)
}
