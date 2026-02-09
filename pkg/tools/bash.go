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
	return `Executes a given bash command with optional timeout. Working directory persists between commands; shell state (everything else) does not. The shell environment is initialized from the user's profile (bash or zsh).

IMPORTANT: This tool is for terminal operations like git, npm, docker, etc. DO NOT use it for file operations (reading, writing, editing, searching, finding files) - use the specialized tools for this instead.

Before executing the command, please follow these steps:

1. Directory Verification:
   - If the command will create new directories or files, first use ` + "`ls`" + ` to verify the parent directory exists and is the correct location
   - For example, before running "mkdir foo/bar", first use ` + "`ls foo`" + ` to check that "foo" exists and is the intended parent directory

2. Command Execution:
   - Always quote file paths that contain spaces with double quotes (e.g., cd "path with spaces/file.txt")
   - Examples of proper quoting:
     - cd "/Users/name/My Documents" (correct)
     - cd /Users/name/My Documents (incorrect - will fail)
     - python "/path/with spaces/script.py" (correct)
     - python /path/with spaces/script.py (incorrect - will fail)
   - After ensuring proper quoting, execute the command.
   - Capture the output of the command.

Usage notes:
  - The command argument is required.
  - You can specify an optional timeout in milliseconds (up to 600000ms / 10 minutes). If not specified, commands will timeout after 120000ms (2 minutes).
  - It is very helpful if you write a clear, concise description of what this command does. For simple commands, keep it brief (5-10 words). For complex commands (piped commands, obscure flags, or anything hard to understand at a glance), add enough context to clarify what it does.
  - If the output exceeds 30000 characters, output will be truncated before being returned to you.

  - You can use the ` + "`run_in_background`" + ` parameter to run the command in the background. Only use this if you don't need the result immediately and are OK being notified when the command completes later. You do not need to check the output right away - you'll be notified when it finishes. You do not need to use '&' at the end of the command when using this parameter.

  - Avoid using Bash with the ` + "`find`" + `, ` + "`grep`" + `, ` + "`cat`" + `, ` + "`head`" + `, ` + "`tail`" + `, ` + "`sed`" + `, ` + "`awk`" + `, or ` + "`echo`" + ` commands, unless explicitly instructed or when these commands are truly necessary for the task. Instead, always prefer using the dedicated tools for these commands:
    - File search: Use Glob (NOT find or ls)
    - Content search: Use Grep (NOT grep or rg)
    - Read files: Use Read (NOT cat/head/tail)
    - Edit files: Use Edit (NOT sed/awk)
    - Write files: Use Write (NOT echo >/cat <<EOF)
    - Communication: Output text directly (NOT echo/printf)
  - When issuing multiple commands:
    - If the commands are independent and can run in parallel, make multiple Bash tool calls in a single message. For example, if you need to run "git status" and "git diff", send a single message with two Bash tool calls in parallel.
    - If the commands depend on each other and must run sequentially, use a single Bash call with '&&' to chain them together (e.g., ` + "`git add . && git commit -m \"message\" && git push`" + `). For instance, if one operation must complete before another starts (like mkdir before cp, Write before Bash for git operations, or git add before git commit), run these operations sequentially instead.
    - Use ';' only when you need to run commands sequentially but don't care if earlier commands fail
    - DO NOT use newlines to separate commands (newlines are ok in quoted strings)
  - Try to maintain your current working directory throughout the session by using absolute paths and avoiding usage of ` + "`cd`" + `. You may use ` + "`cd`" + ` if the User explicitly requests it.`
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
