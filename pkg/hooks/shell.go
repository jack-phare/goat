package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
)

// ShellHookCallback creates a HookCallback that wraps a shell command.
// The command receives hook input as JSON on stdin and returns HookJSONOutput as JSON on stdout.
func ShellHookCallback(command string) HookCallback {
	return func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
		inputJSON, err := json.Marshal(input)
		if err != nil {
			return HookJSONOutput{}, err
		}

		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Stdin = bytes.NewReader(inputJSON)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return HookJSONOutput{}, err
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
