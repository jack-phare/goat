package permission

import (
	"context"
	"errors"
	"sync"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/types"
)

// Checker evaluates permissions for tool invocations.
// It implements agent.PermissionChecker.
type Checker struct {
	mu sync.RWMutex

	mode          types.PermissionMode
	allowedTools  map[string]bool
	disabledTools map[string]bool

	configRules  []PermissionRule // from settings files
	sessionRules []PermissionRule // accumulated during session

	allowDangerouslySkipPermissions bool

	// Hook & callback integration
	hookRunner   agent.HookRunner
	canUseTool   types.CanUseToolFunc
	userPrompter UserPrompter
}

// NewChecker creates a permission Checker from configuration.
func NewChecker(config CheckerConfig) *Checker {
	allowed := make(map[string]bool, len(config.AllowedTools))
	for _, name := range config.AllowedTools {
		allowed[name] = true
	}

	disabled := make(map[string]bool, len(config.DisabledTools))
	for _, name := range config.DisabledTools {
		disabled[name] = true
	}

	mode := types.PermissionMode(config.Mode)
	if mode == "" {
		mode = types.PermissionModeDefault
	}

	return &Checker{
		mode:                            mode,
		allowedTools:                    allowed,
		disabledTools:                   disabled,
		configRules:                     config.Rules,
		allowDangerouslySkipPermissions: config.AllowDangerouslySkipPermissions,
		hookRunner:                      config.HookRunner,
		canUseTool:                      config.CanUseTool,
		userPrompter:                    config.UserPrompter,
	}
}

// Check evaluates whether a tool invocation is permitted.
// Layers: mode → disabled → allowed → rules → hook → callback → mode default → prompter.
func (c *Checker) Check(ctx context.Context, toolName string, input map[string]any) (agent.PermissionResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Layer 1: Mode check
	switch c.mode {
	case types.PermissionModeBypassPermissions:
		if !c.allowDangerouslySkipPermissions {
			return agent.PermissionResult{}, errors.New("bypassPermissions mode requires AllowDangerouslySkipPermissions to be true")
		}
		return agent.PermissionResult{Behavior: "allow"}, nil

	case types.PermissionModePlan:
		return agent.PermissionResult{
			Behavior: "deny",
			Message:  "tool execution is not allowed in plan mode",
		}, nil

	case types.PermissionModeDelegate:
		if toolName != "Agent" {
			return agent.PermissionResult{
				Behavior: "deny",
				Message:  "only Agent tool is allowed in delegate mode",
			}, nil
		}
		return agent.PermissionResult{Behavior: "allow"}, nil
	}

	// Layer 2: Disabled check
	if c.disabledTools[toolName] {
		return agent.PermissionResult{
			Behavior: "deny",
			Message:  "tool is disabled",
		}, nil
	}

	// Layer 3: Allowed check
	if c.allowedTools[toolName] {
		return agent.PermissionResult{Behavior: "allow"}, nil
	}

	// Layer 4: Rules check
	if result, matched := c.checkRules(toolName, input); matched {
		if result.Behavior == "allow" || result.Behavior == "deny" {
			return result, nil
		}
		// "ask" from rules falls through to hook/callback
	}

	// Layer 5: Hook check (PermissionRequest hook)
	if c.hookRunner != nil {
		hookResult, err := c.firePermissionHook(ctx, toolName, input)
		if err == nil && hookResult != nil {
			return *hookResult, nil
		}
	}

	// Layer 6: Callback check (canUseTool)
	if c.canUseTool != nil {
		cbResult, err := c.canUseTool(toolName, input)
		if err == nil && cbResult != nil {
			return agent.PermissionResult{
				Behavior:           cbResult.Behavior,
				UpdatedInput:       cbResult.UpdatedInput,
				UpdatedPermissions: cbResult.Permissions,
				Message:            cbResult.Message,
			}, nil
		}
	}

	// Layer 7: Mode default
	behavior := DefaultBehaviorForTool(c.mode, toolName)

	// If mode default says "ask", try the user prompter
	if behavior == BehaviorAsk {
		if c.userPrompter != nil {
			return c.userPrompter.PromptForPermission(toolName, input, nil)
		}
		// No prompter: deny in headless mode
		return agent.PermissionResult{
			Behavior: "deny",
			Message:  "permission denied (no interactive prompter available)",
		}, nil
	}

	result := agent.PermissionResult{Behavior: string(behavior)}
	if behavior == BehaviorDeny {
		result.Message = "denied by mode default"
	}
	return result, nil
}

// firePermissionHook fires the PermissionRequest hook and interprets the result.
// Returns nil if hook didn't provide a decision (continue).
func (c *Checker) firePermissionHook(ctx context.Context, toolName string, input map[string]any) (*agent.PermissionResult, error) {
	results, err := c.hookRunner.Fire(ctx, types.HookEventPermissionRequest, map[string]any{
		"tool_name":  toolName,
		"tool_input": input,
	})
	if err != nil {
		return nil, err
	}

	for _, hr := range results {
		switch hr.Decision {
		case "allow":
			return &agent.PermissionResult{Behavior: "allow"}, nil
		case "deny":
			msg := hr.Message
			if msg == "" {
				msg = "denied by hook"
			}
			return &agent.PermissionResult{Behavior: "deny", Message: msg}, nil
		default:
			// "" or "continue" — fall through
			continue
		}
	}

	return nil, nil // no decision from hooks
}

// checkRules evaluates config rules then session rules.
func (c *Checker) checkRules(toolName string, input map[string]any) (agent.PermissionResult, bool) {
	for _, rule := range c.configRules {
		if rule.Matches(toolName, input) {
			return agent.PermissionResult{
				Behavior: string(rule.Behavior),
				Message:  ruleMessage(rule),
			}, true
		}
	}

	for _, rule := range c.sessionRules {
		if rule.Matches(toolName, input) {
			return agent.PermissionResult{
				Behavior: string(rule.Behavior),
				Message:  ruleMessage(rule),
			}, true
		}
	}

	return agent.PermissionResult{}, false
}

func ruleMessage(rule PermissionRule) string {
	if rule.Behavior == BehaviorDeny {
		return "denied by permission rule"
	}
	return ""
}

// SetMode changes the permission mode.
func (c *Checker) SetMode(mode types.PermissionMode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mode = mode
}

// Mode returns the current permission mode.
func (c *Checker) Mode() types.PermissionMode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mode
}
