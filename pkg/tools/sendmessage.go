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
	return `Send messages to agent teammates and handle protocol requests/responses in a team.

## Message Types

### type: "message" - Send a Direct Message
Send a message to a single specific teammate. You MUST specify the recipient.

IMPORTANT for teammates: Your plain text output is NOT visible to the team lead or other teammates. To communicate with anyone on your team, you MUST use this tool.

### type: "broadcast" - Send Message to ALL Teammates (USE SPARINGLY)
Send the same message to everyone on the team at once.

WARNING: Broadcasting is expensive. Each broadcast sends a separate message to every teammate, which means N teammates = N separate message deliveries. Costs scale linearly with team size.

CRITICAL: Use broadcast only when absolutely necessary — critical issues requiring immediate team-wide attention. Default to "message" instead for normal communication.

### type: "shutdown_request" - Request a Teammate to Shut Down
Use this to ask a teammate to gracefully shut down. The teammate will receive a shutdown request and can either approve (exit) or reject (continue working).

### type: "shutdown_response" - Respond to a Shutdown Request
When you receive a shutdown request as a JSON message with type: "shutdown_request", you MUST respond to approve or reject it. Extract the requestId from the JSON message and pass it as request_id to the tool. Simply saying "I'll shut down" is not enough — you must call the tool.

### type: "plan_approval_response" - Approve or Reject a Teammate's Plan
When a teammate with plan_mode_required calls ExitPlanMode, they send you a plan approval request. Use this to approve or reject their plan with feedback.

## Important Notes
- Messages from teammates are automatically delivered to you. You do NOT need to manually check your inbox.
- IMPORTANT: Always refer to teammates by their NAME (e.g., "team-lead", "researcher", "tester"), never by UUID.
- Do NOT send structured JSON status messages. Use TaskUpdate to mark tasks completed and the system will automatically send idle notifications when you stop.`
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
