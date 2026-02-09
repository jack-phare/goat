package teams

import (
	"context"
	"strings"
	"testing"

	"github.com/jg-phare/goat/pkg/tools"
)

func TestAdapterSatisfiesInterface(t *testing.T) {
	var _ tools.TeamCoordinator = (*TeamManagerAdapter)(nil)
}

func TestAdapterCreateTeam(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	info, err := adapter.CreateTeam(context.Background(), "adapter-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if info.Name != "adapter-team" {
		t.Errorf("name = %q, want adapter-team", info.Name)
	}
	if info.ConfigPath == "" {
		t.Error("expected non-empty ConfigPath")
	}
}

func TestAdapterCreateTeamError(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	// Create first team
	adapter.CreateTeam(context.Background(), "first")

	// Second should fail
	_, err := adapter.CreateTeam(context.Background(), "second")
	if err == nil {
		t.Fatal("expected error creating second team")
	}
}

func TestAdapterSpawnTeammate(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{
		TM: tm,
		SpawnFunc: func(_ context.Context, name, agentType, prompt string) (tools.TeamMemberInfo, error) {
			return tools.TeamMemberInfo{
				Name:    name,
				AgentID: "spawned-" + name,
			}, nil
		},
	}

	info, err := adapter.SpawnTeammate(context.Background(), "worker", "Explore", "Search the code")
	if err != nil {
		t.Fatalf("SpawnTeammate: %v", err)
	}
	if info.Name != "worker" {
		t.Errorf("name = %q, want worker", info.Name)
	}
	if info.AgentID != "spawned-worker" {
		t.Errorf("agentID = %q, want spawned-worker", info.AgentID)
	}
}

func TestAdapterSpawnTeammateNotConfigured(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	_, err := adapter.SpawnTeammate(context.Background(), "worker", "Explore", "prompt")
	if err == nil {
		t.Fatal("expected error when SpawnFunc is nil")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error = %q, want 'not configured'", err.Error())
	}
}

func TestAdapterRequestShutdown(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	// Create team first
	team, _ := tm.CreateTeam(context.Background(), "shutdown-test")
	team.Members["worker"] = &TeamMember{Name: "worker", State: MemberActive}

	err := adapter.RequestShutdown(context.Background(), "worker")
	if err != nil {
		t.Fatalf("RequestShutdown: %v", err)
	}

	// Verify message was sent
	msgs, _ := team.Mailbox.Receive("worker")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Type != "shutdown_request" {
		t.Errorf("type = %q, want shutdown_request", msgs[0].Type)
	}
}

func TestAdapterRequestShutdownNoTeam(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	err := adapter.RequestShutdown(context.Background(), "worker")
	if err == nil {
		t.Fatal("expected error with no active team")
	}
}

func TestAdapterSendMessage(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	team, _ := tm.CreateTeam(context.Background(), "msg-test")
	team.Members["worker"] = &TeamMember{Name: "worker", State: MemberActive}

	err := adapter.SendMessage(context.Background(), tools.TeamMessage{
		From:    "lead",
		To:      "worker",
		Content: "Do task A",
		Summary: "task A",
		Type:    "message",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	msgs, _ := team.Mailbox.Receive("worker")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Do task A" {
		t.Errorf("content = %q, want 'Do task A'", msgs[0].Content)
	}
	if msgs[0].Summary != "task A" {
		t.Errorf("summary = %q, want 'task A'", msgs[0].Summary)
	}
}

func TestAdapterSendMessageNoTeam(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	err := adapter.SendMessage(context.Background(), tools.TeamMessage{
		From: "lead", To: "worker", Content: "hello",
	})
	if err == nil {
		t.Fatal("expected error with no active team")
	}
}

func TestAdapterBroadcast(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	team, _ := tm.CreateTeam(context.Background(), "broadcast-test")
	team.Members["alice"] = &TeamMember{Name: "alice", State: MemberActive}
	team.Members["bob"] = &TeamMember{Name: "bob", State: MemberActive}

	err := adapter.Broadcast(context.Background(), "lead", "Status update", []string{"alice", "bob"})
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	aliceMsgs, _ := team.Mailbox.Receive("alice")
	if len(aliceMsgs) != 1 {
		t.Fatalf("expected 1 message for alice, got %d", len(aliceMsgs))
	}
	if aliceMsgs[0].Content != "Status update" {
		t.Errorf("alice content = %q, want 'Status update'", aliceMsgs[0].Content)
	}

	bobMsgs, _ := team.Mailbox.Receive("bob")
	if len(bobMsgs) != 1 {
		t.Fatalf("expected 1 message for bob, got %d", len(bobMsgs))
	}
}

func TestAdapterBroadcastNoTeam(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	err := adapter.Broadcast(context.Background(), "lead", "msg", []string{"a"})
	if err == nil {
		t.Fatal("expected error with no active team")
	}
}

func TestAdapterCleanup(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	tm.CreateTeam(context.Background(), "cleanup-test")

	err := adapter.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if tm.GetTeam() != nil {
		t.Error("expected nil team after cleanup")
	}
}

func TestAdapterGetTeamName(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	// No team
	if name := adapter.GetTeamName(); name != "" {
		t.Errorf("expected empty name, got %q", name)
	}

	tm.CreateTeam(context.Background(), "named-team")
	if name := adapter.GetTeamName(); name != "named-team" {
		t.Errorf("name = %q, want named-team", name)
	}
}

func TestAdapterGetMemberNames(t *testing.T) {
	tm := newTestManager(t)
	adapter := &TeamManagerAdapter{TM: tm}

	// No team
	if names := adapter.GetMemberNames(); names != nil {
		t.Errorf("expected nil, got %v", names)
	}

	team, _ := tm.CreateTeam(context.Background(), "names-test")
	team.Members["alice"] = &TeamMember{Name: "alice"}
	team.Members["bob"] = &TeamMember{Name: "bob"}

	names := adapter.GetMemberNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}
