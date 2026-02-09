package tools

import (
	"context"
	"fmt"
)

// SendMessageTool sends messages between teammates.
type SendMessageTool struct {
	Coordinator TeamCoordinator
}

func (s *SendMessageTool) Name() string { return "SendMessage" }

func (s *SendMessageTool) Description() string {
	return "Sends a message to a teammate or broadcasts to all teammates."
}

func (s *SendMessageTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{"message", "broadcast", "shutdown_request", "shutdown_response", "plan_approval_request", "plan_approval_response"},
				"description": "Message type",
			},
			"recipient": map[string]any{
				"type":        "string",
				"description": "Name of the recipient teammate (required for non-broadcast)",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Message content",
			},
			"summary": map[string]any{
				"type":        "string",
				"description": "Short summary of the message",
			},
			"request_id": map[string]any{
				"type":        "string",
				"description": "ID of the request being responded to",
			},
			"approve": map[string]any{
				"type":        "boolean",
				"description": "Whether to approve (for shutdown_response/plan_approval_response)",
			},
		},
		"required": []string{"type", "content"},
	}
}

func (s *SendMessageTool) SideEffect() SideEffectType { return SideEffectMutating }

func (s *SendMessageTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	if !isTeamsEnabled() {
		return ToolOutput{
			Content: "Error: Agent teams are not enabled. Set CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1 to enable.",
			IsError: true,
		}, nil
	}

	msgType, ok := input["type"].(string)
	if !ok || msgType == "" {
		return ToolOutput{Content: "Error: type is required", IsError: true}, nil
	}

	content, ok := input["content"].(string)
	if !ok || content == "" {
		return ToolOutput{Content: "Error: content is required", IsError: true}, nil
	}

	coordinator := s.Coordinator
	if coordinator == nil {
		coordinator = &StubTeamCoordinator{}
	}

	recipient, _ := input["recipient"].(string)
	summary, _ := input["summary"].(string)
	requestID, _ := input["request_id"].(string)
	approve, _ := input["approve"].(bool)

	switch msgType {
	case "broadcast":
		members := coordinator.GetMemberNames()
		if err := coordinator.Broadcast(ctx, "lead", content, members); err != nil {
			return ToolOutput{
				Content: fmt.Sprintf("Error: %s", err),
				IsError: true,
			}, nil
		}
		return ToolOutput{Content: fmt.Sprintf("Broadcast sent to %d teammates.", len(members))}, nil

	case "shutdown_request":
		if recipient == "" {
			return ToolOutput{Content: "Error: recipient is required for shutdown_request", IsError: true}, nil
		}
		if err := coordinator.RequestShutdown(ctx, recipient); err != nil {
			return ToolOutput{
				Content: fmt.Sprintf("Error: %s", err),
				IsError: true,
			}, nil
		}
		return ToolOutput{Content: fmt.Sprintf("Shutdown request sent to %s.", recipient)}, nil

	default:
		// message, shutdown_response, plan_approval_response
		if recipient == "" {
			return ToolOutput{Content: "Error: recipient is required", IsError: true}, nil
		}

		msg := TeamMessage{
			From:      "lead",
			To:        recipient,
			Content:   content,
			Summary:   summary,
			Type:      msgType,
			RequestID: requestID,
			Approve:   approve,
		}

		if err := coordinator.SendMessage(ctx, msg); err != nil {
			return ToolOutput{
				Content: fmt.Sprintf("Error: %s", err),
				IsError: true,
			}, nil
		}

		return ToolOutput{Content: fmt.Sprintf("Message sent to %s.", recipient)}, nil
	}
}
