package tools

import (
	"context"
	"fmt"
	"testing"
)

func TestSendMessageToolDirectMessage(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	var sentMsg TeamMessage
	tool := &SendMessageTool{
		Coordinator: &mockTeamCoordinator{
			sendMessageFn: func(_ context.Context, msg TeamMessage) error {
				sentMsg = msg
				return nil
			},
		},
	}

	output, err := tool.Execute(context.Background(), map[string]any{
		"type":      "message",
		"recipient": "worker-1",
		"content":   "Start on task A",
		"summary":   "Task assignment",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if output.IsError {
		t.Fatalf("unexpected error: %s", output.Content)
	}
	if sentMsg.To != "worker-1" {
		t.Errorf("expected worker-1, got %s", sentMsg.To)
	}
	if sentMsg.Content != "Start on task A" {
		t.Errorf("unexpected content: %s", sentMsg.Content)
	}
	if sentMsg.Summary != "Task assignment" {
		t.Errorf("unexpected summary: %s", sentMsg.Summary)
	}
}

func TestSendMessageToolBroadcast(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	var broadcastContent string
	var broadcastRecipients []string
	tool := &SendMessageTool{
		Coordinator: &mockTeamCoordinator{
			memberNames: []string{"worker-1", "worker-2"},
			broadcastFn: func(_ context.Context, _, content string, recipients []string) error {
				broadcastContent = content
				broadcastRecipients = recipients
				return nil
			},
		},
	}

	output, _ := tool.Execute(context.Background(), map[string]any{
		"type":    "broadcast",
		"content": "Team meeting",
	})
	if output.IsError {
		t.Fatalf("unexpected error: %s", output.Content)
	}
	if broadcastContent != "Team meeting" {
		t.Errorf("unexpected broadcast content: %s", broadcastContent)
	}
	if len(broadcastRecipients) != 2 {
		t.Errorf("expected 2 recipients, got %d", len(broadcastRecipients))
	}
}

func TestSendMessageToolShutdownRequest(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	var shutdownTarget string
	tool := &SendMessageTool{
		Coordinator: &mockTeamCoordinator{
			requestShutFn: func(_ context.Context, name string) error {
				shutdownTarget = name
				return nil
			},
		},
	}

	output, _ := tool.Execute(context.Background(), map[string]any{
		"type":      "shutdown_request",
		"recipient": "worker-1",
		"content":   "Please shut down",
	})
	if output.IsError {
		t.Fatalf("unexpected error: %s", output.Content)
	}
	if shutdownTarget != "worker-1" {
		t.Errorf("expected worker-1, got %s", shutdownTarget)
	}
}

func TestSendMessageToolShutdownRequestNoRecipient(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &SendMessageTool{Coordinator: &mockTeamCoordinator{}}
	output, _ := tool.Execute(context.Background(), map[string]any{
		"type":    "shutdown_request",
		"content": "Shut down",
	})
	if !output.IsError {
		t.Fatal("expected error for missing recipient")
	}
}

func TestSendMessageToolMissingType(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &SendMessageTool{Coordinator: &mockTeamCoordinator{}}
	output, _ := tool.Execute(context.Background(), map[string]any{
		"content": "Hello",
	})
	if !output.IsError {
		t.Fatal("expected error for missing type")
	}
}

func TestSendMessageToolMissingContent(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &SendMessageTool{Coordinator: &mockTeamCoordinator{}}
	output, _ := tool.Execute(context.Background(), map[string]any{
		"type":      "message",
		"recipient": "worker-1",
	})
	if !output.IsError {
		t.Fatal("expected error for missing content")
	}
}

func TestSendMessageToolDirectMessageNoRecipient(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &SendMessageTool{Coordinator: &mockTeamCoordinator{}}
	output, _ := tool.Execute(context.Background(), map[string]any{
		"type":    "message",
		"content": "Hello",
	})
	if !output.IsError {
		t.Fatal("expected error for missing recipient")
	}
}

func TestSendMessageToolCoordinatorError(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	tool := &SendMessageTool{
		Coordinator: &mockTeamCoordinator{
			sendMessageFn: func(_ context.Context, _ TeamMessage) error {
				return fmt.Errorf("mailbox full")
			},
		},
	}

	output, _ := tool.Execute(context.Background(), map[string]any{
		"type":      "message",
		"recipient": "worker-1",
		"content":   "Hello",
	})
	if !output.IsError {
		t.Fatal("expected error from coordinator")
	}
}

func TestSendMessageToolFeatureGate(t *testing.T) {
	tool := &SendMessageTool{}
	output, _ := tool.Execute(context.Background(), map[string]any{
		"type":    "message",
		"content": "Hello",
	})
	if !output.IsError {
		t.Fatal("expected error for disabled feature")
	}
}

func TestSendMessageToolName(t *testing.T) {
	tool := &SendMessageTool{}
	if tool.Name() != "SendMessage" {
		t.Errorf("expected SendMessage, got %s", tool.Name())
	}
}

func TestSendMessageToolShutdownResponse(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "1")

	var sentMsg TeamMessage
	tool := &SendMessageTool{
		Coordinator: &mockTeamCoordinator{
			sendMessageFn: func(_ context.Context, msg TeamMessage) error {
				sentMsg = msg
				return nil
			},
		},
	}

	output, _ := tool.Execute(context.Background(), map[string]any{
		"type":       "shutdown_response",
		"recipient":  "lead",
		"content":    "Approved",
		"request_id": "req-123",
		"approve":    true,
	})
	if output.IsError {
		t.Fatalf("unexpected error: %s", output.Content)
	}
	if sentMsg.Type != "shutdown_response" {
		t.Errorf("expected shutdown_response, got %s", sentMsg.Type)
	}
	if !sentMsg.Approve {
		t.Error("expected approve=true")
	}
	if sentMsg.RequestID != "req-123" {
		t.Errorf("expected req-123, got %s", sentMsg.RequestID)
	}
}
