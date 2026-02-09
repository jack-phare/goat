package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
)

// ProgressFunc is called with stdout/stderr lines during shell hook execution.
type ProgressFunc func(stdout, stderr string)

// ShellHookCallback creates a HookCallback that wraps a shell command.
// The command receives hook input as JSON on stdin and returns HookJSONOutput as JSON on stdout.
func ShellHookCallback(command string) HookCallback {
	return ShellHookCallbackWithProgress(command, nil)
}

// ShellHookCallbackWithProgress creates a HookCallback with optional progress reporting.
func ShellHookCallbackWithProgress(command string, onProgress ProgressFunc) HookCallback {
	return func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
		inputJSON, err := json.Marshal(input)
		if err != nil {
			return HookJSONOutput{}, err
		}

		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Stdin = bytes.NewReader(inputJSON)

		var stdout, stderr bytes.Buffer

		if onProgress != nil {
			// Use TeeReader to capture output while also reporting progress line-by-line
			stdoutPipe, pipeErr := cmd.StdoutPipe()
			if pipeErr != nil {
				return HookJSONOutput{}, pipeErr
			}
			stderrPipe, pipeErr := cmd.StderrPipe()
			if pipeErr != nil {
				return HookJSONOutput{}, pipeErr
			}

			if err := cmd.Start(); err != nil {
				return HookJSONOutput{}, err
			}

			// Read stderr in background
			stderrDone := make(chan struct{})
			go func() {
				defer close(stderrDone)
				buf := make([]byte, 4096)
				for {
					n, readErr := stderrPipe.Read(buf)
					if n > 0 {
						chunk := string(buf[:n])
						stderr.WriteString(chunk)
						onProgress("", chunk)
					}
					if readErr != nil {
						break
					}
				}
			}()

			// Read stdout
			buf := make([]byte, 4096)
			for {
				n, readErr := stdoutPipe.Read(buf)
				if n > 0 {
					chunk := string(buf[:n])
					stdout.WriteString(chunk)
					onProgress(chunk, "")
				}
				if readErr != nil {
					break
				}
			}

			<-stderrDone
			if err := cmd.Wait(); err != nil {
				return HookJSONOutput{}, err
			}
		} else {
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return HookJSONOutput{}, err
			}
		}

		outBytes := stdout.Bytes()
		if len(outBytes) == 0 {
			// Empty stdout = no output, treat as no-op sync result
			return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
		}

		// Try parsing as async first (check for "async" field)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(outBytes, &raw); err != nil {
			return HookJSONOutput{}, err
		}

		if _, hasAsync := raw["async"]; hasAsync {
			var asyncOut AsyncHookJSONOutput
			if err := json.Unmarshal(outBytes, &asyncOut); err == nil && asyncOut.Async {
				return HookJSONOutput{Async: &asyncOut}, nil
			}
		}

		// Parse as sync
		var syncOut SyncHookJSONOutput
		if err := json.Unmarshal(outBytes, &syncOut); err != nil {
			return HookJSONOutput{}, err
		}

		return HookJSONOutput{Sync: &syncOut}, nil
	}
}
