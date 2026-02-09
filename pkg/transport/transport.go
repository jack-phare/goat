// Package transport provides abstractions for communication between the agent
// runtime and external consumers (CLI, WebSocket, SSE, in-process).
package transport

import (
	"encoding/json"
	"errors"
)

// ErrTransportClosed is returned when operations are attempted on a closed transport.
var ErrTransportClosed = errors.New("transport closed")

// TransportMessageType identifies the type of a transport message.
type TransportMessageType string

const (
	TMsgOutput          TransportMessageType = "output"          // agent → consumer
	TMsgControlResponse TransportMessageType = "control_response" // control response
	TMsgError           TransportMessageType = "error"           // error notification
)

// TransportMessage is the envelope for all messages flowing through a Transport.
type TransportMessage struct {
	Type    TransportMessageType `json:"type"`
	Payload json.RawMessage      `json:"payload,omitempty"`
	Error   error                `json:"-"` // non-nil for TMsgError; not serialized
}

// Transport is the interface for bidirectional communication with the agent.
// Implementations include ChannelTransport (in-process), StdioTransport (JSONL),
// WebSocketTransport, and SSETransport.
type Transport interface {
	// Write sends data from the agent to the consumer.
	// Returns ErrTransportClosed if the transport has been closed.
	Write(data []byte) error

	// Close shuts down the transport. Safe to call multiple times.
	Close() error

	// IsReady returns true if the transport is accepting writes.
	IsReady() bool

	// ReadMessages returns a channel of messages from the consumer to the agent.
	// The channel is closed when no more input will arrive (EOF or close).
	ReadMessages() <-chan TransportMessage

	// EndInput signals that no more input will be sent from the consumer side.
	// For channel-based transports, this closes the input channel.
	// For stdio, this is a no-op (stdin EOF is handled by scanner termination).
	EndInput()
}
