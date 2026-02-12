package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// executeTools runs each tool_use block and returns tool result messages.
// If all tools are side-effect-free and there are multiple blocks, they
// execute concurrently up to MaxParallelTools. Otherwise, serial execution.
// If interrupted is true, the caller should stop the loop.
func executeTools(ctx context.Context, toolBlocks []types.ContentBlock, config *AgentConfig, state *LoopState, ch chan<- types.SDKMessage) (results []llm.ToolResult, interrupted bool) {
	maxConcurrency := 5
	if config.MaxParallelTools > 0 {
		maxConcurrency = config.MaxParallelTools
	}

	if len(toolBlocks) > 1 && canRunParallel(toolBlocks, config.ToolRegistry) {
		return executeToolsParallel(ctx, toolBlocks, config, state, ch, maxConcurrency)
	}
	return executeToolsSerial(ctx, toolBlocks, config, state, ch)
}

// canRunParallel returns true if all tool blocks reference side-effect-free tools.
func canRunParallel(toolBlocks []types.ContentBlock, registry *tools.Registry) bool {
	if registry == nil {
		return false
	}
	for _, block := range toolBlocks {
		tool, ok := registry.Get(block.Name)
		if !ok {
			return false // unknown tool, play safe
		}
		if tool.SideEffect() != tools.SideEffectNone {
			return false
		}
	}
	return true
}

// executeToolsSerial runs tools one at a time (the original behavior).
func executeToolsSerial(ctx context.Context, toolBlocks []types.ContentBlock, config *AgentConfig, state *LoopState, ch chan<- types.SDKMessage) (results []llm.ToolResult, interrupted bool) {
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

// executeToolsParallel runs side-effect-free tools concurrently with a semaphore.
func executeToolsParallel(ctx context.Context, toolBlocks []types.ContentBlock, config *AgentConfig, state *LoopState, ch chan<- types.SDKMessage, maxConcurrency int) ([]llm.ToolResult, bool) {
	results := make([]llm.ToolResult, len(toolBlocks))
	var interrupted atomic.Bool

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var contextMu sync.Mutex
	var allAdditionalContext []string

	for i, block := range toolBlocks {
		if interrupted.Load() {
			results[i] = llm.ToolResult{
				ToolUseID: block.ID,
				Content:   "Error: execution interrupted",
			}
			continue
		}

		wg.Add(1)
		sem <- struct{}{} // acquire semaphore
		go func(idx int, blk types.ContentBlock) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore

			result, permInterrupt := executeSingleToolParallel(ctx, blk, config, &contextMu, &allAdditionalContext, ch, state)
			results[idx] = result
			if permInterrupt {
				interrupted.Store(true)
			}
		}(i, block)
	}

	wg.Wait()

	// Merge collected context into state
	state.PendingAdditionalContext = append(state.PendingAdditionalContext, allAdditionalContext...)

	// Fill any remaining empty results if interrupted
	if interrupted.Load() {
		for i := range results {
			if results[i].ToolUseID == "" {
				results[i] = llm.ToolResult{
					ToolUseID: toolBlocks[i].ID,
					Content:   "Error: execution interrupted by permission check",
				}
			}
		}
		return results, true
	}
	return results, false
}

// executeSingleToolParallel is a parallel-safe variant of executeSingleTool.
// It uses a mutex for shared context collection and records file access
// under a lock to protect the shared LoopState.
func executeSingleToolParallel(ctx context.Context, block types.ContentBlock, config *AgentConfig, contextMu *sync.Mutex, allAdditionalContext *[]string, ch chan<- types.SDKMessage, state *LoopState) (llm.ToolResult, bool) {
	toolName := block.Name
	toolUseID := block.ID
	input := block.Input

	tool, ok := config.ToolRegistry.Get(toolName)
	if !ok {
		return llm.ToolResult{
			ToolUseID: toolUseID,
			Content:   fmt.Sprintf("Error: unknown tool %q", toolName),
		}, false
	}

	// Check permissions (apply skill scope if active)
	checker := effectivePermissionChecker(config.Permissions, state)
	permResult, err := checker.Check(ctx, toolName, input)
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
	if permResult.UpdatedInput != nil {
		input = permResult.UpdatedInput
	}

	// Fire PreToolUse hook
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
	// Collect hook context under lock
	contextMu.Lock()
	for _, r := range preResults {
		if r.SystemMessage != "" {
			*allAdditionalContext = append(*allAdditionalContext, r.SystemMessage)
		}
	}
	contextMu.Unlock()

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
		failResults, _ := config.Hooks.Fire(ctx, types.HookEventPostToolUseFailure, map[string]any{
			"tool_name":   toolName,
			"tool_use_id": toolUseID,
			"error":       err.Error(),
		})
		contextMu.Lock()
		for _, r := range failResults {
			if r.SystemMessage != "" {
				*allAdditionalContext = append(*allAdditionalContext, r.SystemMessage)
			}
		}
		contextMu.Unlock()

		return llm.ToolResult{
			ToolUseID: toolUseID,
			Content:   fmt.Sprintf("Error: %s", err),
		}, false
	}

	// Record file access under lock (shared state)
	contextMu.Lock()
	recordToolFileAccess(state, toolName, input)
	contextMu.Unlock()

	// Fire PostToolUse hook
	postResults, _ := config.Hooks.Fire(ctx, types.HookEventPostToolUse, map[string]any{
		"tool_name":     toolName,
		"tool_use_id":   toolUseID,
		"tool_response": output.Content,
	})
	contextMu.Lock()
	for _, r := range postResults {
		if r.SystemMessage != "" {
			*allAdditionalContext = append(*allAdditionalContext, r.SystemMessage)
		}
	}
	contextMu.Unlock()

	content := output.Content
	if output.IsError {
		content = "Error: " + content
	}
	if shouldSuppressOutput(postResults) {
		content = "[output suppressed by hook]"
	}

	return llm.ToolResult{
		ToolUseID: toolUseID,
		Content:   content,
	}, false
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

	// Check permissions (apply skill scope if active)
	checker := effectivePermissionChecker(config.Permissions, state)
	permResult, err := checker.Check(ctx, toolName, input)
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

	// Record file access for tracking
	recordToolFileAccess(state, toolName, input)

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

// effectivePermissionChecker returns the appropriate permission checker,
// wrapping it with skill scope if an active skill has allowed-tools.
func effectivePermissionChecker(base PermissionChecker, state *LoopState) PermissionChecker {
	if state.ActiveSkill != nil && len(state.ActiveSkill.AllowedTools) > 0 {
		return &skillPermissionWrapper{
			allowedTools: state.ActiveSkill.AllowedTools,
			inner:        base,
		}
	}
	return base
}

// skillPermissionWrapper wraps a PermissionChecker to auto-allow tools
// that match a skill's allowed-tools patterns.
type skillPermissionWrapper struct {
	allowedTools []string
	inner        PermissionChecker
}

func (w *skillPermissionWrapper) Check(ctx context.Context, toolName string, input map[string]any) (PermissionResult, error) {
	for _, pattern := range w.allowedTools {
		if matchSkillToolPattern(pattern, toolName, input) {
			return PermissionResult{Behavior: "allow"}, nil
		}
	}
	return w.inner.Check(ctx, toolName, input)
}

// matchSkillToolPattern matches a single allowed-tools pattern against a tool name and input.
// Supports exact names ("Bash"), globs ("mcp__*"), and constrained patterns ("Bash(gh:*)").
func matchSkillToolPattern(pattern, toolName string, input map[string]any) bool {
	if parenIdx := strings.Index(pattern, "("); parenIdx >= 0 {
		namePattern := pattern[:parenIdx]
		constraint := strings.TrimSuffix(pattern[parenIdx+1:], ")")
		if !matchSkillName(namePattern, toolName) {
			return false
		}
		return matchSkillConstraint(constraint, toolName, input)
	}
	return matchSkillName(pattern, toolName)
}

func matchSkillName(pattern, name string) bool {
	if pattern == name {
		return true
	}
	matched, _ := filepath.Match(pattern, name)
	return matched
}

func matchSkillConstraint(constraint, toolName string, input map[string]any) bool {
	var value string
	switch toolName {
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			value = cmd
		}
	default:
		for _, key := range []string{"command", "path", "file_path", "url"} {
			if v, ok := input[key].(string); ok {
				value = v
				break
			}
		}
	}
	if value == "" {
		return false
	}
	if strings.HasSuffix(constraint, ":*") {
		prefix := strings.TrimSuffix(constraint, ":*")
		return strings.HasPrefix(value, prefix+" ") || value == prefix
	}
	if constraint == value {
		return true
	}
	matched, _ := filepath.Match(constraint, value)
	return matched
}

// setActiveSkillScope checks if any tool block is a Skill invocation and sets
// the active skill scope on the loop state for subsequent permission checks.
func setActiveSkillScope(toolBlocks []types.ContentBlock, config *AgentConfig, state *LoopState) {
	if config.Skills == nil {
		return
	}
	for _, block := range toolBlocks {
		if block.Name != "Skill" {
			continue
		}
		skillName, ok := block.Input["skill"].(string)
		if !ok || skillName == "" {
			continue
		}
		entry, found := config.Skills.GetSkill(skillName)
		if found && len(entry.AllowedTools) > 0 {
			state.ActiveSkill = &SkillScope{
				SkillName:    skillName,
				AllowedTools: entry.AllowedTools,
			}
			return // use the first Skill invocation found
		}
	}
}

// recordToolFileAccess extracts file paths from tool input and records them in state.
// It also updates config.ActiveFilePaths for conditional rules injection.
func recordToolFileAccess(state *LoopState, toolName string, input map[string]any) {
	opMap := map[string]string{
		"Read":         "read",
		"Write":        "write",
		"Edit":         "edit",
		"Glob":         "glob",
		"Grep":         "grep",
		"Bash":         "exec",
		"NotebookEdit": "edit",
	}
	op, tracked := opMap[toolName]
	if !tracked {
		return
	}
	if path, ok := input["file_path"].(string); ok && path != "" {
		state.RecordFileAccess(path, op)
	}
	if path, ok := input["notebook_path"].(string); ok && path != "" {
		state.RecordFileAccess(path, op)
	}
	if path, ok := input["path"].(string); ok && path != "" {
		state.RecordFileAccess(path, op)
	}
}

// syncActiveFilePaths updates config.ActiveFilePaths from state.AccessedFiles.
func syncActiveFilePaths(config *AgentConfig, state *LoopState) {
	if state.AccessedFiles == nil {
		return
	}
	paths := make([]string, 0, len(state.AccessedFiles))
	for p := range state.AccessedFiles {
		paths = append(paths, p)
	}
	config.ActiveFilePaths = paths
}
