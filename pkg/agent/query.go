package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// ErrQueryClosed is returned when operations are attempted on a closed Query.
var ErrQueryClosed = errors.New("query closed")

// Query is the Go equivalent of the SDK's AsyncGenerator<SDKMessage>.
// Callers receive messages on the channel and can control execution via methods.
//
// In multi-turn mode (MultiTurn=true on AgentConfig), the loop waits for
// additional user input after each end_turn instead of exiting. Use
// SendUserMessage to inject follow-up messages and Close to terminate.
type Query struct {
	messages <-chan types.SDKMessage
	done     chan struct{}

	// Multi-turn channels (nil in one-shot mode)
	inputCh     chan []byte               // buffered channel for injecting user messages
	controlCh   chan types.ControlRequest  // channel for control commands
	controlResp chan types.ControlResponse // response channel for control commands
	closeCh     chan struct{}              // explicit close signal

	mu          sync.Mutex
	state       *LoopState
	costTracker *llm.CostTracker
	cancel      context.CancelFunc
	closed      bool
}

// Messages returns the channel of SDKMessages emitted by the loop.
func (q *Query) Messages() <-chan types.SDKMessage {
	return q.messages
}

// Wait blocks until the loop completes.
func (q *Query) Wait() {
	<-q.done
}

// Interrupt requests the loop to stop after the current operation.
func (q *Query) Interrupt() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.state.IsInterrupted = true
	q.cancel()
	return nil
}

// SendUserMessage injects a follow-up user message into the loop.
// Only works in multi-turn mode. Blocks if the input channel is full.
func (q *Query) SendUserMessage(data []byte) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return ErrQueryClosed
	}
	ch := q.inputCh
	q.mu.Unlock()

	if ch == nil {
		return errors.New("not in multi-turn mode")
	}

	select {
	case ch <- data:
		return nil
	case <-q.done:
		return ErrQueryClosed
	}
}

// SendControl dispatches a synchronous control request and waits for the response.
func (q *Query) SendControl(req types.ControlRequest) (types.ControlResponse, error) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return types.ControlResponse{}, ErrQueryClosed
	}
	ctrlCh := q.controlCh
	respCh := q.controlResp
	q.mu.Unlock()

	if ctrlCh == nil {
		return types.ControlResponse{}, errors.New("not in multi-turn mode")
	}

	// Send request
	select {
	case ctrlCh <- req:
	case <-q.done:
		return types.ControlResponse{}, ErrQueryClosed
	}

	// Wait for response
	select {
	case resp := <-respCh:
		return resp, nil
	case <-q.done:
		return types.ControlResponse{}, ErrQueryClosed
	}
}

// Close gracefully shuts down the loop. Safe to call multiple times.
func (q *Query) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil
	}
	q.closed = true
	if q.closeCh != nil {
		close(q.closeCh)
	}
	return nil
}

// SessionID returns the session identifier.
func (q *Query) SessionID() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state.SessionID
}

// TotalUsage returns accumulated token usage.
func (q *Query) TotalUsage() types.BetaUsage {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state.TotalUsage
}

// TotalCostUSD returns the accumulated cost in USD.
func (q *Query) TotalCostUSD() float64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state.TotalCostUSD
}

// TurnCount returns the number of LLM round-trips completed.
func (q *Query) TurnCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state.TurnCount
}

// ExitReason returns why the loop terminated (empty string if still running).
func (q *Query) GetExitReason() ExitReason {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state.ExitReason
}

// State returns the current loop state snapshot.
// The caller should not mutate the returned state.
func (q *Query) State() *LoopState {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state
}

// SetPermissionMode updates the permission mode at runtime (multi-turn only).
func (q *Query) SetPermissionMode(mode types.PermissionMode) (types.ControlResponse, error) {
	return q.SendControl(types.ControlRequest{
		RequestID: "set-mode",
		Request: types.ControlRequestInner{
			Subtype: types.ControlSubtypeSetPermissionMode,
			Mode:    mode,
		},
	})
}

// SetModel updates the LLM model at runtime (multi-turn only).
func (q *Query) SetModel(model string) (types.ControlResponse, error) {
	return q.SendControl(types.ControlRequest{
		RequestID: "set-model",
		Request: types.ControlRequestInner{
			Subtype: types.ControlSubtypeSetModel,
			Model:   model,
		},
	})
}

// SetMaxThinkingTokens updates the thinking token limit at runtime (multi-turn only).
func (q *Query) SetMaxThinkingTokens(tokens int) (types.ControlResponse, error) {
	return q.SendControl(types.ControlRequest{
		RequestID: "set-thinking",
		Request: types.ControlRequestInner{
			Subtype:           types.ControlSubtypeSetMaxThinkingTokens,
			MaxThinkingTokens: &tokens,
		},
	})
}

// ModelBreakdown returns per-model cost breakdown.
// Returns nil if no CostTracker is configured.
func (q *Query) ModelBreakdown() map[string]llm.ModelUsageAccum {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.costTracker == nil {
		return nil
	}
	return q.costTracker.ModelBreakdown()
}
