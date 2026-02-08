package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

func boolPtr(b bool) *bool { return &b }

func TestRunner_ImplementsHookRunner(t *testing.T) {
	var _ agent.HookRunner = &Runner{}
}

func TestRunner_NoHooks(t *testing.T) {
	r := NewRunner(RunnerConfig{})
	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestRunner_NoMatchingEvent(t *testing.T) {
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventStop: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					return HookJSONOutput{Sync: &SyncHookJSONOutput{Decision: "approve"}}, nil
				}}},
			},
		},
	})
	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for non-matching event, got %v", results)
	}
}

func TestRunner_GoCallbackExecution(t *testing.T) {
	called := false
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					called = true
					return HookJSONOutput{Sync: &SyncHookJSONOutput{Decision: "approve"}}, nil
				}}},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{
		"tool_name": "Bash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("callback was not called")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Decision != "allow" {
		t.Errorf("decision = %q, want 'allow' (converted from 'approve')", results[0].Decision)
	}
}

func TestRunner_DecisionMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"approve", "allow"},
		{"block", "deny"},
		{"allow", "allow"},
		{"deny", "deny"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := NewRunner(RunnerConfig{
				Hooks: map[types.HookEvent][]CallbackMatcher{
					types.HookEventPreToolUse: {
						{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
							return HookJSONOutput{Sync: &SyncHookJSONOutput{Decision: tt.input}}, nil
						}}},
					},
				},
			})
			results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if results[0].Decision != tt.expected {
				t.Errorf("decision = %q, want %q", results[0].Decision, tt.expected)
			}
		})
	}
}

func TestRunner_MatcherExact(t *testing.T) {
	called := false
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{
					Matcher: "Bash",
					Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						called = true
						return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
					}},
				},
			},
		},
	})

	// Non-matching tool name
	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{
		"tool_name": "Grep",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("callback should not have been called for non-matching tool")
	}
	if results != nil {
		t.Errorf("expected nil results for non-matching tool, got %v", results)
	}

	// Matching tool name
	results, err = r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{
		"tool_name": "Bash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("callback should have been called for matching tool")
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestRunner_MatcherGlob(t *testing.T) {
	callCount := 0
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{
					Matcher: "mcp__*",
					Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						callCount++
						return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
					}},
				},
			},
		},
	})

	// Matches
	r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{"tool_name": "mcp__foo__bar"})
	if callCount != 1 {
		t.Errorf("expected 1 call for mcp__foo__bar, got %d", callCount)
	}

	// Doesn't match
	r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{"tool_name": "Bash"})
	if callCount != 1 {
		t.Errorf("expected still 1 call (Bash shouldn't match mcp__*), got %d", callCount)
	}
}

func TestRunner_MatcherEmpty(t *testing.T) {
	callCount := 0
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{
					Matcher: "", // matches all
					Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						callCount++
						return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
					}},
				},
			},
		},
	})

	r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{"tool_name": "Bash"})
	r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{"tool_name": "Grep"})
	r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{"tool_name": "mcp__foo"})

	if callCount != 3 {
		t.Errorf("expected 3 calls with empty matcher, got %d", callCount)
	}
}

func TestRunner_TypedInput(t *testing.T) {
	called := false
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{
					Matcher: "Bash",
					Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						called = true
						return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
					}},
				},
			},
		},
	})

	// Use typed struct instead of map
	r.Fire(context.Background(), types.HookEventPreToolUse, &PreToolUseHookInput{
		ToolName: "Bash",
	})
	if !called {
		t.Error("callback should match typed PreToolUseHookInput")
	}
}

func TestRunner_ContinueFalse(t *testing.T) {
	calls := []int{}
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{
					func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						calls = append(calls, 1)
						return HookJSONOutput{Sync: &SyncHookJSONOutput{Continue: boolPtr(false)}}, nil
					},
					func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						calls = append(calls, 2) // should NOT be reached
						return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
					},
				}},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 1 {
		t.Errorf("expected 1 call (second should be skipped), got %d: %v", len(calls), calls)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestRunner_ContinueTrue(t *testing.T) {
	calls := 0
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{
					func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						calls++
						return HookJSONOutput{Sync: &SyncHookJSONOutput{Continue: boolPtr(true)}}, nil
					},
					func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						calls++
						return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
					},
				}},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls (continue=true should not stop), got %d", calls)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestRunner_MultipleMatchers(t *testing.T) {
	calls := []string{}
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{
					Matcher: "Bash",
					Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						calls = append(calls, "bash-matcher")
						return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
					}},
				},
				{
					Matcher: "", // matches all
					Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						calls = append(calls, "all-matcher")
						return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
					}},
				},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{
		"tool_name": "Bash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Errorf("expected 2 calls (both matchers match Bash), got %d: %v", len(calls), calls)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestRunner_ErrorIsolation(t *testing.T) {
	calls := 0
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{
					func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						calls++
						return HookJSONOutput{}, fmt.Errorf("hook error")
					},
					func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						calls++
						return HookJSONOutput{Sync: &SyncHookJSONOutput{Decision: "approve"}}, nil
					},
				}},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls (error should not stop others), got %d", calls)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (errored hook skipped), got %d", len(results))
	}
	if results[0].Decision != "allow" {
		t.Errorf("decision = %q, want 'allow'", results[0].Decision)
	}
}

func TestRunner_Timeout(t *testing.T) {
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{
					Timeout: 1, // 1 second
					Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						select {
						case <-time.After(5 * time.Second):
							return HookJSONOutput{Sync: &SyncHookJSONOutput{Decision: "approve"}}, nil
						case <-ctx.Done():
							return HookJSONOutput{}, ctx.Err()
						}
					}},
				},
			},
		},
	})

	start := time.Now()
	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should complete quickly due to timeout, not wait 5 seconds
	if elapsed > 3*time.Second {
		t.Errorf("took %v, expected timeout around 1s", elapsed)
	}

	// Error from timeout is isolated — result should be empty
	if len(results) != 0 {
		t.Errorf("expected 0 results (timeout hook should be skipped), got %d", len(results))
	}
}

func TestRunner_SyncOutputFields(t *testing.T) {
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					return HookJSONOutput{Sync: &SyncHookJSONOutput{
						Decision:      "approve",
						SystemMessage: "injected context",
						Reason:        "test reason",
						StopReason:    "custom_stop",
						SuppressOutput: boolPtr(true),
						HookSpecificOutput: &PreToolUseSpecificOutput{
							PermissionDecision: "allow",
							AdditionalContext:  "extra",
						},
					}}, nil
				}}},
			},
		},
	})

	results, _ := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r0 := results[0]
	if r0.Decision != "allow" {
		t.Errorf("Decision = %q, want 'allow'", r0.Decision)
	}
	if r0.SystemMessage != "injected context" {
		t.Errorf("SystemMessage = %q, want 'injected context'", r0.SystemMessage)
	}
	if r0.Reason != "test reason" {
		t.Errorf("Reason = %q, want 'test reason'", r0.Reason)
	}
	if r0.StopReason != "custom_stop" {
		t.Errorf("StopReason = %q, want 'custom_stop'", r0.StopReason)
	}
	if r0.SuppressOutput == nil || !*r0.SuppressOutput {
		t.Error("SuppressOutput should be true")
	}
	specific, ok := r0.HookSpecificOutput.(*PreToolUseSpecificOutput)
	if !ok {
		t.Fatalf("HookSpecificOutput type = %T, want *PreToolUseSpecificOutput", r0.HookSpecificOutput)
	}
	if specific.PermissionDecision != "allow" {
		t.Errorf("specific.PermissionDecision = %q, want 'allow'", specific.PermissionDecision)
	}
	if specific.AdditionalContext != "extra" {
		t.Errorf("specific.AdditionalContext = %q, want 'extra'", specific.AdditionalContext)
	}
}

func TestRunner_NilSyncOutput(t *testing.T) {
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					return HookJSONOutput{}, nil // no sync or async
				}}},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Should be an empty result
	if results[0].Decision != "" {
		t.Errorf("expected empty decision for nil sync output, got %q", results[0].Decision)
	}
}

func TestRunner_ShellCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix shell")
	}

	// Create a temp script that echoes JSON
	dir := t.TempDir()
	script := filepath.Join(dir, "hook.sh")
	os.WriteFile(script, []byte(`#!/bin/sh
read INPUT
echo '{"decision":"approve","reason":"shell hook"}'
`), 0o755)

	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Commands: []string{script}},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{
		"tool_name": "Bash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Decision != "allow" {
		t.Errorf("decision = %q, want 'allow' (mapped from 'approve')", results[0].Decision)
	}
	if results[0].Reason != "shell hook" {
		t.Errorf("reason = %q, want 'shell hook'", results[0].Reason)
	}
}

func TestRunner_ShellCommandReceivesJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix shell")
	}

	dir := t.TempDir()
	outputFile := filepath.Join(dir, "input.json")
	script := filepath.Join(dir, "hook.sh")
	os.WriteFile(script, []byte(fmt.Sprintf(`#!/bin/sh
cat > %s
echo '{}'
`, outputFile)), 0o755)

	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Commands: []string{script}},
			},
		},
	})

	input := &PreToolUseHookInput{
		BaseHookInput: BaseHookInput{SessionID: "test-session"},
		ToolName:      "Bash",
		ToolUseID:     "call_123",
	}
	r.Fire(context.Background(), types.HookEventPreToolUse, input)

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var received map[string]any
	json.Unmarshal(data, &received)

	if received["tool_name"] != "Bash" {
		t.Errorf("shell received tool_name = %v, want 'Bash'", received["tool_name"])
	}
	if received["session_id"] != "test-session" {
		t.Errorf("shell received session_id = %v, want 'test-session'", received["session_id"])
	}
}

func TestRunner_ShellCommandError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix shell")
	}

	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Commands: []string{"exit 1"}},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error (should be isolated): %v", err)
	}
	// Error is isolated — no results
	if len(results) != 0 {
		t.Errorf("expected 0 results from errored shell hook, got %d", len(results))
	}
}

func TestRunner_MixedGoAndShellCallbacks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell tests require unix shell")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "hook.sh")
	os.WriteFile(script, []byte(`#!/bin/sh
cat > /dev/null
echo '{"decision":"approve","reason":"from shell"}'
`), 0o755)

	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{
					Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
						return HookJSONOutput{Sync: &SyncHookJSONOutput{Reason: "from go"}}, nil
					}},
					Commands: []string{script},
				},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results (1 go + 1 shell), got %d", len(results))
	}
	if results[0].Reason != "from go" {
		t.Errorf("results[0].Reason = %q, want 'from go'", results[0].Reason)
	}
	if results[1].Reason != "from shell" {
		t.Errorf("results[1].Reason = %q, want 'from shell'", results[1].Reason)
	}
}

func TestMatchToolName_ExactMatch(t *testing.T) {
	if !matchToolName("Bash", map[string]any{"tool_name": "Bash"}) {
		t.Error("should match exact tool name")
	}
	if matchToolName("Bash", map[string]any{"tool_name": "Grep"}) {
		t.Error("should not match different tool name")
	}
}

func TestMatchToolName_GlobMatch(t *testing.T) {
	if !matchToolName("mcp__*", map[string]any{"tool_name": "mcp__foo"}) {
		t.Error("should match glob pattern")
	}
	if matchToolName("mcp__*", map[string]any{"tool_name": "Bash"}) {
		t.Error("should not match glob pattern for non-matching name")
	}
}

func TestMatchToolName_EmptyPattern(t *testing.T) {
	if !matchToolName("", map[string]any{"tool_name": "anything"}) {
		t.Error("empty pattern should match everything")
	}
}

func TestMatchToolName_NoToolName(t *testing.T) {
	if !matchToolName("Bash", map[string]any{}) {
		t.Error("no tool_name in input should match (no filter possible)")
	}
}

func TestMatchToolName_TypedInput(t *testing.T) {
	if !matchToolName("Bash", &PreToolUseHookInput{ToolName: "Bash"}) {
		t.Error("should match typed struct")
	}
	if matchToolName("Bash", &PreToolUseHookInput{ToolName: "Grep"}) {
		t.Error("should not match different tool in typed struct")
	}
}

// --- Test Parity: Hook Event-Specific Tests (ported from Python Agent SDK) ---

func TestRunner_NotificationEvent(t *testing.T) {
	// Notification hook fires with message and notification_type
	var receivedInput any
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventNotification: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					receivedInput = input
					return HookJSONOutput{Sync: &SyncHookJSONOutput{Reason: "notified"}}, nil
				}}},
			},
		},
	})

	input := &NotificationHookInput{
		BaseHookInput:    BaseHookInput{SessionID: "test-session"},
		HookEventName:    "Notification",
		Message:          "Build completed",
		NotificationType: "build_status",
	}
	results, err := r.Fire(context.Background(), types.HookEventNotification, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Reason != "notified" {
		t.Errorf("reason = %q, want 'notified'", results[0].Reason)
	}
	// Verify typed input was passed through
	ni, ok := receivedInput.(*NotificationHookInput)
	if !ok {
		t.Fatalf("input type = %T, want *NotificationHookInput", receivedInput)
	}
	if ni.Message != "Build completed" {
		t.Errorf("input.Message = %q, want 'Build completed'", ni.Message)
	}
	if ni.NotificationType != "build_status" {
		t.Errorf("input.NotificationType = %q, want 'build_status'", ni.NotificationType)
	}
}

func TestRunner_PermissionRequestEvent(t *testing.T) {
	// PermissionRequest hook fires with tool_name and tool_input
	var receivedInput any
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPermissionRequest: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					receivedInput = input
					return HookJSONOutput{Sync: &SyncHookJSONOutput{Decision: "approve"}}, nil
				}}},
			},
		},
	})

	input := &PermissionRequestHookInput{
		BaseHookInput: BaseHookInput{SessionID: "test-session"},
		HookEventName: "PermissionRequest",
		ToolName:      "Bash",
		ToolInput:     map[string]any{"command": "rm -rf /"},
	}
	results, err := r.Fire(context.Background(), types.HookEventPermissionRequest, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Decision != "allow" {
		t.Errorf("decision = %q, want 'allow' (mapped from 'approve')", results[0].Decision)
	}
	pri, ok := receivedInput.(*PermissionRequestHookInput)
	if !ok {
		t.Fatalf("input type = %T, want *PermissionRequestHookInput", receivedInput)
	}
	if pri.ToolName != "Bash" {
		t.Errorf("input.ToolName = %q, want 'Bash'", pri.ToolName)
	}
}

func TestRunner_SubagentStartEvent(t *testing.T) {
	// SubagentStart hook fires with agent_id and agent_type
	var receivedInput any
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventSubagentStart: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					receivedInput = input
					return HookJSONOutput{Sync: &SyncHookJSONOutput{Reason: "subagent starting"}}, nil
				}}},
			},
		},
	})

	input := &SubagentStartHookInput{
		BaseHookInput: BaseHookInput{SessionID: "test-session"},
		HookEventName: "SubagentStart",
		AgentID:       "agent-explore-001",
		AgentType:     "Explore",
	}
	results, err := r.Fire(context.Background(), types.HookEventSubagentStart, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	si, ok := receivedInput.(*SubagentStartHookInput)
	if !ok {
		t.Fatalf("input type = %T, want *SubagentStartHookInput", receivedInput)
	}
	if si.AgentID != "agent-explore-001" {
		t.Errorf("input.AgentID = %q, want 'agent-explore-001'", si.AgentID)
	}
	if si.AgentType != "Explore" {
		t.Errorf("input.AgentType = %q, want 'Explore'", si.AgentType)
	}
}

func TestRunner_PostToolUseUpdatedMCPOutput(t *testing.T) {
	// PostToolUse hook output can contain updatedMCPToolOutput
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPostToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					return HookJSONOutput{Sync: &SyncHookJSONOutput{
						HookSpecificOutput: &PostToolUseSpecificOutput{
							HookEventName:        "PostToolUse",
							UpdatedMCPToolOutput: map[string]any{"cleaned": true, "redacted_fields": []string{"password"}},
						},
					}}, nil
				}}},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPostToolUse, &PostToolUseHookInput{
		ToolName: "mcp__db__query",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	specific, ok := results[0].HookSpecificOutput.(*PostToolUseSpecificOutput)
	if !ok {
		t.Fatalf("HookSpecificOutput type = %T, want *PostToolUseSpecificOutput", results[0].HookSpecificOutput)
	}
	updatedOutput, ok := specific.UpdatedMCPToolOutput.(map[string]any)
	if !ok {
		t.Fatalf("UpdatedMCPToolOutput type = %T, want map[string]any", specific.UpdatedMCPToolOutput)
	}
	if updatedOutput["cleaned"] != true {
		t.Errorf("cleaned = %v, want true", updatedOutput["cleaned"])
	}
}

func TestRunner_PermissionDecisionField(t *testing.T) {
	// PreToolUse hook output can contain permissionDecision
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					return HookJSONOutput{Sync: &SyncHookJSONOutput{
						HookSpecificOutput: &PreToolUseSpecificOutput{
							HookEventName:      "PreToolUse",
							PermissionDecision: "allow",
							PermissionDecisionReason: "whitelisted command",
						},
					}}, nil
				}}},
			},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, &PreToolUseHookInput{
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "ls"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	specific, ok := results[0].HookSpecificOutput.(*PreToolUseSpecificOutput)
	if !ok {
		t.Fatalf("HookSpecificOutput type = %T, want *PreToolUseSpecificOutput", results[0].HookSpecificOutput)
	}
	if specific.PermissionDecision != "allow" {
		t.Errorf("PermissionDecision = %q, want 'allow'", specific.PermissionDecision)
	}
	if specific.PermissionDecisionReason != "whitelisted command" {
		t.Errorf("PermissionDecisionReason = %q, want 'whitelisted command'", specific.PermissionDecisionReason)
	}
}

func TestConvertOutput_AsyncIgnored(t *testing.T) {
	// When only Async is set, convertOutput returns empty result
	result := convertOutput(HookJSONOutput{Async: &AsyncHookJSONOutput{Async: true}})
	if result.Decision != "" {
		t.Errorf("expected empty decision for async-only output, got %q", result.Decision)
	}
}

func TestRunner_EmitChannel(t *testing.T) {
	ch := make(chan types.SDKMessage, 100)

	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					return HookJSONOutput{Sync: &SyncHookJSONOutput{Decision: "approve"}}, nil
				}}},
			},
		},
		EmitChannel: ch,
		SessionID:   "test-session",
	})

	r.Fire(context.Background(), types.HookEventPreToolUse, map[string]any{"tool_name": "Bash"})

	// Drain channel
	close(ch)
	var msgs []types.SDKMessage
	for msg := range ch {
		msgs = append(msgs, msg)
	}

	// Should have emitted: HookStarted + HookResponse
	if len(msgs) != 2 {
		t.Fatalf("expected 2 emitted messages (started + response), got %d", len(msgs))
	}

	// First should be HookStarted
	started, ok := msgs[0].(*types.HookStartedMessage)
	if !ok {
		t.Fatalf("msgs[0] type = %T, want *HookStartedMessage", msgs[0])
	}
	if started.Subtype != types.SystemSubtypeHookStarted {
		t.Errorf("started.Subtype = %s, want hook_started", started.Subtype)
	}
	if started.HookEvent != "PreToolUse" {
		t.Errorf("started.HookEvent = %s, want PreToolUse", started.HookEvent)
	}
	if started.SessionID != "test-session" {
		t.Errorf("started.SessionID = %s, want test-session", started.SessionID)
	}

	// Second should be HookResponse
	response, ok := msgs[1].(*types.HookResponseMessage)
	if !ok {
		t.Fatalf("msgs[1] type = %T, want *HookResponseMessage", msgs[1])
	}
	if response.Subtype != types.SystemSubtypeHookResponse {
		t.Errorf("response.Subtype = %s, want hook_response", response.Subtype)
	}
	if response.Outcome != "success" {
		t.Errorf("response.Outcome = %s, want success", response.Outcome)
	}
}

func TestRunner_EmitChannelNil(t *testing.T) {
	// Verify no panic when emit channel is nil
	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
				}}},
			},
		},
		// No EmitChannel set
	})

	// Should not panic
	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestRunner_RegisterScoped(t *testing.T) {
	baseCalled := false
	scopedCalled := false

	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					baseCalled = true
					return HookJSONOutput{Sync: &SyncHookJSONOutput{Reason: "base"}}, nil
				}}},
			},
		},
	})

	// Register scoped hooks
	r.RegisterScoped("agent-1", map[types.HookEvent][]CallbackMatcher{
		types.HookEventPreToolUse: {
			{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
				scopedCalled = true
				return HookJSONOutput{Sync: &SyncHookJSONOutput{Reason: "scoped"}}, nil
			}}},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !baseCalled {
		t.Error("base hook should have been called")
	}
	if !scopedCalled {
		t.Error("scoped hook should have been called")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results (base + scoped), got %d", len(results))
	}
}

func TestRunner_UnregisterScoped(t *testing.T) {
	callCount := 0

	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					callCount++
					return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
				}}},
			},
		},
	})

	// Register and then unregister scoped hooks
	r.RegisterScoped("agent-1", map[types.HookEvent][]CallbackMatcher{
		types.HookEventPreToolUse: {
			{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
				callCount++
				return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
			}}},
		},
	})

	r.UnregisterScoped("agent-1")

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (only base, scoped removed), got %d", callCount)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestRunner_ScopedOnlyEvent(t *testing.T) {
	// Scoped hooks for an event that has no base hooks
	called := false
	r := NewRunner(RunnerConfig{})

	r.RegisterScoped("agent-1", map[types.HookEvent][]CallbackMatcher{
		types.HookEventSubagentStart: {
			{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
				called = true
				return HookJSONOutput{Sync: &SyncHookJSONOutput{Reason: "scoped-only"}}, nil
			}}},
		},
	})

	results, err := r.Fire(context.Background(), types.HookEventSubagentStart, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("scoped hook should have been called")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Reason != "scoped-only" {
		t.Errorf("reason = %q, want 'scoped-only'", results[0].Reason)
	}
}

func TestRunner_MultipleScopedHooks(t *testing.T) {
	callCount := 0
	r := NewRunner(RunnerConfig{})

	for _, id := range []string{"agent-1", "agent-2"} {
		r.RegisterScoped(id, map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					callCount++
					return HookJSONOutput{Sync: &SyncHookJSONOutput{}}, nil
				}}},
			},
		})
	}

	results, err := r.Fire(context.Background(), types.HookEventPreToolUse, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls from 2 scoped hooks, got %d", callCount)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestRunner_EmitChannelError(t *testing.T) {
	ch := make(chan types.SDKMessage, 100)

	r := NewRunner(RunnerConfig{
		Hooks: map[types.HookEvent][]CallbackMatcher{
			types.HookEventPreToolUse: {
				{Hooks: []HookCallback{func(input any, toolUseID string, ctx context.Context) (HookJSONOutput, error) {
					return HookJSONOutput{}, fmt.Errorf("hook failed")
				}}},
			},
		},
		EmitChannel: ch,
	})

	r.Fire(context.Background(), types.HookEventPreToolUse, nil)

	close(ch)
	var msgs []types.SDKMessage
	for msg := range ch {
		msgs = append(msgs, msg)
	}

	// Should have: HookStarted + HookResponse(error)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	response, ok := msgs[1].(*types.HookResponseMessage)
	if !ok {
		t.Fatalf("msgs[1] type = %T, want *HookResponseMessage", msgs[1])
	}
	if response.Outcome != "error" {
		t.Errorf("response.Outcome = %s, want error", response.Outcome)
	}
}
