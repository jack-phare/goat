package hooks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

// RunnerConfig configures a Runner.
type RunnerConfig struct {
	Hooks       map[types.HookEvent][]CallbackMatcher
	EmitChannel chan<- types.SDKMessage // optional: emit hook lifecycle messages
	SessionID   string
	CWD         string
}

// Runner manages hook registration and execution.
// It implements agent.HookRunner.
type Runner struct {
	hooks       map[types.HookEvent][]CallbackMatcher
	emitCh      chan<- types.SDKMessage
	sessionID   string
	cwd         string

	mu          sync.RWMutex
	scopedHooks map[string]map[types.HookEvent][]CallbackMatcher // scopeID → event → matchers
}

// NewRunner creates a Runner from configuration.
func NewRunner(config RunnerConfig) *Runner {
	hooks := config.Hooks
	if hooks == nil {
		hooks = make(map[types.HookEvent][]CallbackMatcher)
	}
	return &Runner{
		hooks:     hooks,
		emitCh:    config.EmitChannel,
		sessionID: config.SessionID,
		cwd:       config.CWD,
	}
}

// RegisterScoped adds hooks for a specific scope (e.g., a subagent ID).
// These hooks are merged with base hooks when Fire is called.
func (r *Runner) RegisterScoped(scopeID string, hookMap map[types.HookEvent][]CallbackMatcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.scopedHooks == nil {
		r.scopedHooks = make(map[string]map[types.HookEvent][]CallbackMatcher)
	}
	r.scopedHooks[scopeID] = hookMap
}

// UnregisterScoped removes hooks for a specific scope.
func (r *Runner) UnregisterScoped(scopeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.scopedHooks, scopeID)
}

// Fire executes all matching hooks for an event, collecting results.
// It implements agent.HookRunner.
func (r *Runner) Fire(ctx context.Context, event types.HookEvent, input any) ([]agent.HookResult, error) {
	// Merge base hooks with all scoped hooks for this event
	matchers := r.mergeMatchers(event)
	if len(matchers) == 0 {
		return nil, nil
	}

	var results []agent.HookResult

	for _, matcher := range matchers {
		// Check matcher pattern
		if matcher.Matcher != "" && !matchToolName(matcher.Matcher, input) {
			continue
		}

		// Apply timeout for this matcher
		hookCtx := ctx
		if matcher.Timeout > 0 {
			var cancel context.CancelFunc
			hookCtx, cancel = context.WithTimeout(ctx, time.Duration(matcher.Timeout)*time.Second)
			defer cancel()
		}

		// Run Go function callbacks
		stop := r.executeCallbacks(hookCtx, event, matcher.Hooks, input, &results)
		if stop {
			return results, nil
		}

		// Run shell command hooks
		stop = r.executeShellCommands(hookCtx, event, matcher.Commands, input, &results)
		if stop {
			return results, nil
		}
	}

	return results, nil
}

// executeCallbacks runs Go function callbacks sequentially, appending results.
// Returns true if processing should stop (continue=false).
func (r *Runner) executeCallbacks(ctx context.Context, event types.HookEvent, callbacks []HookCallback, input any, results *[]agent.HookResult) bool {
	for i, hook := range callbacks {
		hookID := fmt.Sprintf("%s-go-%d", event, i)
		hookName := fmt.Sprintf("%s callback %d", event, i)

		r.emitHookStarted(hookID, hookName, event)

		output, err := hook(input, "", ctx)
		if err != nil {
			r.emitHookResponse(hookID, hookName, event, "", "", "error")
			continue
		}

		// Handle async hooks: re-execute with async timeout
		if output.Async != nil && output.Async.Async {
			asyncOutput, asyncErr := executeAsync(ctx, hook, input, output.Async.AsyncTimeout)
			if asyncErr != nil {
				r.emitHookResponse(hookID, hookName, event, "", "", "error")
				continue
			}
			output = asyncOutput
		}

		r.emitHookResponse(hookID, hookName, event, "", "", "success")

		result := convertOutput(output)
		*results = append(*results, result)

		if result.Continue != nil && !*result.Continue {
			return true
		}
	}
	return false
}

// executeShellCommands runs shell command hooks sequentially, appending results.
// Returns true if processing should stop (continue=false).
func (r *Runner) executeShellCommands(ctx context.Context, event types.HookEvent, commands []string, input any, results *[]agent.HookResult) bool {
	for i, command := range commands {
		hookID := fmt.Sprintf("%s-shell-%d", event, i)
		hookName := command

		r.emitHookStarted(hookID, hookName, event)

		shellCB := ShellHookCallbackWithProgress(command, func(stdout, stderr string) {
			r.emitHookProgress(hookID, hookName, event, stdout, stderr)
		})
		output, err := shellCB(input, "", ctx)
		if err != nil {
			r.emitHookResponse(hookID, hookName, event, "", "", "error")
			continue
		}

		// Handle async hooks: re-execute with async timeout
		if output.Async != nil && output.Async.Async {
			asyncCB := ShellHookCallbackWithProgress(command, func(stdout, stderr string) {
				r.emitHookProgress(hookID, hookName, event, stdout, stderr)
			})
			asyncOutput, asyncErr := executeAsync(ctx, asyncCB, input, output.Async.AsyncTimeout)
			if asyncErr != nil {
				r.emitHookResponse(hookID, hookName, event, "", "", "error")
				continue
			}
			output = asyncOutput
		}

		r.emitHookResponse(hookID, hookName, event, "", "", "success")

		result := convertOutput(output)
		*results = append(*results, result)

		if result.Continue != nil && !*result.Continue {
			return true
		}
	}
	return false
}

// --- SDK Message Emission ---

func (r *Runner) emitHookStarted(hookID, hookName string, event types.HookEvent) {
	if r.emitCh == nil {
		return
	}
	r.emitCh <- &types.HookStartedMessage{
		BaseMessage: types.BaseMessage{UUID: uuid.New(), SessionID: r.sessionID},
		Type:        types.MessageTypeSystem,
		Subtype:     types.SystemSubtypeHookStarted,
		HookID:      hookID,
		HookName:    hookName,
		HookEvent:   string(event),
	}
}

func (r *Runner) emitHookProgress(hookID, hookName string, event types.HookEvent, stdout, stderr string) {
	if r.emitCh == nil {
		return
	}
	r.emitCh <- &types.HookProgressMessage{
		BaseMessage: types.BaseMessage{UUID: uuid.New(), SessionID: r.sessionID},
		Type:        types.MessageTypeSystem,
		Subtype:     types.SystemSubtypeHookProgress,
		HookID:      hookID,
		HookName:    hookName,
		HookEvent:   string(event),
		Stdout:      stdout,
		Stderr:      stderr,
		Output:      stdout + stderr,
	}
}

func (r *Runner) emitHookResponse(hookID, hookName string, event types.HookEvent, stdout, stderr, outcome string) {
	if r.emitCh == nil {
		return
	}
	r.emitCh <- &types.HookResponseMessage{
		BaseMessage: types.BaseMessage{UUID: uuid.New(), SessionID: r.sessionID},
		Type:        types.MessageTypeSystem,
		Subtype:     types.SystemSubtypeHookResponse,
		HookID:      hookID,
		HookName:    hookName,
		HookEvent:   string(event),
		Stdout:      stdout,
		Stderr:      stderr,
		Outcome:     outcome,
	}
}

// convertOutput converts a HookJSONOutput to an agent.HookResult.
func convertOutput(output HookJSONOutput) agent.HookResult {
	if output.Sync == nil {
		return agent.HookResult{}
	}

	s := output.Sync

	// Map decision values: "approve" → "allow", "block" → "deny"
	decision := s.Decision
	switch decision {
	case "approve":
		decision = "allow"
	case "block":
		decision = "deny"
	}

	return agent.HookResult{
		Decision:           decision,
		Message:            s.Reason,
		Continue:           s.Continue,
		SystemMessage:      s.SystemMessage,
		SuppressOutput:     s.SuppressOutput,
		StopReason:         s.StopReason,
		Reason:             s.Reason,
		HookSpecificOutput: s.HookSpecificOutput,
	}
}

// mergeMatchers combines base hooks with all scoped hooks for the given event.
func (r *Runner) mergeMatchers(event types.HookEvent) []CallbackMatcher {
	base := r.hooks[event]

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.scopedHooks) == 0 {
		return base
	}

	// Copy base matchers to avoid mutation
	merged := make([]CallbackMatcher, len(base))
	copy(merged, base)

	for _, hookMap := range r.scopedHooks {
		if scoped, ok := hookMap[event]; ok {
			merged = append(merged, scoped...)
		}
	}

	return merged
}

// Verify Runner implements agent.HookRunner at compile time.
var _ agent.HookRunner = (*Runner)(nil)
