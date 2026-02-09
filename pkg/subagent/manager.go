package subagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
const maxCompletedAgents = 100

// ManagerOpts configures a Manager.
type ManagerOpts struct {
	TranscriptDir     string
	OutputDir         string // directory for background agent output files
	ParentConfig      *agent.AgentConfig
	HookRunner        *hooks.Runner
	LLMClient         llm.Client
	PromptAssembler   agent.SystemPromptAssembler
	PermissionChecker agent.PermissionChecker
	CostTracker       *llm.CostTracker
	ParentRegistry    *tools.Registry
	TaskRestriction   *TaskRestriction // limits which agent types can be spawned
	SessionStore      agent.SessionStore // pass to subagents for transcript persistence
}

// Manager creates, tracks, and controls subagent instances.
// It implements tools.SubagentSpawner.
type Manager struct {
	mu            sync.RWMutex
	agents        map[string]Definition    // all registered agent definitions
	active        map[string]*RunningAgent // running agent instances by ID
	completed     map[string]*RunningAgent // recently completed agents (bounded to maxCompletedAgents)
	completedOrder []string                // insertion order for oldest eviction
	builtIn       map[string]Definition
	cliAgents     map[string]Definition
	opts          ManagerOpts
}

// NewManager creates a Manager with built-in agents and optional CLI/file-based agents.
func NewManager(opts ManagerOpts, cliAgents map[string]Definition) *Manager {
	builtIn := BuiltInAgents()
	m := &Manager{
		agents:    make(map[string]Definition),
		active:    make(map[string]*RunningAgent),
		completed: make(map[string]*RunningAgent),
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

	// 2b. Enforce task restriction
	if r := m.opts.TaskRestriction; r != nil && !r.Unrestricted {
		allowed := toSet(r.AllowedTypes)
		if !allowed[input.SubagentType] {
			return tools.AgentResult{}, fmt.Errorf("agent type %q not allowed by task restriction (allowed: %v)", input.SubagentType, r.AllowedTypes)
		}
	}

	// 3. Generate ID (or use resume ID)
	agentID := uuid.New().String()
	if input.Resume != nil && *input.Resume != "" {
		agentID = *input.Resume
		// Check if we're resuming an existing agent
		m.mu.RLock()
		existing, existsActive := m.active[agentID]
		completed, existsCompleted := m.completed[agentID]
		m.mu.RUnlock()
		if existsActive {
			return m.resumeRunningAgent(ctx, existing, input)
		}
		if existsCompleted {
			return m.resumeCompletedAgent(ctx, completed, input, def)
		}
		return tools.AgentResult{}, fmt.Errorf("cannot resume unknown agent %q", agentID)
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

	// Validate tool names against parent registry
	var spawnWarnings []string
	if m.opts.ParentRegistry != nil && len(def.Tools) > 0 {
		parentSet := toSet(parentToolNames)
		for _, t := range def.Tools {
			if !parentSet[t] && t != "Agent" && !isTaskEntry(t) {
				spawnWarnings = append(spawnWarnings, fmt.Sprintf("tool %q in definition not found in parent registry", t))
			}
		}
	}

	// 7. Resolve permission mode
	permMode := m.resolvePermissionMode(def, input)

	// 8. Memory
	var memoryContent string
	if def.Memory != "" {
		memDir := resolveMemoryDir(def.Name, def.Memory, m.parentCWD())
		ensureMemoryDir(memDir)
		content, _ := loadMemoryContent(memDir)
		memoryContent = content
		// Ensure file tools are available when memory is enabled
		toolNames = ensureTools(toolNames, "FileRead", "FileWrite", "FileEdit")
	}

	// 9. Build system prompt
	systemPrompt := m.buildSystemPrompt(def, input, memoryContent)

	// 10. Build config
	maxTurns := 50
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
		SessionStore:      m.resolveSessionStore(),
	}

	// 11. Build scoped tool registry
	config.ToolRegistry = m.buildScopedRegistry(toolNames, taskRestriction)

	// 12. Register scoped hooks if agent has hooks defined
	if m.opts.HookRunner != nil && len(def.Hooks) > 0 {
		scopedMatchers := convertHooks(def.Hooks)
		m.opts.HookRunner.RegisterScoped(agentID, scopedMatchers)
	}

	// Build transcript path if session store is configured
	var transcriptPath string
	if m.opts.SessionStore != nil && m.opts.TranscriptDir != "" {
		os.MkdirAll(m.opts.TranscriptDir, 0o755)
		transcriptPath = filepath.Join(m.opts.TranscriptDir, "agent-"+agentID+".jsonl")
	}

	// Create RunningAgent
	ra := &RunningAgent{
		ID:             agentID,
		Type:           input.SubagentType,
		Name:           m.resolveDisplayName(def, input),
		Definition:     def,
		State:          StateRunning,
		StartedAt:      time.Now(),
		Output:         &AgentOutput{},
		TranscriptPath: transcriptPath,
		Done:           make(chan struct{}),
		Warnings:       spawnWarnings,
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
		// Create output file for background agents
		outputFilePath := m.createOutputFile(agentID)
		ra.OutputFile = outputFilePath

		// Background: launch goroutine, return immediately
		go m.drainAndFinish(query, ra)
		return tools.AgentResult{AgentID: agentID, OutputFile: outputFilePath}, nil
	}

	// Foreground: block until complete
	dr := m.drainQuery(query)
	m.finishAgent(ra, query, dr)

	return tools.AgentResult{
		AgentID: agentID,
		Output:  dr.output,
		Error:   dr.errorMsg,
		Metrics: taskMetricsToAgentMetrics(ra.Metrics),
	}, nil
}

// GetOutput retrieves the output of a running or completed agent.
func (m *Manager) GetOutput(taskID string, block bool, timeout time.Duration) (*TaskResult, error) {
	m.mu.RLock()
	ra, ok := m.active[taskID]
	if !ok {
		ra, ok = m.completed[taskID]
	}
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
	if !ok {
		// Check completed — already done, return nil
		_, inCompleted := m.completed[taskID]
		m.mu.RUnlock()
		if inCompleted {
			return nil
		}
		return fmt.Errorf("unknown agent %q", taskID)
	}
	m.mu.RUnlock()

	if ra.GetState() != StateRunning {
		return nil // already done
	}

	if ra.Cancel != nil {
		ra.Cancel()
	}
	ra.SetState(StateStopped)
	return nil
}

// List returns the status of all known agents (active and recently completed).
func (m *Manager) List() []AgentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]AgentStatus, 0, len(m.active)+len(m.completed))
	for _, ra := range m.active {
		result = append(result, AgentStatus{
			ID:        ra.ID,
			Type:      ra.Type,
			Name:      ra.Name,
			State:     ra.GetState(),
			StartedAt: ra.StartedAt,
		})
	}
	for _, ra := range m.completed {
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
// Returns any warnings encountered during loading (malformed files, etc.).
func (m *Manager) Reload(cwd string) ([]LoadWarning, error) {
	loader := NewLoader(cwd, "", /* no plugin dirs for now */)
	fileBased, warnings, err := loader.LoadAll()
	if err != nil {
		return warnings, err
	}

	m.mu.Lock()
	m.agents = Resolve(m.builtIn, m.cliAgents, fileBased)
	m.mu.Unlock()
	return warnings, nil
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

// drainResult holds the text output and any error captured from the subagent stream.
type drainResult struct {
	output   string
	errorMsg string
}

func (m *Manager) drainAndFinish(query *agent.Query, ra *RunningAgent) {
	dr := m.drainQuery(query)
	// Write output file before finishAgent closes Done channel
	content := dr.output
	if dr.errorMsg != "" {
		content += "\n\nError: " + dr.errorMsg
	}
	writeOutputFile(ra.OutputFile, content)
	m.finishAgent(ra, query, dr)
}

func (m *Manager) drainQuery(query *agent.Query) drainResult {
	var textParts []string
	var errorMsg string
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
		case types.ResultMessage:
			if am.IsError && len(am.Errors) > 0 {
				errorMsg = strings.Join(am.Errors, "; ")
			}
		case *types.ResultMessage:
			if am.IsError && len(am.Errors) > 0 {
				errorMsg = strings.Join(am.Errors, "; ")
			}
		}
	}
	return drainResult{
		output:   strings.Join(textParts, ""),
		errorMsg: errorMsg,
	}
}

func (m *Manager) finishAgent(ra *RunningAgent, query *agent.Query, dr drainResult) {
	query.Wait()

	state := query.State()
	duration := time.Since(ra.StartedAt)

	metrics := TaskMetrics{
		Duration:  duration,
		TurnCount: state.TurnCount,
		CostUSD:   state.TotalCostUSD,
		TokensUsed: state.TotalUsage,
	}

	// Determine final state
	finalState := StateCompleted
	exitReason := query.GetExitReason()
	switch {
	case exitReason == agent.ExitInterrupted || exitReason == agent.ExitAborted:
		finalState = StateStopped
	case exitReason == agent.ExitReason("error") || exitReason == agent.ExitMaxBudget:
		finalState = StateFailed
	case state.LastError != nil:
		finalState = StateFailed
	}

	// Collect error message from query state or drain result
	errorMsg := dr.errorMsg
	if state.LastError != nil && errorMsg == "" {
		errorMsg = state.LastError.Error()
	}

	ra.SetMetrics(metrics)
	ra.SetState(finalState)
	ra.Output.Append(dr.output)
	ra.Output.SetResult(&TaskResult{
		Content: dr.output,
		Metrics: metrics,
		State:   finalState,
		AgentID: ra.ID,
		Error:   errorMsg,
	})

	// Signal completion
	close(ra.Done)

	// Move from active to completed
	m.mu.Lock()
	delete(m.active, ra.ID)
	m.completed[ra.ID] = ra
	m.completedOrder = append(m.completedOrder, ra.ID)
	// Evict oldest if over limit
	for len(m.completedOrder) > maxCompletedAgents {
		oldest := m.completedOrder[0]
		m.completedOrder = m.completedOrder[1:]
		delete(m.completed, oldest)
	}
	m.mu.Unlock()

	// Cleanup scoped hooks
	ra.Cleanup()

	// Fire SubagentStop hook
	if m.opts.HookRunner != nil {
		m.opts.HookRunner.Fire(context.Background(), types.HookEventSubagentStop, &hooks.SubagentStopHookInput{
			BaseHookInput: hooks.BaseHookInput{
				SessionID: m.parentSessionID(),
			},
			HookEventName:       "SubagentStop",
			AgentID:             ra.ID,
			AgentType:           ra.Type,
			AgentTranscriptPath: ra.TranscriptPath,
		})
	}
}

// resumeRunningAgent returns current output from an agent that's still running.
func (m *Manager) resumeRunningAgent(_ context.Context, ra *RunningAgent, _ tools.AgentInput) (tools.AgentResult, error) {
	result := ra.Output.GetResult()
	if result != nil {
		return tools.AgentResult{AgentID: ra.ID, Output: result.Content}, nil
	}
	return tools.AgentResult{AgentID: ra.ID, Output: ra.Output.String()}, nil
}

// resumeCompletedAgent re-launches a completed/stopped/failed agent with the new prompt,
// prepending the previous output as conversation context.
func (m *Manager) resumeCompletedAgent(ctx context.Context, ra *RunningAgent, input tools.AgentInput, def Definition) (tools.AgentResult, error) {
	// Build the previous context message
	previousOutput := ra.Output.String()
	contextPrompt := input.Prompt
	if previousOutput != "" {
		contextPrompt = "Previous agent output:\n\n" + previousOutput + "\n\n---\n\nNew request: " + input.Prompt
	}

	// Remove from completed map since we're re-spawning
	m.mu.Lock()
	delete(m.completed, ra.ID)
	// Remove from completedOrder
	for i, id := range m.completedOrder {
		if id == ra.ID {
			m.completedOrder = append(m.completedOrder[:i], m.completedOrder[i+1:]...)
			break
		}
	}
	m.mu.Unlock()

	// Re-spawn with the same ID
	resumeInput := tools.AgentInput{
		Description:     input.Description,
		Prompt:          contextPrompt,
		SubagentType:    input.SubagentType,
		Model:           input.Model,
		RunInBackground: input.RunInBackground,
		MaxTurns:        input.MaxTurns,
		Name:            input.Name,
		Mode:            input.Mode,
		// Don't set Resume — we're handling it here
	}

	// Resolve model from the original definition
	model := resolveModel(def.Model, input.Model, m.parentModel())

	// Build system prompt
	systemPrompt := m.buildSystemPrompt(def, input, "")

	maxTurns := 50
	if input.MaxTurns != nil {
		maxTurns = *input.MaxTurns
	} else if def.MaxTurns != nil {
		maxTurns = *def.MaxTurns
	}

	isBackground := input.RunInBackground != nil && *input.RunInBackground
	permMode := m.resolvePermissionMode(def, resumeInput)

	config := agent.AgentConfig{
		Model:             model,
		MaxTurns:          maxTurns,
		CWD:               m.parentCWD(),
		SessionID:         ra.ID,
		PermissionMode:    permMode,
		AgentType:         ra.Type,
		BackgroundMode:    isBackground,
		CanSpawnSubagents: false,
		LLMClient:         m.opts.LLMClient,
		Prompter:          &agent.StaticPromptAssembler{Prompt: systemPrompt},
		Permissions:       m.resolvePermissions(isBackground),
		Hooks:             m.resolveHooks(),
		Compactor:         &agent.NoOpCompactor{},
		CostTracker:       m.opts.CostTracker,
		SessionStore:      m.resolveSessionStore(),
	}

	// Build scoped tool registry
	parentToolNames := m.parentToolNames()
	toolNames := resolveTools(def.Tools, def.DisallowedTools, parentToolNames)
	toolNames = filterFunc(toolNames, func(s string) bool { return s != "Agent" })
	_, toolNames = parseTaskRestriction(toolNames)
	config.ToolRegistry = m.buildScopedRegistry(toolNames, nil)

	// Create new RunningAgent with the same ID
	newRA := &RunningAgent{
		ID:         ra.ID,
		Type:       ra.Type,
		Name:       ra.Name,
		Definition: def,
		State:      StateRunning,
		StartedAt:  time.Now(),
		Output:     &AgentOutput{},
		Done:       make(chan struct{}),
		cleanupFn: func() {
			if m.opts.HookRunner != nil {
				m.opts.HookRunner.UnregisterScoped(ra.ID)
			}
		},
	}

	m.mu.Lock()
	m.active[ra.ID] = newRA
	m.mu.Unlock()

	// Launch the agentic loop with the context-enriched prompt
	query := agent.RunLoop(ctx, contextPrompt, config)
	newRA.Cancel = func() { query.Interrupt() }

	if isBackground {
		outputFilePath := m.createOutputFile(ra.ID)
		newRA.OutputFile = outputFilePath
		go m.drainAndFinish(query, newRA)
		return tools.AgentResult{AgentID: ra.ID, OutputFile: outputFilePath}, nil
	}

	// Foreground: block until complete
	dr := m.drainQuery(query)
	m.finishAgent(newRA, query, dr)

	return tools.AgentResult{
		AgentID: ra.ID,
		Output:  dr.output,
		Error:   dr.errorMsg,
		Metrics: taskMetricsToAgentMetrics(newRA.Metrics),
	}, nil
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
	// If parent uses bypassPermissions, subagent inherits it unconditionally
	if m.opts.ParentConfig != nil && m.opts.ParentConfig.PermissionMode == types.PermissionModeBypassPermissions {
		return types.PermissionModeBypassPermissions
	}
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

// backgroundPreApprovedTools is the set of tools auto-allowed for background agents.
// These are read-only tools and tools that don't require user interaction.
var backgroundPreApprovedTools = map[string]bool{
	"Read":         true,
	"FileRead":     true,
	"Glob":         true,
	"Grep":         true,
	"Bash":         true,
	"Write":        true,
	"FileWrite":    true,
	"Edit":         true,
	"FileEdit":     true,
	"WebFetch":     true,
	"WebSearch":    true,
	"NotebookEdit": true,
	"TodoWrite":    true,
	"Config":       true,
}

func (m *Manager) resolvePermissions(isBackground bool) agent.PermissionChecker {
	if isBackground {
		// Background agents deny tools not in the pre-approved set
		return &agent.BackgroundPermissionChecker{PreApproved: backgroundPreApprovedTools}
	}
	if m.opts.PermissionChecker != nil {
		return m.opts.PermissionChecker
	}
	return &agent.AllowAllChecker{}
}

func (m *Manager) resolveSessionStore() agent.SessionStore {
	if m.opts.SessionStore != nil {
		return m.opts.SessionStore
	}
	return nil
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

// createOutputFile creates an output file for a background agent.
// Returns the file path, or empty string if output dir is not set.
func (m *Manager) createOutputFile(agentID string) string {
	dir := m.opts.OutputDir
	if dir == "" {
		return ""
	}
	os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, agentID+".output")
}

// writeOutputFile writes content to the agent's output file.
func writeOutputFile(path, content string) {
	if path == "" {
		return
	}
	os.WriteFile(path, []byte(content), 0o644)
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

// taskMetricsToAgentMetrics converts internal TaskMetrics to the tools.AgentMetrics type.
func taskMetricsToAgentMetrics(m TaskMetrics) *tools.AgentMetrics {
	return &tools.AgentMetrics{
		DurationSecs: m.Duration.Seconds(),
		TurnCount:    m.TurnCount,
		CostUSD:      m.CostUSD,
		InputTokens:  m.TokensUsed.InputTokens,
		OutputTokens: m.TokensUsed.OutputTokens,
	}
}

// Verify Manager implements tools.SubagentSpawner at compile time.
var _ tools.SubagentSpawner = (*Manager)(nil)
