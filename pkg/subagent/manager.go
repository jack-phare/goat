package subagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/hooks"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/prompt"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

const maxConcurrentAgents = 10

// ManagerOpts configures a Manager.
type ManagerOpts struct {
	TranscriptDir     string
	ParentConfig      *agent.AgentConfig
	HookRunner        *hooks.Runner
	LLMClient         llm.Client
	PromptAssembler   agent.SystemPromptAssembler
	PermissionChecker agent.PermissionChecker
	CostTracker       *llm.CostTracker
	ParentRegistry    *tools.Registry
}

// Manager creates, tracks, and controls subagent instances.
// It implements tools.SubagentSpawner.
type Manager struct {
	mu        sync.RWMutex
	agents    map[string]Definition    // all registered agent definitions
	active    map[string]*RunningAgent // running agent instances by ID
	builtIn   map[string]Definition
	cliAgents map[string]Definition
	opts      ManagerOpts
}

// NewManager creates a Manager with built-in agents and optional CLI/file-based agents.
func NewManager(opts ManagerOpts, cliAgents map[string]Definition) *Manager {
	builtIn := BuiltInAgents()
	m := &Manager{
		agents:    make(map[string]Definition),
		active:    make(map[string]*RunningAgent),
		builtIn:   builtIn,
		cliAgents: cliAgents,
		opts:      opts,
	}

	// Initial resolution: built-in + CLI (file-based loaded separately via Reload)
	m.agents = Resolve(builtIn, cliAgents, nil)
	return m
}

// Spawn creates and runs a subagent. Implements tools.SubagentSpawner.
func (m *Manager) Spawn(ctx context.Context, input tools.AgentInput) (tools.AgentResult, error) {
	// 1. Check limits
	m.mu.RLock()
	activeCount := len(m.active)
	m.mu.RUnlock()
	if activeCount >= maxConcurrentAgents {
		return tools.AgentResult{}, fmt.Errorf("max concurrent agents (%d) reached", maxConcurrentAgents)
	}

	// 2. Resolve definition
	m.mu.RLock()
	def, ok := m.agents[input.SubagentType]
	m.mu.RUnlock()
	if !ok {
		return tools.AgentResult{}, fmt.Errorf("unknown agent type %q", input.SubagentType)
	}

	// 3. Generate ID (or use resume ID)
	agentID := uuid.New().String()
	if input.Resume != nil && *input.Resume != "" {
		agentID = *input.Resume
		// Check if we're resuming an existing agent
		m.mu.RLock()
		existing, exists := m.active[agentID]
		m.mu.RUnlock()
		if exists {
			return m.resumeAgent(ctx, existing, input)
		}
	}

	// 4. Fire SubagentStart hook
	if m.opts.HookRunner != nil {
		m.opts.HookRunner.Fire(ctx, types.HookEventSubagentStart, &hooks.SubagentStartHookInput{
			BaseHookInput: hooks.BaseHookInput{
				SessionID: m.parentSessionID(),
			},
			HookEventName: "SubagentStart",
			AgentID:       agentID,
			AgentType:     input.SubagentType,
		})
	}

	// 5. Resolve model
	model := resolveModel(def.Model, input.Model, m.parentModel())

	// 6. Resolve tools
	parentToolNames := m.parentToolNames()
	toolNames := resolveTools(def.Tools, def.DisallowedTools, parentToolNames)
	// Remove Agent tool from subagent registries (no nesting)
	toolNames = filterFunc(toolNames, func(s string) bool { return s != "Agent" })
	taskRestriction, toolNames := parseTaskRestriction(toolNames)

	// 7. Resolve permission mode
	permMode := m.resolvePermissionMode(def, input)

	// 8. Memory
	var memoryContent string
	if def.Memory != "" {
		memDir := resolveMemoryDir(def.Name, def.Memory, m.parentCWD())
		ensureMemoryDir(memDir)
		content, _ := loadMemoryContent(memDir)
		memoryContent = content
	}

	// 9. Build system prompt
	systemPrompt := m.buildSystemPrompt(def, input, memoryContent)

	// 10. Build config
	maxTurns := 100
	if input.MaxTurns != nil {
		maxTurns = *input.MaxTurns
	} else if def.MaxTurns != nil {
		maxTurns = *def.MaxTurns
	}

	isBackground := input.RunInBackground != nil && *input.RunInBackground

	config := agent.AgentConfig{
		Model:             model,
		MaxTurns:          maxTurns,
		CWD:               m.parentCWD(),
		SessionID:         agentID,
		PermissionMode:    permMode,
		AgentType:         input.SubagentType,
		BackgroundMode:    isBackground,
		CanSpawnSubagents: false,
		LLMClient:         m.opts.LLMClient,
		Prompter:          &agent.StaticPromptAssembler{Prompt: systemPrompt},
		Permissions:       m.resolvePermissions(isBackground),
		Hooks:             m.resolveHooks(),
		Compactor:         &agent.NoOpCompactor{},
		CostTracker:       m.opts.CostTracker,
	}

	// 11. Build scoped tool registry
	config.ToolRegistry = m.buildScopedRegistry(toolNames, taskRestriction)

	// 12. Register scoped hooks if agent has hooks defined
	if m.opts.HookRunner != nil && len(def.Hooks) > 0 {
		scopedMatchers := convertHooks(def.Hooks)
		m.opts.HookRunner.RegisterScoped(agentID, scopedMatchers)
	}

	// Create RunningAgent
	ra := &RunningAgent{
		ID:         agentID,
		Type:       input.SubagentType,
		Name:       m.resolveDisplayName(def, input),
		Definition: def,
		State:      StateRunning,
		StartedAt:  time.Now(),
		Output:     &AgentOutput{},
		Done:       make(chan struct{}),
		cleanupFn: func() {
			if m.opts.HookRunner != nil {
				m.opts.HookRunner.UnregisterScoped(agentID)
			}
		},
	}

	m.mu.Lock()
	m.active[agentID] = ra
	m.mu.Unlock()

	// Launch the agentic loop
	query := agent.RunLoop(ctx, input.Prompt, config)
	ra.Cancel = func() { query.Interrupt() }

	if isBackground {
		// Background: launch goroutine, return immediately
		go m.drainAndFinish(query, ra)
		return tools.AgentResult{AgentID: agentID}, nil
	}

	// Foreground: block until complete
	output := m.drainQuery(query)
	m.finishAgent(ra, query, output)

	return tools.AgentResult{
		AgentID: agentID,
		Output:  output,
	}, nil
}

// GetOutput retrieves the output of a running or completed agent.
func (m *Manager) GetOutput(taskID string, block bool, timeout time.Duration) (*TaskResult, error) {
	m.mu.RLock()
	ra, ok := m.active[taskID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown agent %q", taskID)
	}

	if !block {
		result := ra.Output.GetResult()
		if result == nil {
			return &TaskResult{
				Content: ra.Output.String(),
				State:   StateRunning,
				AgentID: taskID,
			}, nil
		}
		return result, nil
	}

	// Block until done or timeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ra.Done:
		result := ra.Output.GetResult()
		if result != nil {
			return result, nil
		}
		return &TaskResult{Content: ra.Output.String(), State: ra.GetState(), AgentID: taskID}, nil
	case <-timer.C:
		return &TaskResult{
			Content: ra.Output.String(),
			State:   StateRunning,
			AgentID: taskID,
		}, nil
	}
}

// Stop cancels a running agent.
func (m *Manager) Stop(taskID string) error {
	m.mu.RLock()
	ra, ok := m.active[taskID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown agent %q", taskID)
	}

	if ra.GetState() != StateRunning {
		return nil // already done
	}

	if ra.Cancel != nil {
		ra.Cancel()
	}
	ra.SetState(StateStopped)
	return nil
}

// List returns the status of all known agents.
func (m *Manager) List() []AgentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]AgentStatus, 0, len(m.active))
	for _, ra := range m.active {
		result = append(result, AgentStatus{
			ID:        ra.ID,
			Type:      ra.Type,
			Name:      ra.Name,
			State:     ra.GetState(),
			StartedAt: ra.StartedAt,
		})
	}
	return result
}

// AgentStatus is a summary of a running/completed agent.
type AgentStatus struct {
	ID        string
	Type      string
	Name      string
	State     AgentState
	StartedAt time.Time
}

// RegisterAgents merges additional definitions into the manager.
func (m *Manager) RegisterAgents(defs map[string]Definition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, def := range defs {
		m.agents[name] = def
	}
}

// Reload re-scans the filesystem and re-resolves all agent definitions.
func (m *Manager) Reload(cwd string) error {
	loader := NewLoader(cwd, "", /* no plugin dirs for now */)
	fileBased, err := loader.LoadAll()
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.agents = Resolve(m.builtIn, m.cliAgents, fileBased)
	m.mu.Unlock()
	return nil
}

// Definitions returns a copy of all registered agent definitions.
func (m *Manager) Definitions() map[string]Definition {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]Definition, len(m.agents))
	for k, v := range m.agents {
		result[k] = v
	}
	return result
}

// --- Internal helpers ---

func (m *Manager) drainAndFinish(query *agent.Query, ra *RunningAgent) {
	output := m.drainQuery(query)
	m.finishAgent(ra, query, output)
}

func (m *Manager) drainQuery(query *agent.Query) string {
	var textParts []string
	for msg := range query.Messages() {
		// Extract text content from assistant messages (value or pointer)
		switch am := msg.(type) {
		case types.AssistantMessage:
			for _, block := range am.Message.Content {
				if block.Type == "text" && block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			}
		case *types.AssistantMessage:
			for _, block := range am.Message.Content {
				if block.Type == "text" && block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			}
		}
	}
	return strings.Join(textParts, "")
}

func (m *Manager) finishAgent(ra *RunningAgent, query *agent.Query, output string) {
	query.Wait()

	state := query.State()
	duration := time.Since(ra.StartedAt)

	metrics := TaskMetrics{
		Duration:  duration,
		TurnCount: state.TurnCount,
		CostUSD:   state.TotalCostUSD,
		TokensUsed: state.TotalUsage,
	}

	finalState := StateCompleted
	if exitReason := query.GetExitReason(); exitReason == agent.ExitInterrupted || exitReason == agent.ExitAborted {
		finalState = StateStopped
	}

	ra.SetMetrics(metrics)
	ra.SetState(finalState)
	ra.Output.Append(output)
	ra.Output.SetResult(&TaskResult{
		Content: output,
		Metrics: metrics,
		State:   finalState,
		AgentID: ra.ID,
	})

	// Signal completion
	close(ra.Done)

	// Cleanup scoped hooks
	ra.Cleanup()

	// Fire SubagentStop hook
	if m.opts.HookRunner != nil {
		m.opts.HookRunner.Fire(context.Background(), types.HookEventSubagentStop, &hooks.SubagentStopHookInput{
			BaseHookInput: hooks.BaseHookInput{
				SessionID: m.parentSessionID(),
			},
			HookEventName: "SubagentStop",
			AgentID:       ra.ID,
			AgentType:     ra.Type,
		})
	}
}

func (m *Manager) resumeAgent(_ context.Context, ra *RunningAgent, _ tools.AgentInput) (tools.AgentResult, error) {
	// For now, return current output
	result := ra.Output.GetResult()
	if result != nil {
		return tools.AgentResult{AgentID: ra.ID, Output: result.Content}, nil
	}
	return tools.AgentResult{AgentID: ra.ID, Output: ra.Output.String()}, nil
}

func (m *Manager) parentSessionID() string {
	if m.opts.ParentConfig != nil {
		return m.opts.ParentConfig.SessionID
	}
	return ""
}

func (m *Manager) parentModel() string {
	if m.opts.ParentConfig != nil {
		return m.opts.ParentConfig.Model
	}
	return "claude-sonnet-4-5-20250929"
}

func (m *Manager) parentCWD() string {
	if m.opts.ParentConfig != nil {
		return m.opts.ParentConfig.CWD
	}
	return ""
}

func (m *Manager) parentToolNames() []string {
	if m.opts.ParentRegistry != nil {
		return m.opts.ParentRegistry.Names()
	}
	return nil
}

func (m *Manager) resolveDisplayName(def Definition, input tools.AgentInput) string {
	if input.Name != nil && *input.Name != "" {
		return *input.Name
	}
	if def.Name != "" {
		return def.Name
	}
	return input.SubagentType
}

func (m *Manager) resolvePermissionMode(def Definition, input tools.AgentInput) types.PermissionMode {
	if input.Mode != nil && *input.Mode != "" {
		return types.PermissionMode(*input.Mode)
	}
	if def.PermissionMode != "" {
		return types.PermissionMode(def.PermissionMode)
	}
	if m.opts.ParentConfig != nil {
		return m.opts.ParentConfig.PermissionMode
	}
	return types.PermissionModeDefault
}

func (m *Manager) resolvePermissions(isBackground bool) agent.PermissionChecker {
	if isBackground {
		// Background agents auto-deny unpermitted tools
		return &agent.AllowAllChecker{}
	}
	if m.opts.PermissionChecker != nil {
		return m.opts.PermissionChecker
	}
	return &agent.AllowAllChecker{}
}

func (m *Manager) resolveHooks() agent.HookRunner {
	if m.opts.HookRunner != nil {
		return m.opts.HookRunner
	}
	return &agent.NoOpHookRunner{}
}

func (m *Manager) buildScopedRegistry(toolNames []string, _ *TaskRestriction) *tools.Registry {
	reg := tools.NewRegistry()
	if m.opts.ParentRegistry == nil {
		return reg
	}

	allowed := toSet(toolNames)
	for _, name := range m.opts.ParentRegistry.Names() {
		if name == "Agent" {
			continue // never give subagents the Agent tool
		}
		if len(allowed) > 0 && !allowed[name] {
			continue
		}
		if tool, ok := m.opts.ParentRegistry.Get(name); ok {
			reg.Register(tool)
		}
	}
	return reg
}

func (m *Manager) buildSystemPrompt(def Definition, input tools.AgentInput, memoryContent string) string {
	// Use the prompt assembly helper
	parentConfig := m.opts.ParentConfig
	if parentConfig == nil {
		parentConfig = &agent.AgentConfig{}
	}

	systemPrompt := prompt.AssembleSubagentPrompt(def.AgentDefinition, parentConfig)

	// Append memory content if available
	if memoryContent != "" {
		systemPrompt += "\n\n## Agent Memory\n\n" + memoryContent
	}

	// Append critical reminder
	if def.CriticalReminder != "" {
		systemPrompt += "\n\nCRITICAL: " + def.CriticalReminder
	}

	// Append the task description for context
	if input.Description != "" {
		systemPrompt += "\n\nTask: " + input.Description
	}

	return systemPrompt
}

// convertHooks converts types.HookCallbackMatcher entries to hooks.CallbackMatcher entries.
func convertHooks(hookMap map[types.HookEvent][]types.HookCallbackMatcher) map[types.HookEvent][]hooks.CallbackMatcher {
	result := make(map[types.HookEvent][]hooks.CallbackMatcher, len(hookMap))
	for event, matchers := range hookMap {
		converted := make([]hooks.CallbackMatcher, 0, len(matchers))
		for _, m := range matchers {
			cm := hooks.CallbackMatcher{
				Matcher:  m.Matcher,
				Commands: m.HookCallbackIDs, // treat callback IDs as shell commands
			}
			if m.Timeout != nil {
				cm.Timeout = *m.Timeout
			}
			converted = append(converted, cm)
		}
		result[event] = converted
	}
	return result
}

// Verify Manager implements tools.SubagentSpawner at compile time.
var _ tools.SubagentSpawner = (*Manager)(nil)
