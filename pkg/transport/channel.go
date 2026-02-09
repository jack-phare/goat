package transport

import (
	"sync"
	"sync/atomic"
)

// ChannelTransport is an in-process transport that uses Go channels for
// bidirectional communication. No serialization overhead — messages are
// passed directly as byte slices.
type ChannelTransport struct {
	inputCh      chan TransportMessage
	outputCh     chan []byte
	doneCh       chan struct{}
	ready        atomic.Bool
	closeOnce    sync.Once
	endInputOnce sync.Once
}

// NewChannelTransport creates a new in-process channel transport.
// bufferSize controls the capacity of both input and output channels.
func NewChannelTransport(bufferSize int) *ChannelTransport {
	if bufferSize <= 0 {
		bufferSize = 64
	}
	t := &ChannelTransport{
		inputCh:  make(chan TransportMessage, bufferSize),
		outputCh: make(chan []byte, bufferSize),
		doneCh:   make(chan struct{}),
	}
	t.ready.Store(true)
	return t
}

// Write sends data from the agent to the consumer.
func (t *ChannelTransport) Write(data []byte) error {
	if !t.ready.Load() {
		return ErrTransportClosed
	}
	select {
	case t.outputCh <- data:
		return nil
	case <-t.doneCh:
		return ErrTransportClosed
	}
}

// Close shuts down the transport. Safe to call multiple times.
func (t *ChannelTransport) Close() error {
	t.closeOnce.Do(func() {
		t.ready.Store(false)
		close(t.doneCh)
		// Also close input channel so ReadMessages consumers see EOF
		t.endInputOnce.Do(func() {
			close(t.inputCh)
		})
	})
	return nil
}

// IsReady returns true if the transport is accepting writes.
func (t *ChannelTransport) IsReady() bool {
	return t.ready.Load()
}

// ReadMessages returns a channel of messages from the consumer to the agent.
func (t *ChannelTransport) ReadMessages() <-chan TransportMessage {
	return t.inputCh
}

// EndInput signals that no more input will be sent. Closes the input channel.
// Safe to call multiple times.
func (t *ChannelTransport) EndInput() {
	t.endInputOnce.Do(func() {
		close(t.inputCh)
	})
}

// Send sends a message into the transport's input channel (consumer → agent).
// This is the consumer-side API for injecting messages.
func (t *ChannelTransport) Send(msg TransportMessage) error {
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

// Receive reads from the transport's output channel (agent → consumer).
// Returns the data and true, or nil and false if the channel is closed.
func (t *ChannelTransport) Receive() ([]byte, bool) {
	select {
	case data, ok := <-t.outputCh:
		return data, ok
	case <-t.doneCh:
		return nil, false
	}
}

// Output returns the raw output channel for consumers that need direct access.
func (t *ChannelTransport) Output() <-chan []byte {
	return t.outputCh
}
