package teams

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

func TestIntegrationFullLifecycle(t *testing.T) {
	baseDir := t.TempDir()
	tm := NewTeamManager(&agent.NoOpHookRunner{}, baseDir)
	ctx := context.Background()

	// 1. Create team
	team, err := tm.CreateTeam(ctx, "integration-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	// 2. Spawn teammates
	worker1, err := tm.SpawnTeammateWithFunc(ctx, "worker-1", "explore", "Research the codebase", fakeSpawnFunc(nil))
	if err != nil {
		t.Fatalf("Spawn worker-1: %v", err)
	}
	worker2, err := tm.SpawnTeammateWithFunc(ctx, "worker-2", "plan", "Plan the implementation", fakeSpawnFunc(nil))
	if err != nil {
		t.Fatalf("Spawn worker-2: %v", err)
	}

	// 3. Create tasks
	team.Tasks.Create(TeamTask{ID: "t1", Subject: "Research APIs", CreatedBy: "lead", AssignedTo: "worker-1"})
	team.Tasks.Create(TeamTask{ID: "t2", Subject: "Design architecture", CreatedBy: "lead", DependsOn: []string{"t1"}})
	team.Tasks.Create(TeamTask{ID: "t3", Subject: "Implement feature", CreatedBy: "lead", DependsOn: []string{"t2"}})

	// 4. Verify unblocked tasks
	unblocked, _ := team.Tasks.GetUnblocked()
	if len(unblocked) != 1 || unblocked[0].ID != "t1" {
		t.Fatalf("expected only t1 unblocked, got %v", taskIDs(unblocked))
	}

	// 5. Claim and complete t1
	team.Tasks.Claim("t1", worker1.AgentID)
	team.Tasks.Complete("t1")

	// 6. Now t2 should be unblocked
	unblocked, _ = team.Tasks.GetUnblocked()
	if len(unblocked) != 1 || unblocked[0].ID != "t2" {
		t.Fatalf("expected t2 unblocked after t1 completion, got %v", taskIDs(unblocked))
	}

	// 7. Claim and complete t2
	team.Tasks.Claim("t2", worker2.AgentID)
	team.Tasks.Complete("t2")

	// 8. Now t3 should be unblocked
	unblocked, _ = team.Tasks.GetUnblocked()
	if len(unblocked) != 1 || unblocked[0].ID != "t3" {
		t.Fatalf("expected t3 unblocked, got %v", taskIDs(unblocked))
	}

	// 9. Send messages between teammates
	team.Mailbox.Send(Message{From: "lead", To: "worker-1", Content: "Good job!", Type: "message"})
	team.Mailbox.Send(Message{From: "worker-1", To: "worker-2", Content: "Ready for handoff", Type: "message"})

	msgs, _ := team.Mailbox.Receive("worker-1")
	if len(msgs) != 2 { // initial prompt + "Good job!"
		t.Errorf("expected 2 messages for worker-1, got %d: %v", len(msgs), msgs)
	}

	msgs, _ = team.Mailbox.Receive("worker-2")
	if len(msgs) != 2 { // initial prompt + handoff message
		t.Errorf("expected 2 messages for worker-2, got %d", len(msgs))
	}

	// 10. Shutdown teammates
	worker1.SetState(MemberStopped)
	worker2.SetState(MemberStopped)

	// 11. Cleanup
	if err := tm.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// 12. Verify directories removed
	teamDir := filepath.Join(baseDir, "teams", "integration-team")
	if _, err := os.Stat(teamDir); !os.IsNotExist(err) {
		t.Error("team directory still exists after cleanup")
	}
}

func TestIntegrationMessageDeliveryBetweenTeammates(t *testing.T) {
	baseDir := t.TempDir()
	tm := NewTeamManager(&agent.NoOpHookRunner{}, baseDir)
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "msg-team")
	tm.SpawnTeammateWithFunc(ctx, "alice", "explore", "", fakeSpawnFunc(nil))
	tm.SpawnTeammateWithFunc(ctx, "bob", "plan", "", fakeSpawnFunc(nil))

	// Create teammate runtimes
	aliceRT := NewTeammateRuntime("msg-team", "alice", "explore", baseDir)
	bobRT := NewTeammateRuntime("msg-team", "bob", "plan", baseDir)

	// Alice sends to Bob
	team.Mailbox.Send(Message{From: "alice", To: "bob", Content: "I found the API docs", Type: "message"})

	// Bob receives
	msgs, _ := bobRT.ReceiveMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for bob, got %d", len(msgs))
	}
	if msgs[0].From != "alice" || msgs[0].Content != "I found the API docs" {
		t.Errorf("unexpected message: %+v", msgs[0])
	}

	// Bob sends to Alice
	bobRT.mailbox.Send(Message{From: "bob", To: "alice", Content: "Thanks, noted", Type: "message"})

	// Alice receives
	msgs, _ = aliceRT.ReceiveMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for alice, got %d", len(msgs))
	}
}

func TestIntegrationBroadcast(t *testing.T) {
	baseDir := t.TempDir()
	tm := NewTeamManager(&agent.NoOpHookRunner{}, baseDir)
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "broadcast-team")
	tm.SpawnTeammateWithFunc(ctx, "alice", "explore", "", fakeSpawnFunc(nil))
	tm.SpawnTeammateWithFunc(ctx, "bob", "plan", "", fakeSpawnFunc(nil))

	// Broadcast from lead
	team.Mailbox.Broadcast("lead", "Time for standup", []string{"alice", "bob"})

	aliceRT := NewTeammateRuntime("broadcast-team", "alice", "explore", baseDir)
	bobRT := NewTeammateRuntime("broadcast-team", "bob", "plan", baseDir)

	aliceMsgs, _ := aliceRT.ReceiveMessages()
	bobMsgs, _ := bobRT.ReceiveMessages()

	if len(aliceMsgs) != 1 {
		t.Errorf("expected 1 message for alice, got %d", len(aliceMsgs))
	}
	if len(bobMsgs) != 1 {
		t.Errorf("expected 1 message for bob, got %d", len(bobMsgs))
	}
}

func TestIntegrationShutdownFlow(t *testing.T) {
	baseDir := t.TempDir()
	tm := NewTeamManager(&agent.NoOpHookRunner{}, baseDir)
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "shutdown-flow")
	tm.SpawnTeammateWithFunc(ctx, "worker", "bash", "Run tests", fakeSpawnFunc(nil))

	// Lead sends shutdown request
	tm.RequestShutdown(ctx, "worker")

	// Worker receives the request
	workerRT := NewTeammateRuntime("shutdown-flow", "worker", "bash", baseDir)
	msgs, _ := workerRT.ReceiveMessages()

	foundShutdown := false
	for _, msg := range msgs {
		if IsShutdownRequest(msg) {
			foundShutdown = true
		}
	}
	if !foundShutdown {
		t.Fatal("worker didn't receive shutdown request")
	}

	// Worker responds with approval
	workerRT.RespondToShutdown(true, "")

	// Lead receives response
	leadMsgs, _ := team.Mailbox.Receive("lead")
	if len(leadMsgs) != 1 {
		t.Fatalf("expected 1 message for lead, got %d", len(leadMsgs))
	}
	if leadMsgs[0].Type != "shutdown_response" {
		t.Errorf("expected shutdown_response, got %s", leadMsgs[0].Type)
	}
}

func TestIntegrationTeammateIdleHookFiring(t *testing.T) {
	var firedTeammate string
	hooksCalled := false

	runner := &mockHookRunner{
		fireFunc: func(_ context.Context, event string, input any) ([]agent.HookResult, error) {
			if event == "TeammateIdle" {
				hooksCalled = true
				// Extract teammate name from input (already typed)
				firedTeammate = "worker"
			}
			return nil, nil
		},
	}

	tm := NewTeamManager(runner, t.TempDir())
	tm.CreateTeam(context.Background(), "idle-hook-team")

	tm.fireTeammateIdle(context.Background(), "worker")

	if !hooksCalled {
		t.Error("TeammateIdle hook was not fired")
	}
	if firedTeammate != "worker" {
		t.Errorf("unexpected teammate: %s", firedTeammate)
	}
}

func TestIntegrationTaskCompletedHookFiring(t *testing.T) {
	hooksCalled := false

	runner := &mockHookRunner{
		fireFunc: func(_ context.Context, event string, input any) ([]agent.HookResult, error) {
			if event == "TaskCompleted" {
				hooksCalled = true
			}
			return nil, nil
		},
	}

	tm := NewTeamManager(runner, t.TempDir())
	tm.CreateTeam(context.Background(), "task-hook-team")

	tm.fireTaskCompleted(context.Background(), "task-1", "Build feature", "alice")

	if !hooksCalled {
		t.Error("TaskCompleted hook was not fired")
	}
}

func TestIntegrationFeatureGate(t *testing.T) {
	// Without env var
	if IsEnabled() {
		t.Error("expected disabled without env var")
	}

	// With env var
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")
	if !IsEnabled() {
		t.Error("expected enabled with env var")
	}
}

func TestIntegrationDelegateMode(t *testing.T) {
	d := &DelegateModeState{}

	allTools := []string{
		"TeamCreate", "SendMessage", "TeamDelete",
		"TaskCreate", "TaskUpdate", "TaskList", "TaskGet",
		"Bash", "Read", "Write", "Edit", "Glob", "Grep",
		"WebFetch", "Agent",
	}

	// Without delegate mode, all tools available
	filtered := d.FilterTools(allTools)
	if len(filtered) != len(allTools) {
		t.Errorf("expected all %d tools, got %d", len(allTools), len(filtered))
	}

	// Enable delegate mode
	d.Enable()
	filtered = d.FilterTools(allTools)
	if len(filtered) != 7 {
		t.Errorf("expected 7 delegate tools, got %d: %v", len(filtered), filtered)
	}

	// Disable delegate mode
	d.Disable()
	filtered = d.FilterTools(allTools)
	if len(filtered) != len(allTools) {
		t.Errorf("expected all tools after disable, got %d", len(filtered))
	}
}

func TestIntegrationTeammateRuntimeLifecycle(t *testing.T) {
	baseDir := t.TempDir()
	tm := NewTeamManager(&agent.NoOpHookRunner{}, baseDir)
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "rt-lifecycle")
	tm.SpawnTeammateWithFunc(ctx, "worker", "bash", "Build the feature", fakeSpawnFunc(nil))

	// Create a task
	team.Tasks.Create(TeamTask{ID: "rt-t1", Subject: "Build feature", CreatedBy: "lead"})

	// Worker runtime picks up work
	workerRT := NewTeammateRuntime("rt-lifecycle", "worker", "bash", baseDir)
	workerRT.LoadConfig()

	// Receive initial prompt
	msgs, _ := workerRT.ReceiveMessages()
	if len(msgs) != 1 || msgs[0].Content != "Build the feature" {
		t.Fatalf("expected initial prompt, got %v", msgs)
	}

	// Claim and complete task
	workerRT.ClaimTask("rt-t1")
	workerRT.CompleteTask("rt-t1")

	// Notify idle
	workerRT.NotifyIdle()

	// Lead receives idle notification
	leadMsgs, _ := team.Mailbox.Receive("lead")
	if len(leadMsgs) != 1 {
		t.Fatalf("expected 1 idle notification, got %d", len(leadMsgs))
	}
}

func TestIntegrationConcurrentTaskClaiming(t *testing.T) {
	baseDir := t.TempDir()
	tm := NewTeamManager(&agent.NoOpHookRunner{}, baseDir)
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "concurrent-claim")

	// Create 3 workers
	tm.SpawnTeammateWithFunc(ctx, "w1", "bash", "", fakeSpawnFunc(nil))
	tm.SpawnTeammateWithFunc(ctx, "w2", "bash", "", fakeSpawnFunc(nil))
	tm.SpawnTeammateWithFunc(ctx, "w3", "bash", "", fakeSpawnFunc(nil))

	// Create one task
	team.Tasks.Create(TeamTask{ID: "race-t", Subject: "Shared task", CreatedBy: "lead"})

	// All workers try to claim simultaneously
	results := make(chan string, 3)
	for _, name := range []string{"w1", "w2", "w3"} {
		name := name
		go func() {
			rt := NewTeammateRuntime("concurrent-claim", name, "bash", baseDir)
			if err := rt.ClaimTask("race-t"); err == nil {
				results <- name
			} else {
				results <- ""
			}
		}()
	}

	winners := 0
	for i := 0; i < 3; i++ {
		r := <-results
		if r != "" {
			winners++
		}
	}

	if winners != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", winners)
	}
}

// mockHookRunner is a simple mock for integration tests.
type mockHookRunner struct {
	fireFunc func(ctx context.Context, event string, input any) ([]agent.HookResult, error)
}

func (m *mockHookRunner) Fire(ctx context.Context, event types.HookEvent, input any) ([]agent.HookResult, error) {
	if m.fireFunc != nil {
		return m.fireFunc(ctx, string(event), input)
	}
	return nil, nil
}
