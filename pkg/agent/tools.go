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

	// Fire PreToolUse hook and process results
	preResults, _ := config.Hooks.Fire(ctx, types.HookEventPreToolUse, map[string]any{
		"tool_name":   toolName,
		"tool_use_id": toolUseID,
		"tool_input":  input,
	})
	if decision, reason := processPreToolUseResults(preResults); decision != "" {
		if decision == "deny" {
			msg := reason
			if msg == "" {
				msg = "denied by hook"
			}
			return llm.ToolResult{
				ToolUseID: toolUseID,
				Content:   fmt.Sprintf("Error: %s", msg),
			}, false
		}
	}
	// Collect additional context from PreToolUse hooks
	collectAdditionalContext(state, preResults)
	// Check for updated input from hooks
	if updatedInput := getUpdatedInputFromHookResults(preResults); updatedInput != nil {
		input = updatedInput
	}

	// Emit tool progress (start)
	emitToolProgress(ch, toolName, toolUseID, 0, state)

	// Execute the tool
	startTime := time.Now()
	output, err := tool.Execute(ctx, input)
	elapsed := time.Since(startTime).Seconds()

	// Emit tool progress (complete)
	emitToolProgress(ch, toolName, toolUseID, elapsed, state)

	if err != nil {
		// Fire PostToolUseFailure hook and collect context
		failResults, _ := config.Hooks.Fire(ctx, types.HookEventPostToolUseFailure, map[string]any{
			"tool_name":   toolName,
			"tool_use_id": toolUseID,
			"error":       err.Error(),
		})
		collectAdditionalContext(state, failResults)

		return llm.ToolResult{
			ToolUseID: toolUseID,
			Content:   fmt.Sprintf("Error: %s", err),
		}, false
	}

	// Fire PostToolUse hook and collect context
	postResults, _ := config.Hooks.Fire(ctx, types.HookEventPostToolUse, map[string]any{
		"tool_name":     toolName,
		"tool_use_id":   toolUseID,
		"tool_response": output.Content,
	})
	collectAdditionalContext(state, postResults)

	content := output.Content
	if output.IsError {
		content = "Error: " + content
	}

	// Check suppress output from hooks
	if shouldSuppressOutput(postResults) {
		content = "[output suppressed by hook]"
	}

	return llm.ToolResult{
		ToolUseID: toolUseID,
		Content:   content,
	}, false
}

// processPreToolUseResults checks hook results for permission decisions.
// Returns ("deny", reason) if denied, ("allow", "") if allowed, ("", "") if no decision.
func processPreToolUseResults(results []HookResult) (string, string) {
	for _, r := range results {
		if r.Decision == "deny" {
			return "deny", r.Message
		}
		// Check hook-specific output for permission decisions
		if specific, ok := r.HookSpecificOutput.(interface{ GetPermissionDecision() string }); ok {
			decision := specific.GetPermissionDecision()
			if decision == "deny" {
				return "deny", r.Reason
			}
		}
	}
	for _, r := range results {
		if r.Decision == "allow" {
			return "allow", ""
		}
	}
	return "", "" // no decision
}

// getUpdatedInputFromHookResults checks for updatedInput from PreToolUse hook-specific output.
func getUpdatedInputFromHookResults(results []HookResult) map[string]any {
	for _, r := range results {
		if m, ok := r.HookSpecificOutput.(map[string]any); ok {
			if updatedInput, ok := m["updatedInput"].(map[string]any); ok {
				return updatedInput
			}
		}
	}
	return nil
}

// shouldSuppressOutput checks if any hook result requests output suppression.
func shouldSuppressOutput(results []HookResult) bool {
	for _, r := range results {
		if r.SuppressOutput != nil && *r.SuppressOutput {
			return true
		}
	}
	return false
}
