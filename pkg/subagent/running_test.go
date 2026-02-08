package subagent

import (
	"sync"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/types"
)

func TestAgentOutput_Append(t *testing.T) {
	o := &AgentOutput{}
	o.Append("hello ")
	o.Append("world")
	if got := o.String(); got != "hello world" {
		t.Errorf("String() = %q, want 'hello world'", got)
	}
}

func TestAgentOutput_ConcurrentAppend(t *testing.T) {
	o := &AgentOutput{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o.Append("x")
		}()
	}
	wg.Wait()
	if got := len(o.String()); got != 100 {
		t.Errorf("length = %d, want 100", got)
	}
}

func TestAgentOutput_Result(t *testing.T) {
	o := &AgentOutput{}
	if r := o.GetResult(); r != nil {
		t.Error("expected nil result initially")
	}

	result := &TaskResult{
		Content: "done",
		State:   StateCompleted,
		AgentID: "agent-1",
	}
	o.SetResult(result)

	got := o.GetResult()
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Content != "done" {
		t.Errorf("Content = %q", got.Content)
	}
	if got.State != StateCompleted {
		t.Errorf("State = %v", got.State)
	}
}

func TestRunningAgent_StateTransitions(t *testing.T) {
	agent := &RunningAgent{
		ID:    "agent-1",
		State: StateRunning,
	}
	if agent.GetState() != StateRunning {
		t.Errorf("initial state = %v, want Running", agent.GetState())
	}

	agent.SetState(StateCompleted)
	if agent.GetState() != StateCompleted {
		t.Errorf("state = %v, want Completed", agent.GetState())
	}

	agent.SetState(StateFailed)
	if agent.GetState() != StateFailed {
		t.Errorf("state = %v, want Failed", agent.GetState())
	}
}

func TestRunningAgent_Cleanup(t *testing.T) {
	cleaned := false
	agent := &RunningAgent{
		ID:        "agent-1",
		cleanupFn: func() { cleaned = true },
	}

	agent.Cleanup()
	if !cleaned {
		t.Error("cleanup function should have been called")
	}

	// Second call should be no-op (cleanupFn cleared)
	cleaned = false
	agent.Cleanup()
	if cleaned {
		t.Error("cleanup function should not be called again")
	}
}

func TestRunningAgent_SetMetrics(t *testing.T) {
	agent := &RunningAgent{ID: "agent-1"}
	metrics := TaskMetrics{
		TokensUsed: types.BetaUsage{InputTokens: 100, OutputTokens: 50},
		ToolUses:   3,
		Duration:   5 * time.Second,
		TurnCount:  2,
		CostUSD:    0.01,
	}
	agent.SetMetrics(metrics)

	agent.mu.Lock()
	got := agent.Metrics
	agent.mu.Unlock()

	if got.TokensUsed.InputTokens != 100 {
		t.Errorf("InputTokens = %d", got.TokensUsed.InputTokens)
	}
	if got.ToolUses != 3 {
		t.Errorf("ToolUses = %d", got.ToolUses)
	}
}

func TestAgentState_String(t *testing.T) {
	tests := []struct {
		state AgentState
		want  string
	}{
		{StateRunning, "running"},
		{StateCompleted, "completed"},
		{StateFailed, "failed"},
		{StateStopped, "stopped"},
		{AgentState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
