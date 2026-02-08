package agent

import (
	"time"

	"github.com/google/uuid"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// emitInit sends the SystemInitMessage at session start.
func emitInit(ch chan<- types.SDKMessage, config *AgentConfig, state *LoopState) {
	var toolNames []string
	if config.ToolRegistry != nil {
		toolNames = config.ToolRegistry.Names()
	}

	msg := &types.SystemInitMessage{
		BaseMessage:       types.BaseMessage{UUID: uuid.New(), SessionID: state.SessionID},
		Type:              types.MessageTypeSystem,
		Subtype:           types.SystemSubtypeInit,
		Model:             config.Model,
		ClaudeCodeVersion: "goat-0.1.0",
		CWD:               config.CWD,
		PermissionMode:    config.PermissionMode,
		Tools:             toolNames,
	}
	ch <- msg
}

// emitAssistant sends an AssistantMessage after LLM response accumulation.
func emitAssistant(ch chan<- types.SDKMessage, resp *llm.CompletionResponse, state *LoopState) {
	msg := llm.EmitAssistantMessage(resp, nil, state.SessionID, nil)
	ch <- msg
}

// emitStreamEvent sends a PartialAssistantMessage for a streaming chunk.
func emitStreamEvent(ch chan<- types.SDKMessage, chunk *llm.StreamChunk, state *LoopState) {
	msg := llm.EmitStreamEvent(chunk, nil, state.SessionID)
	ch <- msg
}

// emitToolProgress sends a ToolProgressMessage for tool execution tracking.
func emitToolProgress(ch chan<- types.SDKMessage, toolName, toolUseID string, elapsed float64, state *LoopState) {
	msg := &types.ToolProgressMessage{
		BaseMessage:        types.BaseMessage{UUID: uuid.New(), SessionID: state.SessionID},
		Type:               types.MessageTypeToolProgress,
		ToolUseID:          toolUseID,
		ToolName:           toolName,
		ElapsedTimeSeconds: elapsed,
	}
	ch <- msg
}

// emitResult sends the final ResultMessage when the loop terminates.
func emitResult(ch chan<- types.SDKMessage, state *LoopState, startTime time.Time, apiDuration time.Duration) {
	duration := time.Since(startTime).Milliseconds()
	apiMs := apiDuration.Milliseconds()

	// Determine result subtype from exit reason
	switch state.ExitReason {
	case ExitEndTurn:
		// Extract final text from the last assistant message for the result
		result := extractLastTextContent(state)
		msg := types.NewResultSuccess(result, state.TurnCount, state.TotalCostUSD,
			state.TotalUsage, nil, duration, apiMs, state.SessionID)
		ch <- msg

	case ExitMaxTurns:
		msg := types.NewResultError(types.ResultSubtypeErrorMaxTurns,
			[]string{"max turns reached"}, state.TurnCount, state.TotalCostUSD,
			state.TotalUsage, nil, duration, apiMs, state.SessionID)
		ch <- msg

	case ExitMaxBudget:
		msg := types.NewResultError(types.ResultSubtypeErrorMaxBudget,
			[]string{"max budget exceeded"}, state.TurnCount, state.TotalCostUSD,
			state.TotalUsage, nil, duration, apiMs, state.SessionID)
		ch <- msg

	default:
		msg := types.NewResultError(types.ResultSubtypeErrorDuringExecution,
			[]string{string(state.ExitReason)}, state.TurnCount, state.TotalCostUSD,
			state.TotalUsage, nil, duration, apiMs, state.SessionID)
		ch <- msg
	}
}

// extractLastTextContent gets the text content from the last assistant message in state.
func extractLastTextContent(state *LoopState) string {
	// Walk backward to find the last assistant message with text content
	for i := len(state.Messages) - 1; i >= 0; i-- {
		msg := state.Messages[i]
		if msg.Role == "assistant" {
			if s, ok := msg.Content.(string); ok {
				return s
			}
		}
	}
	return ""
}
