package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	bashDefaultTimeout = 120 * time.Second
	bashMaxTimeout     = 600 * time.Second
	bashMaxOutput      = 30000 // characters
)

// BashTool executes shell commands.
type BashTool struct {
	CWD         string       // working directory for command execution
	TaskManager *TaskManager // optional, for run_in_background support
}

func (b *BashTool) Name() string { return "Bash" }

func (b *BashTool) Description() string {
	return "Executes a given bash command with optional timeout.\n\nIMPORTANT: This tool is for terminal operations."
}

func (b *BashTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The command to execute",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Optional timeout in milliseconds (max 600000)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Clear, concise description of what this command does",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "Set to true to run this command in the background",
			},
		},
		"required": []string{"command"},
	}
}

func (b *BashTool) SideEffect() SideEffectType { return SideEffectMutating }

func (b *BashTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return ToolOutput{Content: "Error: command is required", IsError: true}, nil
	}

	// Handle run_in_background
	if bg, ok := input["run_in_background"].(bool); ok && bg {
		return b.executeBackground(ctx, command, input)
	}

	return b.executeForeground(ctx, command, input)
}

func (b *BashTool) executeBackground(ctx context.Context, command string, input map[string]any) (ToolOutput, error) {
	if b.TaskManager == nil {
		return ToolOutput{Content: "Error: background execution not available (no task manager configured)", IsError: true}, nil
	}

	taskID := generateID()
	cwd := b.CWD

	timeout := bashDefaultTimeout
	if t, ok := input["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t) * time.Millisecond
		if timeout > bashMaxTimeout {
			timeout = bashMaxTimeout
		}
	}

	b.TaskManager.Launch(ctx, taskID, func(taskCtx context.Context) (string, error) {
		taskCtx, cancel := context.WithTimeout(taskCtx, timeout)
		defer cancel()

		cmd := exec.CommandContext(taskCtx, "bash", "-c", command)
		if cwd != "" {
			cmd.Dir = cwd
		}

		output, err := cmd.CombinedOutput()
		result := string(output)

		if len(result) > bashMaxOutput {
			result = result[:bashMaxOutput] + fmt.Sprintf(
				"\n... (truncated, %d total characters. Consider using head/tail or piping to limit output)",
				len(string(output)))
		}

		if err != nil {
			if taskCtx.Err() == context.DeadlineExceeded {
				return fmt.Sprintf("Error: command timed out after %s\n%s", timeout, result), err
			}
			return strings.TrimRight(result, "\n"), err
		}

		return strings.TrimRight(result, "\n"), nil
	})

	return ToolOutput{
		Content: fmt.Sprintf("Task started in background with ID: %s\nUse TaskOutput tool with task_id=%q to check results.", taskID, taskID),
	}, nil
}

func (b *BashTool) executeForeground(ctx context.Context, command string, input map[string]any) (ToolOutput, error) {
	timeout := bashDefaultTimeout
	if t, ok := input["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t) * time.Millisecond
		if timeout > bashMaxTimeout {
			timeout = bashMaxTimeout
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if b.CWD != "" {
		cmd.Dir = b.CWD
	}

	output, err := cmd.CombinedOutput()
	result := string(output)

	// Truncate large output
	if len(result) > bashMaxOutput {
		result = result[:bashMaxOutput] + fmt.Sprintf(
			"\n... (truncated, %d total characters. Consider using head/tail, piping to limit output, or running in background with run_in_background parameter)",
			len(string(output)))
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ToolOutput{
				Content: fmt.Sprintf("Error: command timed out after %s\n%s", timeout, result),
				IsError: true,
			}, nil
		}
		// Non-zero exit code — include output with the error
		return ToolOutput{
			Content: strings.TrimRight(result, "\n"),
			IsError: true,
		}, nil
	}

	return ToolOutput{Content: strings.TrimRight(result, "\n")}, nil
}
