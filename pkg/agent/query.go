package agent

import (
	"context"
	"sync"

	"github.com/jg-phare/goat/pkg/types"
)

// Query is the Go equivalent of the SDK's AsyncGenerator<SDKMessage>.
// Callers receive messages on the channel and can control execution via methods.
type Query struct {
	messages <-chan types.SDKMessage
	done     chan struct{}

	mu     sync.Mutex
	state  *LoopState
	cancel context.CancelFunc
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
