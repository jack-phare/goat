# Spec 06: Permission System

**Go Package**: `pkg/permission/`
**Source References**:
- `sdk.d.ts:824-897` — PermissionMode, PermissionBehavior, PermissionResult, PermissionUpdate, PermissionUpdateDestination
- `sdk.d.ts:504` — `allowedTools` (auto-allowed)
- `sdk.d.ts:511` — `canUseTool` callback (CanUseTool type)
- `sdk.d.ts:663` — `permissionMode` option
- `sdk.d.ts:671` — `allowDangerouslySkipPermissions`
- `sdk.d.ts:831-837` — PermissionRequestHookInput
- `sdk.d.ts:838-852` — PermissionRequestHookSpecificOutput

---

## 1. Purpose

The permission system gates tool execution. Before any tool runs, the system checks whether the tool invocation is allowed, denied, or requires user approval. Permissions flow through a layered stack: mode → rules → hooks → callback → default.

---

## 2. Permission Modes (from `sdk.d.ts:826`)

```go
type PermissionMode string

const (
    ModeDefault            PermissionMode = "default"            // Prompt for dangerous ops
    ModeAcceptEdits        PermissionMode = "acceptEdits"        // Auto-accept file edits
    ModeBypassPermissions  PermissionMode = "bypassPermissions"  // Skip all checks (requires safety flag)
    ModePlan               PermissionMode = "plan"               // No tool execution, plan only
    ModeDelegate           PermissionMode = "delegate"           // Restrict to Task/Teammate tools only
    ModeDontAsk            PermissionMode = "dontAsk"            // Deny if not pre-approved
)
```

### Mode Behavior Matrix

| Mode | Read Tools | Write Tools | Bash | Network | User Prompt |
|------|-----------|-------------|------|---------|-------------|
| `default` | ✅ auto | ⚠️ ask | ⚠️ ask | ⚠️ ask | Yes |
| `acceptEdits` | ✅ auto | ✅ auto | ⚠️ ask | ⚠️ ask | Yes |
| `bypassPermissions` | ✅ auto | ✅ auto | ✅ auto | ✅ auto | No |
| `plan` | ❌ deny | ❌ deny | ❌ deny | ❌ deny | No |
| `delegate` | ❌ deny | ❌ deny | ❌ deny | ❌ deny | No (only Task) |
| `dontAsk` | ✅ auto | ❌ deny* | ❌ deny* | ❌ deny* | No |

*Unless pre-approved via `allowedTools` or permission rules.

---

## 3. Permission Check Flow

```
Tool invocation (name, input)
         │
         ▼
    ┌────────────┐
    │ Mode Check │  Is mode == plan? → deny all
    │            │  Is mode == bypassPermissions? → allow all
    └─────┬──────┘
          │
          ▼
    ┌────────────┐
    │ Disabled?  │  Is tool in disallowedTools? → deny
    └─────┬──────┘
          │
          ▼
    ┌────────────┐
    │ Auto-allow │  Is tool in allowedTools? → allow
    └─────┬──────┘
          │
          ▼
    ┌────────────┐
    │ Rules Check│  Match permission rules (tool + ruleContent)
    │            │  → allow | deny | ask | no match
    └─────┬──────┘
          │
          ▼
    ┌────────────┐
    │ Hook Check │  Fire PermissionRequest hook
    │            │  → allow | deny | continue
    └─────┬──────┘
          │
          ▼
    ┌────────────┐
    │ Callback   │  Call canUseTool callback (if provided)
    │            │  → allow | deny | (with updated input/permissions)
    └─────┬──────┘
          │
          ▼
    ┌────────────┐
    │ Mode       │  Apply mode defaults:
    │ Default    │  default → ask, dontAsk → deny, acceptEdits → by tool type
    └────────────┘
```

---

## 4. Go Types

### 4.1 Permission Result

```go
// PermissionResult is the outcome of a permission check.
// Maps to sdk.d.ts:853-867.
type PermissionResult struct {
    Behavior           PermissionBehavior
    UpdatedInput       map[string]interface{} // modified tool input (allow only)
    UpdatedPermissions []PermissionUpdate     // rule changes to persist
    Message            string                 // deny reason
    Interrupt          bool                   // stop the loop entirely
    ToolUseID          string                 // for correlation
}

type PermissionBehavior string

const (
    BehaviorAllow PermissionBehavior = "allow"
    BehaviorDeny  PermissionBehavior = "deny"
    BehaviorAsk   PermissionBehavior = "ask"
)
```

### 4.2 Permission Update

```go
// PermissionUpdate represents a change to permission rules.
// Maps to sdk.d.ts:869-897.
type PermissionUpdate struct {
    Type        PermissionUpdateType
    Rules       []PermissionRuleValue
    Behavior    PermissionBehavior
    Destination PermissionUpdateDestination

    // For setMode type
    Mode PermissionMode

    // For directory types
    Directories []string
}

type PermissionUpdateType string

const (
    UpdateAddRules         PermissionUpdateType = "addRules"
    UpdateReplaceRules     PermissionUpdateType = "replaceRules"
    UpdateRemoveRules      PermissionUpdateType = "removeRules"
    UpdateSetMode          PermissionUpdateType = "setMode"
    UpdateAddDirectories   PermissionUpdateType = "addDirectories"
    UpdateRemoveDirectories PermissionUpdateType = "removeDirectories"
)

type PermissionUpdateDestination string

const (
    DestUserSettings    PermissionUpdateDestination = "userSettings"
    DestProjectSettings PermissionUpdateDestination = "projectSettings"
    DestLocalSettings   PermissionUpdateDestination = "localSettings"
    DestSession         PermissionUpdateDestination = "session"
    DestCLIArg          PermissionUpdateDestination = "cliArg"
)

type PermissionRuleValue struct {
    ToolName    string `json:"toolName"`
    RuleContent string `json:"ruleContent,omitempty"` // semantic pattern
}
```

### 4.3 Permission Checker

```go
// Checker evaluates permissions for tool invocations.
type Checker struct {
    mode         PermissionMode
    allowedTools map[string]bool
    disallowed   map[string]bool
    rules        []PermissionRule       // from settings files
    sessionRules []PermissionRule       // accumulated during session
    canUseTool   CanUseToolFunc         // user callback
    hookRunner   HookRunner             // for PermissionRequest hook
    userPrompter UserPrompter           // for "ask" behavior
}

type CanUseToolFunc func(toolName string, input map[string]interface{}, signal context.Context) (PermissionResult, error)

type UserPrompter interface {
    // PromptForPermission shows the user a permission request and returns their decision.
    PromptForPermission(toolName string, input map[string]interface{}, suggestions []PermissionUpdate) (PermissionResult, error)
}

func NewChecker(config PermissionConfig) *Checker
func (c *Checker) Check(ctx context.Context, toolName string, input map[string]interface{}) (PermissionResult, error)
func (c *Checker) ApplyUpdate(update PermissionUpdate) error
func (c *Checker) SetMode(mode PermissionMode)
```

---

## 5. Permission Rules

Rules are loaded from settings files (user, project, local) and matched against tool invocations:

```go
type PermissionRule struct {
    ToolName    string
    RuleContent string             // semantic pattern (e.g., "read files in /src")
    Behavior    PermissionBehavior // allow | deny
    Source      PermissionUpdateDestination
}

// RuleMatch checks if a rule applies to a given tool invocation.
func (r *PermissionRule) Matches(toolName string, input map[string]interface{}) bool {
    if r.ToolName != toolName {
        return false
    }
    if r.RuleContent == "" {
        return true // matches all invocations of this tool
    }
    // Semantic matching: ruleContent is a pattern like "run tests"
    // For Bash: match against command description or command content
    // For FileEdit: match against file_path
    return semanticMatch(r.RuleContent, toolName, input)
}
```

---

## 6. Tool Classification

Tools are classified for default permission behavior:

```go
type ToolRiskLevel int

const (
    RiskNone     ToolRiskLevel = iota // FileRead, Glob, Grep, TodoWrite
    RiskLow                           // Config, ListMcpResources
    RiskMedium                        // FileWrite, FileEdit, NotebookEdit
    RiskHigh                          // Bash, WebFetch, WebSearch, MCP tools
    RiskCritical                      // Agent (spawns subagent)
)

func DefaultBehaviorForTool(mode PermissionMode, tool string) PermissionBehavior {
    risk := toolRiskLevel(tool)
    switch mode {
    case ModeDefault:
        if risk <= RiskLow { return BehaviorAllow }
        return BehaviorAsk
    case ModeAcceptEdits:
        if risk <= RiskMedium { return BehaviorAllow }
        return BehaviorAsk
    case ModeBypassPermissions:
        return BehaviorAllow
    case ModeDontAsk:
        return BehaviorDeny // unless explicitly allowed
    case ModePlan, ModeDelegate:
        return BehaviorDeny
    }
    return BehaviorAsk
}
```

---

## 7. Permission Request Hook Integration

The PermissionRequest hook fires when permission check reaches the "ask" stage:

```go
// From sdk.d.ts:831-852
type PermissionRequestHookInput struct {
    BaseHookInput
    HookEventName        string             `json:"hook_event_name"` // "PermissionRequest"
    ToolName             string             `json:"tool_name"`
    ToolInput            interface{}        `json:"tool_input"`
    PermissionSuggestions []PermissionUpdate `json:"permission_suggestions,omitempty"`
}

type PermissionRequestHookOutput struct {
    HookEventName string `json:"hookEventName"` // "PermissionRequest"
    Decision      struct {
        Behavior         PermissionBehavior     `json:"behavior"` // "allow" | "deny"
        UpdatedInput     map[string]interface{} `json:"updatedInput,omitempty"`
        UpdatedPerms     []PermissionUpdate     `json:"updatedPermissions,omitempty"`
        Message          string                 `json:"message,omitempty"` // deny only
        Interrupt        bool                   `json:"interrupt,omitempty"` // deny only
    } `json:"decision"`
}
```

If a PermissionRequest hook returns a decision, it short-circuits the user prompt.

---

## 8. Headless Mode (SDK/API Use)

In headless mode (no terminal UI), the permission system must handle "ask" differently:

```go
// For headless operation, "ask" behaviors must resolve without user interaction.
// Options:
// 1. Use canUseTool callback (SDK provides this)
// 2. Use PermissionRequest hook
// 3. Default deny (mode: dontAsk)
// 4. Default allow (mode: bypassPermissions + safety flag)

type HeadlessPermissionStrategy int

const (
    HeadlessCallback HeadlessPermissionStrategy = iota // Use canUseTool
    HeadlessHook                                        // Use PermissionRequest hook
    HeadlessDeny                                        // Deny all "ask" results
    HeadlessAllow                                       // Allow all (requires safety flag)
)
```

For the Phare Agent Hub, we use the hook-based approach, wiring permission requests to the platform's approval workflow.

---

## 9. Verification Checklist

- [ ] **Mode coverage**: All 6 PermissionModes implemented with correct default behaviors
- [ ] **Check order**: Mode → disabled → allowed → rules → hook → callback → default
- [ ] **Update persistence**: PermissionUpdates applied to correct destination (session vs settings)
- [ ] **Rule matching**: Semantic rule matching works for Bash commands and file paths
- [ ] **Hook integration**: PermissionRequest hook can short-circuit user prompts
- [ ] **Input modification**: `updatedInput` from allow decisions replaces original tool input
- [ ] **Interrupt behavior**: `interrupt: true` on deny stops the entire loop (not just current tool)
- [ ] **bypassPermissions safety**: Requires `allowDangerouslySkipPermissions` flag to activate
- [ ] **Plan mode**: All tools denied except reading/informational queries
- [ ] **Delegate mode**: Only Agent/Task tool allowed
- [ ] **Headless support**: "ask" behavior resolves via callback/hook without terminal
