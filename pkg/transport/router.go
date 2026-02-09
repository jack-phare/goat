package transport

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

// InputMessage is the envelope for messages sent from the consumer to the agent
// through a transport. It is parsed from the transport's ReadMessages channel.
type InputMessage struct {
	Type    string          `json:"type"`    // "user_message" | "control_request"
	Payload json.RawMessage `json:"payload"` // raw content for the specific type
}

// Router connects a Transport to a Query with bidirectional message routing.
// It runs two goroutines: an input pump (transport → query) and an output
// pump (query → transport).
type Router struct {
	transport Transport
	query     *agent.Query
}

// NewRouter creates a new Router connecting the given transport to the query.
func NewRouter(transport Transport, query *agent.Query) *Router {
	return &Router{
		transport: transport,
		query:     query,
	}
}

// Run starts the bidirectional message pump and blocks until both sides are done.
// The input pump reads from the transport and dispatches to the query.
// The output pump reads from the query and writes to the transport.
// When either side closes, the other is cleaned up.
func (r *Router) Run() error {
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	// Output pump: query → transport
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := r.outputPump()
		if err != nil {
			errCh <- fmt.Errorf("output pump: %w", err)
		}
		// When output pump finishes (query done), close transport
		r.transport.Close()
	}()

	// Input pump: transport → query
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := r.inputPump()
		if err != nil {
			errCh <- fmt.Errorf("input pump: %w", err)
		}
		// When input pump finishes (transport closed/EOF), close query
		r.query.Close()
	}()

	wg.Wait()
	close(errCh)

	// Return first error, if any
	for err := range errCh {
		return err
	}
	return nil
}

// outputPump reads SDKMessages from the query and writes them to the transport.
func (r *Router) outputPump() error {
	for msg := range r.query.Messages() {
		data, err := json.Marshal(msg)
		if err != nil {
			continue // skip unserializable messages
		}
		if err := r.transport.Write(data); err != nil {
			if err == ErrTransportClosed {
				return nil // normal shutdown
			}
			return err
		}
	}
	return nil
}

// inputPump reads TransportMessages from the transport and dispatches them to the query.
func (r *Router) inputPump() error {
	for msg := range r.transport.ReadMessages() {
		if msg.Error != nil {
			continue // skip error messages
		}

		if err := r.dispatchInput(msg); err != nil {
			// Write error response back to transport
			errResp := TransportMessage{
				Type:    TMsgError,
				Payload: json.RawMessage(fmt.Sprintf(`{"error":%q}`, err.Error())),
			}
			data, _ := json.Marshal(errResp)
			r.transport.Write(data)
		}
	}
	return nil
}

// dispatchInput parses a transport message and routes it to the query.
func (r *Router) dispatchInput(msg TransportMessage) error {
	// Try to parse the payload as an InputMessage
	var input InputMessage
	if err := json.Unmarshal(msg.Payload, &input); err != nil {
		// Treat the raw payload as a user message
		return r.query.SendUserMessage(msg.Payload)
	}

	switch input.Type {
	case "user_message":
		return r.query.SendUserMessage(input.Payload)

	case "control_request":
		var req types.ControlRequest
		if err := json.Unmarshal(input.Payload, &req); err != nil {
			return fmt.Errorf("invalid control request: %w", err)
		}
		resp, err := r.query.SendControl(req)
		if err != nil {
			return err
		}
		// Write control response back to transport
		respData, _ := json.Marshal(TransportMessage{
			Type:    TMsgControlResponse,
			Payload: mustMarshal(resp),
		})
		return r.transport.Write(respData)

	default:
		// Treat unknown types as raw user messages
		return r.query.SendUserMessage(msg.Payload)
	}
}

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
