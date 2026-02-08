package teams

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTeammateRuntime(t *testing.T) (*TeammateRuntime, string) {
	t.Helper()
	baseDir := t.TempDir()

	// Create team config
	teamDir := filepath.Join(baseDir, "teams", "test-team")
	os.MkdirAll(teamDir, 0o755)

	config := TeamConfig{
		Name: "test-team",
		Members: []MemberConfig{
			{Name: "lead", AgentID: "lead-id", AgentType: "general-purpose"},
			{Name: "worker-1", AgentID: "worker-id", AgentType: "explore"},
		},
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(filepath.Join(teamDir, "config.json"), data, 0o644)

	// Create tasks dir
	tasksDir := filepath.Join(baseDir, "tasks", "test-team")
	os.MkdirAll(tasksDir, 0o755)

	// Create mailbox dir
	mailboxDir := filepath.Join(teamDir, "mailbox")
	os.MkdirAll(mailboxDir, 0o755)

	tr := NewTeammateRuntime("test-team", "worker-1", "explore", baseDir)
	return tr, baseDir
}

func TestTeammateRuntimeLoadConfig(t *testing.T) {
	tr, _ := setupTeammateRuntime(t)

	if err := tr.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if tr.Config().Name != "test-team" {
		t.Errorf("expected test-team, got %s", tr.Config().Name)
	}
	if len(tr.Config().Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(tr.Config().Members))
	}
}

func TestTeammateRuntimeLoadConfigMissing(t *testing.T) {
	tr := NewTeammateRuntime("nonexistent", "worker", "bash", t.TempDir())
	if err := tr.LoadConfig(); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestTeammateRuntimeAccessors(t *testing.T) {
	tr, _ := setupTeammateRuntime(t)

	if tr.TeamName() != "test-team" {
		t.Errorf("expected test-team, got %s", tr.TeamName())
	}
	if tr.AgentName() != "worker-1" {
		t.Errorf("expected worker-1, got %s", tr.AgentName())
	}
	if tr.AgentType() != "explore" {
		t.Errorf("expected explore, got %s", tr.AgentType())
	}
}

func TestTeammateRuntimeReceiveMessages(t *testing.T) {
	tr, _ := setupTeammateRuntime(t)

	// Send a message to the teammate
	tr.mailbox.Send(Message{
		From:    "lead",
		To:      "worker-1",
		Content: "Start task A",
		Type:    "message",
	})

	msgs, err := tr.ReceiveMessages()
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Start task A" {
		t.Errorf("unexpected content: %s", msgs[0].Content)
	}
}

func TestTeammateRuntimeSendToLead(t *testing.T) {
	tr, baseDir := setupTeammateRuntime(t)

	if err := tr.SendToLead("Task A complete", "message"); err != nil {
		t.Fatalf("SendToLead: %v", err)
	}

	// Read from lead's mailbox
	leadMailbox := NewMailbox(filepath.Join(baseDir, "teams", "test-team", "mailbox"))
	msgs, _ := leadMailbox.Receive("lead")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for lead, got %d", len(msgs))
	}
	if msgs[0].From != "worker-1" {
		t.Errorf("expected from worker-1, got %s", msgs[0].From)
	}
	if msgs[0].Content != "Task A complete" {
		t.Errorf("unexpected content: %s", msgs[0].Content)
	}
}

func TestTeammateRuntimeNotifyIdle(t *testing.T) {
	tr, baseDir := setupTeammateRuntime(t)

	if err := tr.NotifyIdle(); err != nil {
		t.Fatalf("NotifyIdle: %v", err)
	}

	leadMailbox := NewMailbox(filepath.Join(baseDir, "teams", "test-team", "mailbox"))
	msgs, _ := leadMailbox.Receive("lead")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].From != "worker-1" {
		t.Errorf("expected from worker-1, got %s", msgs[0].From)
	}
}

func TestTeammateRuntimeRespondToShutdownApprove(t *testing.T) {
	tr, baseDir := setupTeammateRuntime(t)

	if err := tr.RespondToShutdown(true, ""); err != nil {
		t.Fatalf("RespondToShutdown: %v", err)
	}

	leadMailbox := NewMailbox(filepath.Join(baseDir, "teams", "test-team", "mailbox"))
	msgs, _ := leadMailbox.Receive("lead")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Type != "shutdown_response" {
		t.Errorf("expected shutdown_response, got %s", msgs[0].Type)
	}
	if msgs[0].Content != "Shutdown approved." {
		t.Errorf("unexpected content: %s", msgs[0].Content)
	}
}

func TestTeammateRuntimeRespondToShutdownReject(t *testing.T) {
	tr, baseDir := setupTeammateRuntime(t)

	if err := tr.RespondToShutdown(false, "Still working on task B"); err != nil {
		t.Fatalf("RespondToShutdown: %v", err)
	}

	leadMailbox := NewMailbox(filepath.Join(baseDir, "teams", "test-team", "mailbox"))
	msgs, _ := leadMailbox.Receive("lead")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Shutdown rejected: Still working on task B" {
		t.Errorf("unexpected content: %s", msgs[0].Content)
	}
}

func TestTeammateRuntimeWatchMessages(t *testing.T) {
	tr, _ := setupTeammateRuntime(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := tr.WatchMessages(ctx)
	if err != nil {
		t.Fatalf("WatchMessages: %v", err)
	}

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	tr.mailbox.Send(Message{
		From:    "lead",
		To:      "worker-1",
		Content: "New instructions",
		Type:    "message",
	})

	select {
	case msg := <-ch:
		if msg.Content != "New instructions" {
			t.Errorf("unexpected content: %s", msg.Content)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watched message")
	}
}

func TestTeammateRuntimeTaskOperations(t *testing.T) {
	tr, _ := setupTeammateRuntime(t)

	// Create a task
	task := TeamTask{
		ID:        "task-1",
		Subject:   "Build feature",
		CreatedBy: "lead",
	}
	if err := tr.tasks.Create(task); err != nil {
		t.Fatalf("Create task: %v", err)
	}

	// Get unblocked tasks
	unblocked, err := tr.GetUnblockedTasks()
	if err != nil {
		t.Fatalf("GetUnblockedTasks: %v", err)
	}
	if len(unblocked) != 1 {
		t.Fatalf("expected 1 unblocked task, got %d", len(unblocked))
	}

	// Claim the task
	if err := tr.ClaimTask("task-1"); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	// Complete the task
	if err := tr.CompleteTask("task-1"); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Verify no more unblocked tasks
	unblocked, _ = tr.GetUnblockedTasks()
	if len(unblocked) != 0 {
		t.Errorf("expected 0 unblocked tasks, got %d", len(unblocked))
	}
}

func TestIsShutdownRequest(t *testing.T) {
	if !IsShutdownRequest(Message{Type: "shutdown_request"}) {
		t.Error("expected true for shutdown_request")
	}
	if IsShutdownRequest(Message{Type: "message"}) {
		t.Error("expected false for regular message")
	}
	if IsShutdownRequest(Message{Type: ""}) {
		t.Error("expected false for empty type")
	}
}
