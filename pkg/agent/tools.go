package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// executeTools runs each tool_use block and returns tool result messages.
// If interrupted is true, the caller should stop the loop.
func executeTools(ctx context.Context, toolBlocks []types.ContentBlock, config *AgentConfig, state *LoopState, ch chan<- types.SDKMessage) (results []llm.ToolResult, interrupted bool) {
	results = make([]llm.ToolResult, 0, len(toolBlocks))

	for _, block := range toolBlocks {
		// Check for interrupt/cancellation before each tool
		select {
		case <-ctx.Done():
			results = append(results, llm.ToolResult{
				ToolUseID: block.ID,
				Content:   "Error: operation cancelled",
			})
			return results, true
		default:
		}

		result, permInterrupt := executeSingleTool(ctx, block, config, state, ch)
		results = append(results, result)

		if permInterrupt {
			// Permission interrupt: stop processing remaining tools
			for _, remaining := range toolBlocks[len(results):] {
				results = append(results, llm.ToolResult{
					ToolUseID: remaining.ID,
					Content:   "Error: execution interrupted by permission check",
				})
			}
			return results, true
		}
	}

	return results, false
}

// executeSingleTool runs one tool and returns its result.
// permInterrupt is true if the permission check set Interrupt=true.
func executeSingleTool(ctx context.Context, block types.ContentBlock, config *AgentConfig, state *LoopState, ch chan<- types.SDKMessage) (llm.ToolResult, bool) {
	toolName := block.Name
	toolUseID := block.ID
	input := block.Input

	// Look up tool in registry
	tool, ok := config.ToolRegistry.Get(toolName)
	if !ok {
		return llm.ToolResult{
			ToolUseID: toolUseID,
			Content:   fmt.Sprintf("Error: unknown tool %q", toolName),
		}, false
	}

	// Check permissions
	permResult, err := config.Permissions.Check(ctx, toolName, input)
	if err != nil {
		return llm.ToolResult{
			ToolUseID: toolUseID,
			Content:   fmt.Sprintf("Error: permission check failed: %s", err),
		}, false
	}

	if permResult.Behavior != "allow" {
		msg := permResult.Message
		if msg == "" {
			msg = "permission denied"
		}
		return llm.ToolResult{
			ToolUseID: toolUseID,
			Content:   fmt.Sprintf("Error: %s", msg),
		}, permResult.Interrupt
	}

	// Use updated input if permission check modified it
	if permResult.UpdatedInput != nil {
		input = permResult.UpdatedInput
	}

	// Fire PreToolUse hook (ignore results for now — hooks are stub)
	config.Hooks.Fire(ctx, types.HookEventPreToolUse, map[string]any{
		"tool_name":   toolName,
		"tool_use_id": toolUseID,
		"tool_input":  input,
	})

	// Emit tool progress (start)
	emitToolProgress(ch, toolName, toolUseID, 0, state)

	// Execute the tool
	startTime := time.Now()
	output, err := tool.Execute(ctx, input)
	elapsed := time.Since(startTime).Seconds()

	// Emit tool progress (complete)
	emitToolProgress(ch, toolName, toolUseID, elapsed, state)

	if err != nil {
		// Fire PostToolUseFailure hook
		config.Hooks.Fire(ctx, types.HookEventPostToolUseFailure, map[string]any{
			"tool_name":   toolName,
			"tool_use_id": toolUseID,
			"error":       err.Error(),
		})

		return llm.ToolResult{
			ToolUseID: toolUseID,
			Content:   fmt.Sprintf("Error: %s", err),
		}, false
	}

	// Fire PostToolUse hook
	config.Hooks.Fire(ctx, types.HookEventPostToolUse, map[string]any{
		"tool_name":     toolName,
		"tool_use_id":   toolUseID,
		"tool_response": output.Content,
	})

	content := output.Content
	if output.IsError {
		content = "Error: " + content
	}

	return llm.ToolResult{
		ToolUseID: toolUseID,
		Content:   content,
	}, false
}
