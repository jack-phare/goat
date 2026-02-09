package subagent

import (
	"strings"
	"sync"
	"time"

	"github.com/jg-phare/goat/pkg/types"
)

// AgentState represents the lifecycle state of a running agent.
type AgentState int

const (
	StateRunning   AgentState = iota
	StateCompleted
	StateFailed
	StateStopped
)

// String returns a human-readable label for the state.
func (s AgentState) String() string {
	switch s {
	case StateRunning:
		return "running"
	case StateCompleted:
		return "completed"
	case StateFailed:
		return "failed"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// TaskMetrics tracks resource usage for a subagent run.
type TaskMetrics struct {
	TokensUsed types.BetaUsage
	ToolUses   int
	Duration   time.Duration
	TurnCount  int
	CostUSD    float64
}

// TaskResult is the final output of a subagent execution.
type TaskResult struct {
	Content string
	Metrics TaskMetrics
	State   AgentState
	AgentID string
}

// AgentOutput is a thread-safe accumulator for streaming subagent output.
type AgentOutput struct {
	mu     sync.Mutex
	buf    strings.Builder
	result *TaskResult
}

// Append adds text to the accumulated output.
func (o *AgentOutput) Append(text string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.buf.WriteString(text)
}

// String returns the accumulated output so far.
func (o *AgentOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

// SetResult stores the final result.
func (o *AgentOutput) SetResult(r *TaskResult) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.result = r
}

// GetResult returns the final result (nil if not yet complete).
func (o *AgentOutput) GetResult() *TaskResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.result
}

// RunningAgent tracks a single executing subagent instance.
type RunningAgent struct {
	ID             string
	Type           string // agent type name
	Name           string // display name
	Definition     Definition
	State          AgentState
	StartedAt      time.Time
	Output         *AgentOutput
	OutputFile     string       // path to output file (background agents only)
	TranscriptPath string       // path to transcript file (when persistence is enabled)
	Cancel         func()       // cancel function to stop the agent
	Done           chan struct{} // closed when the agent finishes
	Metrics        TaskMetrics

	mu        sync.Mutex
	cleanupFn func() // called on completion to unregister hooks etc.
}

// SetState updates the agent's state in a thread-safe manner.
func (a *RunningAgent) SetState(state AgentState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.State = state
}

// GetState returns the agent's current state.
func (a *RunningAgent) GetState() AgentState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.State
}

// SetMetrics updates the metrics in a thread-safe manner.
func (a *RunningAgent) SetMetrics(m TaskMetrics) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Metrics = m
}

// Cleanup runs the registered cleanup function if any.
func (a *RunningAgent) Cleanup() {
	a.mu.Lock()
	fn := a.cleanupFn
	a.cleanupFn = nil
	a.mu.Unlock()
	if fn != nil {
		fn()
	}
}
