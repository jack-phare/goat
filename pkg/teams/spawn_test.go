package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
)

// fakeSpawnFunc creates a fake spawner that returns a nil process.
func fakeSpawnFunc(pids *[]int) SpawnFunc {
	return func(_ context.Context, name, teamName, agentType, baseDir, agentID string) (*os.Process, error) {
		// Use the current process as a fake (it's alive and findable)
		pid := os.Getpid()
		if pids != nil {
			*pids = append(*pids, pid)
		}
		// Return nil process to avoid actually managing a process
		return nil, nil
	}
}

func failSpawnFunc() SpawnFunc {
	return func(_ context.Context, name, teamName, agentType, baseDir, agentID string) (*os.Process, error) {
		return nil, fmt.Errorf("spawn failed: simulated error")
	}
}

func TestSpawnTeammateWithFunc(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "spawn-test")

	var pids []int
	member, err := tm.SpawnTeammateWithFunc(ctx, "worker-1", "general-purpose", "Do work", fakeSpawnFunc(&pids))
	if err != nil {
		t.Fatalf("SpawnTeammateWithFunc: %v", err)
	}

	if member.Name != "worker-1" {
		t.Errorf("expected worker-1, got %s", member.Name)
	}
	if member.AgentType != "general-purpose" {
		t.Errorf("expected general-purpose, got %s", member.AgentType)
	}
	if member.AgentID == "" {
		t.Error("expected non-empty agent ID")
	}
	if member.GetState() != MemberActive {
		t.Errorf("expected active, got %v", member.GetState())
	}

	// Verify member is in team
	m, ok := team.GetMember("worker-1")
	if !ok {
		t.Fatal("member not found in team")
	}
	if m.AgentID != member.AgentID {
		t.Error("agent IDs don't match")
	}
}

func TestSpawnTeammateUpdatesConfig(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "config-update")

	tm.SpawnTeammateWithFunc(ctx, "alice", "explore", "Research the codebase", fakeSpawnFunc(nil))

	// Read config file
	data, _ := os.ReadFile(team.configPath)
	var config TeamConfig
	json.Unmarshal(data, &config)

	if len(config.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(config.Members))
	}
	if config.Members[0].Name != "alice" {
		t.Errorf("expected alice, got %s", config.Members[0].Name)
	}
	if config.Members[0].AgentType != "explore" {
		t.Errorf("expected explore, got %s", config.Members[0].AgentType)
	}
}

func TestSpawnTeammateSendsInitialPrompt(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "prompt-test")

	tm.SpawnTeammateWithFunc(ctx, "bob", "plan", "Plan the implementation", fakeSpawnFunc(nil))

	// Check mailbox for initial prompt
	msgs, _ := team.Mailbox.Receive("bob")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Plan the implementation" {
		t.Errorf("unexpected prompt: %s", msgs[0].Content)
	}
	if msgs[0].From != "lead" {
		t.Errorf("expected from lead, got %s", msgs[0].From)
	}
}

func TestSpawnTeammateEmptyPrompt(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()
	team, _ := tm.CreateTeam(ctx, "no-prompt")

	tm.SpawnTeammateWithFunc(ctx, "charlie", "bash", "", fakeSpawnFunc(nil))

	msgs, _ := team.Mailbox.Receive("charlie")
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages for empty prompt, got %d", len(msgs))
	}
}

func TestSpawnTeammateNoTeam(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	_, err := tm.SpawnTeammateWithFunc(context.Background(), "worker", "bash", "test", fakeSpawnFunc(nil))
	if err == nil {
		t.Fatal("expected error with no active team")
	}
}

func TestSpawnTeammateDuplicateName(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()
	tm.CreateTeam(ctx, "dup-name")

	tm.SpawnTeammateWithFunc(ctx, "worker", "bash", "first", fakeSpawnFunc(nil))

	_, err := tm.SpawnTeammateWithFunc(ctx, "worker", "bash", "second", fakeSpawnFunc(nil))
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestSpawnTeammateEmptyFields(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()
	tm.CreateTeam(ctx, "empty-fields")

	_, err := tm.SpawnTeammateWithFunc(ctx, "", "bash", "test", fakeSpawnFunc(nil))
	if err == nil {
		t.Fatal("expected error for empty name")
	}

	_, err = tm.SpawnTeammateWithFunc(ctx, "worker", "", "test", fakeSpawnFunc(nil))
	if err == nil {
		t.Fatal("expected error for empty agent type")
	}
}

func TestSpawnTeammateSpawnFailure(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()
	tm.CreateTeam(ctx, "fail-spawn")

	_, err := tm.SpawnTeammateWithFunc(ctx, "worker", "bash", "test", failSpawnFunc())
	if err == nil {
		t.Fatal("expected error from failed spawn")
	}
}

func TestShutdownRequestShutdown(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "shutdown-req")
	tm.SpawnTeammateWithFunc(ctx, "worker", "bash", "test", fakeSpawnFunc(nil))

	if err := tm.RequestShutdown(ctx, "worker"); err != nil {
		t.Fatalf("RequestShutdown: %v", err)
	}

	// Check mailbox for shutdown request
	msgs, _ := team.Mailbox.Receive("worker")
	found := false
	for _, msg := range msgs {
		if msg.Type == "shutdown_request" {
			found = true
		}
	}
	if !found {
		t.Error("expected shutdown_request message in mailbox")
	}
}

func TestShutdownRequestNoTeam(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	err := tm.RequestShutdown(context.Background(), "nobody")
	if err == nil {
		t.Fatal("expected error with no active team")
	}
}

func TestShutdownRequestUnknownMember(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()
	tm.CreateTeam(ctx, "unknown")

	err := tm.RequestShutdown(ctx, "ghost")
	if err == nil {
		t.Fatal("expected error for unknown member")
	}
}

func TestShutdownRequestAlreadyStopped(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()
	team, _ := tm.CreateTeam(ctx, "stopped-req")
	team.Members["worker"] = &TeamMember{Name: "worker", State: MemberStopped}

	err := tm.RequestShutdown(ctx, "worker")
	if err == nil {
		t.Fatal("expected error for already-stopped member")
	}
}

func TestShutdownTeammateAlreadyStopped(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()
	team, _ := tm.CreateTeam(ctx, "already-stopped")
	team.Members["worker"] = &TeamMember{Name: "worker", State: MemberStopped}

	// Should be a no-op
	err := tm.ShutdownTeammate(ctx, "worker", 5*time.Second)
	if err != nil {
		t.Fatalf("ShutdownTeammate on stopped member: %v", err)
	}
}

func TestShutdownTeammateNoProcess(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()
	tm.CreateTeam(ctx, "no-proc")

	// Spawn with nil process
	tm.SpawnTeammateWithFunc(ctx, "worker", "bash", "test", fakeSpawnFunc(nil))

	err := tm.ShutdownTeammate(ctx, "worker", 5*time.Second)
	if err != nil {
		t.Fatalf("ShutdownTeammate: %v", err)
	}

	team := tm.GetTeam()
	m, _ := team.GetMember("worker")
	if m.GetState() != MemberStopped {
		t.Errorf("expected stopped, got %v", m.GetState())
	}
}

func TestShutdownTeammateNoTeam(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	err := tm.ShutdownTeammate(context.Background(), "nobody", 5*time.Second)
	if err == nil {
		t.Fatal("expected error with no active team")
	}
}

func TestShutdownTeammateUnknown(t *testing.T) {
	tm := NewTeamManager(&agent.NoOpHookRunner{}, t.TempDir())
	ctx := context.Background()
	tm.CreateTeam(ctx, "unknown-shutdown")

	err := tm.ShutdownTeammate(ctx, "ghost", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for unknown member")
	}
}
