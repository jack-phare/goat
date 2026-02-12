package agent

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// RunLoop starts an agentic loop and returns a Query for observing/controlling it.
// The loop runs in a background goroutine and emits SDKMessages on the Query's channel.
func RunLoop(ctx context.Context, prompt string, config AgentConfig) *Query {
	loopCtx, cancel := context.WithCancel(ctx)
	ch := make(chan types.SDKMessage, 64)

	state := &LoopState{
		SessionID: config.SessionID,
	}
	if state.SessionID == "" {
		state.SessionID = uuid.New().String()
	}

	q := &Query{
		messages:    ch,
		done:        make(chan struct{}),
		state:       state,
		costTracker: config.CostTracker,
		cancel:      cancel,
	}

	// Set up multi-turn channels if enabled
	if config.MultiTurn {
		q.inputCh = make(chan []byte, 16)
		q.controlCh = make(chan types.ControlRequest, 4)
		q.controlResp = make(chan types.ControlResponse, 1)
		q.closeCh = make(chan struct{})
	}

	go runLoop(loopCtx, prompt, &config, state, ch, q)

	return q
}

func runLoop(ctx context.Context, prompt string, config *AgentConfig, state *LoopState, ch chan<- types.SDKMessage, q *Query) {
	defer close(ch)
	defer close(q.done)

	startTime := time.Now()
	var apiDuration time.Duration

	// 0. Session restore/create (if SessionStore is configured)
	if config.SessionStore != nil {
		initializeSession(config, state)
	}

	// 1. Fire SessionStart hook and collect additional context
	sessionStartResults, _ := config.Hooks.Fire(ctx, types.HookEventSessionStart, map[string]any{
		"source": "startup",
	})
	collectAdditionalContext(state, sessionStartResults)

	// 2. Emit system init message
	emitInit(ch, config, state)

	// 3. Build initial messages (unless restored from session)
	if len(state.Messages) == 0 {
		state.Messages = []llm.ChatMessage{
			{Role: "user", Content: prompt},
		}
	}

	// Persist initial user message
	if config.SessionStore != nil && len(state.Messages) > 0 {
		last := state.Messages[len(state.Messages)-1]
		if last.Role == "user" {
			persistMessage(config.SessionStore, state.SessionID, last)
		}
	}

	// 3.5 Initialize session memory tracker if enabled
	var memTracker *SessionMemoryTracker
	if config.SessionMemoryEnabled && config.SessionDir != "" {
		memTracker = NewSessionMemoryTracker(config.SessionDir, config.LLMClient, config.Prompter)
	}

	// 4. Assemble system prompt
	systemPrompt := config.Prompter.Assemble(config)

	// 4.5 Dynamic model selection (first turn only, based on prompt complexity)
	if config.DynamicModelConfig != nil && state.Model == "" {
		dmc := config.DynamicModelConfig
		promptTokens := len(prompt) / 4 // rough estimate
		if dmc.SimpleThresholdTokens > 0 && promptTokens < dmc.SimpleThresholdTokens && dmc.SimpleModel != "" {
			state.Model = dmc.SimpleModel
		} else if dmc.ComplexThresholdTokens > 0 && promptTokens > dmc.ComplexThresholdTokens && dmc.ComplexModel != "" {
			state.Model = dmc.ComplexModel
		}
		// else: use config.Model (default)
	}

	// 5. Main loop
	for {
		// Process any pending control requests (non-blocking)
		if config.MultiTurn {
			processControlRequests(config, state, q)
		}

		// Check termination conditions
		if reason := checkTermination(ctx, config, state); reason != "" {
			state.ExitReason = reason
			break
		}

		// 5.4 Budget-aware model downgrade
		if config.BudgetDowngradeThreshold > 0 && config.MaxBudgetUSD > 0 &&
			config.BudgetDowngradeModel != "" && !state.BudgetDowngraded {
			threshold := config.MaxBudgetUSD * config.BudgetDowngradeThreshold
			if state.TotalCostUSD >= threshold {
				state.Model = config.BudgetDowngradeModel
				state.BudgetDowngraded = true
			}
		}

		// 5.4b Background session memory extraction
		if memTracker != nil && memTracker.ShouldExtract() {
			go memTracker.Extract(ctx, state.Messages)
		}

		// 5.5 Proactive compaction check
		budget := calculateTokenBudget(config, state, systemPrompt)
		if config.Compactor.ShouldCompact(budget) {
			compacted, err := config.Compactor.Compact(ctx, CompactRequest{
				Messages:   state.Messages,
				Model:      config.Model,
				Budget:     budget,
				Trigger:    "auto",
				SessionID:  state.SessionID,
				SessionDir: config.SessionDir,
				EmitCh:     ch,
			})
			if err == nil {
				state.Messages = compacted
			}
			// On error, continue with uncompacted messages
		}

		// 6. Build completion request (inject pending additional context)
		var llmTools []llm.Tool
		if config.ToolRegistry != nil {
			llmTools = config.ToolRegistry.LLMTools()
		}

		effectivePrompt := systemPrompt
		if len(state.PendingAdditionalContext) > 0 {
			effectivePrompt = systemPrompt + "\n\n" + strings.Join(state.PendingAdditionalContext, "\n")
			state.PendingAdditionalContext = nil // clear after consumption
		}

		// Use dynamic model from LoopState if set, otherwise config
		model := config.Model
		if state.Model != "" {
			model = state.Model
		}

		// Resolve MaxThinkingTokens: state override > config > default (0)
		maxThinkingTokens := 0
		if state.MaxThinkingTokens > 0 {
			maxThinkingTokens = state.MaxThinkingTokens
		} else if config.MaxThinkingTkns != nil {
			maxThinkingTokens = *config.MaxThinkingTkns
		}

		req := llm.BuildCompletionRequest(
			llm.ClientConfig{
				Model:             model,
				MaxTokens:         16384,
				MaxThinkingTokens: maxThinkingTokens,
			},
			effectivePrompt,
			state.Messages,
			llmTools,
			llm.LoopState{SessionID: state.SessionID},
		)

		// 7. Call LLM
		apiStart := time.Now()
		stream, err := config.LLMClient.Complete(ctx, req)
		if err != nil {
			// Check if context was cancelled (interrupt/abort)
			if ctx.Err() != nil {
				q.mu.Lock()
				if state.IsInterrupted {
					state.ExitReason = ExitInterrupted
				} else {
					state.ExitReason = ExitAborted
				}
				q.mu.Unlock()
				break
			}
			// Try fallback model on retriable errors (once only)
			if config.FallbackModel != "" && isRetriableModelError(err) && !state.UsingFallback {
				state.UsingFallback = true
				state.Model = config.FallbackModel
				req = llm.BuildCompletionRequest(
					llm.ClientConfig{Model: config.FallbackModel, MaxTokens: 16384, MaxThinkingTokens: maxThinkingTokens},
					effectivePrompt, state.Messages, llmTools,
					llm.LoopState{SessionID: state.SessionID},
				)
				stream, err = config.LLMClient.Complete(ctx, req)
			}
			if err != nil {
				state.LastError = err
				state.ExitReason = ExitReason("error")
				break
			}
		}

		// 8. Accumulate response with streaming callbacks
		var onChunk func(*llm.StreamChunk)
		if config.IncludePartial {
			onChunk = func(chunk *llm.StreamChunk) {
				emitStreamEvent(ch, chunk, state)
			}
		}

		resp, err := stream.AccumulateWithCallback(onChunk)
		apiDuration += time.Since(apiStart)

		if err != nil {
			if ctx.Err() != nil {
				q.mu.Lock()
				if state.IsInterrupted {
					state.ExitReason = ExitInterrupted
				} else {
					state.ExitReason = ExitAborted
				}
				q.mu.Unlock()
				break
			}
			state.LastError = err
			state.ExitReason = ExitReason("error")
			break
		}

		// 9. Update state
		assistantMsg := responseToAssistantMessage(resp)
		state.Messages = append(state.Messages, assistantMsg)

		q.mu.Lock()
		state.TurnCount++
		state.addUsage(resp.Usage)
		if config.CostTracker != nil {
			state.TotalCostUSD = config.CostTracker.Add(resp.Model, resp.Usage)
		} else {
			state.TotalCostUSD += llm.CalculateCost(resp.Model, resp.Usage)
		}
		q.mu.Unlock()

		// 9.5 Track tokens for session memory extraction
		if memTracker != nil {
			memTracker.TrackTokens(resp.Usage.InputTokens, resp.Usage.OutputTokens)
		}

		// 10. Emit assistant message
		emitAssistant(ch, resp, state)

		// 10.5 Persist assistant message
		persistMessage(config.SessionStore, state.SessionID, assistantMsg)

		// 11. Check stop reason
		switch resp.StopReason {
		case "end_turn":
			state.ActiveSkill = nil // clear skill scope on turn end

			// Fire Stop hook and check if any hook wants to continue
			stopResults, _ := config.Hooks.Fire(ctx, types.HookEventStop, nil)
			if shouldContinue(stopResults) {
				collectAdditionalContext(state, stopResults)
				continue
			}

			if config.MultiTurn {
				// Multi-turn: emit per-turn result, then wait for more input
				emitTurnResult(ch, config, state, startTime, apiDuration)

				// Wait for next user message or close
				if waitForInput(ctx, config, state, ch, q) {
					continue // got new input, continue the loop
				}
				// waitForInput returned false → close/interrupt/context cancelled
				state.ExitReason = ExitEndTurn
				goto done
			}

			state.ExitReason = ExitEndTurn
			goto done

		case "max_tokens":
			// Discard any truncated tool_use blocks with incomplete JSON
			discardTruncatedToolBlocks(resp)

			// Check if compaction can help
			budget := calculateTokenBudget(config, state, systemPrompt)
			if config.Compactor.ShouldCompact(budget) {
				compacted, err := config.Compactor.Compact(ctx, CompactRequest{
					Messages:  state.Messages,
					Model:     config.Model,
					Budget:    budget,
					Trigger:   "auto",
					SessionID: state.SessionID,
					EmitCh:    ch,
				})
				if err == nil {
					state.Messages = compacted
					continue
				}
			}
			state.ExitReason = ExitMaxTokens
			goto done

		case "tool_use":
			// Extract tool_use blocks
			toolBlocks := extractToolUseBlocks(resp)

			// Edge case: stop_reason=tool_use but 0 tool blocks → treat as end_turn
			if len(toolBlocks) == 0 {
				state.ExitReason = ExitEndTurn
				goto done
			}

			// Execute tools
			toolResults, interrupted := executeTools(ctx, toolBlocks, config, state, ch)

			// Track tool calls for session memory
			if memTracker != nil {
				for range toolBlocks {
					memTracker.TrackToolCall()
				}
			}

			// Sync active file paths for conditional rules injection
			syncActiveFilePaths(config, state)

			// Activate skill permission scope if a Skill tool was invoked
			setActiveSkillScope(toolBlocks, config, state)

			// Append tool results as messages
			toolMsgs := llm.ConvertToToolMessages(toolResults)
			state.Messages = append(state.Messages, toolMsgs...)

			// Persist tool result messages
			for _, tm := range toolMsgs {
				persistMessage(config.SessionStore, state.SessionID, tm)
			}

			// Lightweight pruning of old tool results to manage context pressure
			state.Messages = pruneOldToolResults(state.Messages, 10)

			if interrupted {
				state.ExitReason = ExitInterrupted
				goto done
			}

			continue

		case "stop_sequence":
			state.ExitReason = ExitStopSequence
			state.StopSequence = resp.StopSequence
			goto done

		default:
			// Unknown stop reason — treat as end_turn
			state.ExitReason = ExitEndTurn
			goto done
		}
	}

done:
	// 11.5 Flush session metadata
	finalizeSession(config, state)

	// 12. Emit result message
	emitResult(ch, config, state, startTime, apiDuration)

	// 13. Fire SessionEnd hook
	config.Hooks.Fire(ctx, types.HookEventSessionEnd, map[string]any{
		"reason": string(state.ExitReason),
	})
}

// waitForInput blocks until the user sends a new message, a control request arrives,
// or the query is closed. Returns true if the loop should continue with new input.
func waitForInput(ctx context.Context, config *AgentConfig, state *LoopState, ch chan<- types.SDKMessage, q *Query) bool {
	for {
		select {
		case msg, ok := <-q.inputCh:
			if !ok {
				return false // input channel closed
			}
			state.ActiveSkill = nil // clear skill scope on new user input
			// Append user message to conversation
			userMsg := llm.ChatMessage{Role: "user", Content: string(msg)}
			state.Messages = append(state.Messages, userMsg)
			persistMessage(config.SessionStore, state.SessionID, userMsg)
			return true

		case req := <-q.controlCh:
			// Process control request and send response
			resp := dispatchControl(config, state, q, req)
			q.controlResp <- resp

			// Check if the control request caused an interrupt
			if state.IsInterrupted {
				return false
			}
			// After control, continue waiting for input
			continue

		case <-q.closeCh:
			return false

		case <-ctx.Done():
			return false
		}
	}
}

// processControlRequests drains any pending control requests (non-blocking).
func processControlRequests(config *AgentConfig, state *LoopState, q *Query) {
	for {
		select {
		case req := <-q.controlCh:
			resp := dispatchControl(config, state, q, req)
			q.controlResp <- resp
		default:
			return
		}
	}
}

// dispatchControl handles a single control request and returns the response.
func dispatchControl(config *AgentConfig, state *LoopState, q *Query, req types.ControlRequest) types.ControlResponse {
	switch req.Request.Subtype {
	case types.ControlSubtypeInterrupt:
		q.mu.Lock()
		state.IsInterrupted = true
		q.mu.Unlock()
		q.cancel()
		return types.ControlResponse{
			Type:     "control_response",
			Response: types.ControlSuccessResponse{RequestID: req.RequestID},
		}

	case types.ControlSubtypeSetModel:
		model := req.Request.Model
		if model == "" {
			return types.ControlResponse{
				Type: "control_response",
				Response: types.ControlErrorResponse{
					RequestID: req.RequestID,
					Error:     "model is required",
				},
			}
		}
		q.mu.Lock()
		state.Model = model
		q.mu.Unlock()
		return types.ControlResponse{
			Type:     "control_response",
			Response: types.ControlSuccessResponse{RequestID: req.RequestID, Result: model},
		}

	case types.ControlSubtypeSetPermissionMode:
		mode := req.Request.Mode
		if mode == "" {
			return types.ControlResponse{
				Type: "control_response",
				Response: types.ControlErrorResponse{
					RequestID: req.RequestID,
					Error:     "mode is required",
				},
			}
		}
		config.PermissionMode = mode
		return types.ControlResponse{
			Type:     "control_response",
			Response: types.ControlSuccessResponse{RequestID: req.RequestID, Result: string(mode)},
		}

	case types.ControlSubtypeSetMaxThinkingTokens:
		if req.Request.MaxThinkingTokens == nil {
			return types.ControlResponse{
				Type: "control_response",
				Response: types.ControlErrorResponse{
					RequestID: req.RequestID,
					Error:     "max_thinking_tokens is required",
				},
			}
		}
		q.mu.Lock()
		state.MaxThinkingTokens = *req.Request.MaxThinkingTokens
		q.mu.Unlock()
		return types.ControlResponse{
			Type:     "control_response",
			Response: types.ControlSuccessResponse{RequestID: req.RequestID, Result: *req.Request.MaxThinkingTokens},
		}

	default:
		return types.ControlResponse{
			Type: "control_response",
			Response: types.ControlErrorResponse{
				RequestID: req.RequestID,
				Error:     "unknown control subtype: " + req.Request.Subtype,
			},
		}
	}
}

// shouldContinue checks if any hook result has Continue=true.
func shouldContinue(results []HookResult) bool {
	for _, r := range results {
		if r.Continue != nil && *r.Continue {
			return true
		}
	}
	return false
}

// collectAdditionalContext extracts additional context from hook results
// and appends it to the loop state's pending context.
func collectAdditionalContext(state *LoopState, results []HookResult) {
	for _, r := range results {
		if r.SystemMessage != "" {
			state.PendingAdditionalContext = append(state.PendingAdditionalContext, r.SystemMessage)
		}
	}
}

// checkTermination evaluates whether the loop should stop.
func checkTermination(ctx context.Context, config *AgentConfig, state *LoopState) ExitReason {
	// Check context
	select {
	case <-ctx.Done():
		if state.IsInterrupted {
			return ExitInterrupted
		}
		return ExitAborted
	default:
	}

	// Check turn limit
	if config.MaxTurns > 0 && state.TurnCount >= config.MaxTurns {
		return ExitMaxTurns
	}

	// Check budget
	if config.MaxBudgetUSD > 0 && state.TotalCostUSD >= config.MaxBudgetUSD {
		return ExitMaxBudget
	}

	// Check per-model budgets
	if len(config.ModelBudgets) > 0 && config.CostTracker != nil {
		breakdown := config.CostTracker.ModelBreakdown()
		for model, limit := range config.ModelBudgets {
			if accum, ok := breakdown[model]; ok && accum.CostUSD >= limit {
				return ExitMaxBudget
			}
		}
	}

	return "" // no termination
}

// calculateTokenBudget estimates the current token budget for context management.
func calculateTokenBudget(config *AgentConfig, state *LoopState, systemPrompt string) TokenBudget {
	// Estimate message tokens using the simple len/4 heuristic
	msgTokens := 0
	for _, msg := range state.Messages {
		msgTokens += estimateMessageTokens(msg)
	}
	sysTokens := len(systemPrompt) / 4

	// Use actual context limit for current model
	contextLimit := 200_000
	if config.ContextLimitFunc != nil {
		model := config.Model
		if state.Model != "" {
			model = state.Model
		}
		contextLimit = config.ContextLimitFunc(model, config.Betas)
	}

	return TokenBudget{
		ContextLimit:     contextLimit,
		SystemPromptTkns: sysTokens,
		MaxOutputTkns:    16384,
		MessageTkns:      msgTokens,
	}
}

// isRetriableModelError checks if the error is a retriable model error
// (rate limit, service unavailable, model not found).
func isRetriableModelError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "model_not_found") ||
		strings.Contains(errStr, "rate_limit")
}

// estimateMessageTokens estimates the token count for a single message.
func estimateMessageTokens(msg llm.ChatMessage) int {
	overhead := 4 // role + separators
	switch c := msg.Content.(type) {
	case string:
		return len(c)/4 + overhead
	case nil:
		return overhead
	default:
		// For complex content ([]ContentPart etc), use a rough estimate
		_ = c
		return overhead
	}
}

// persistMessage writes a ChatMessage to the session store as a MessageEntry.
// Errors are logged but not fatal — persistence is best-effort.
func persistMessage(store SessionStore, sessionID string, msg llm.ChatMessage) {
	if store == nil {
		return
	}
	entry := MessageEntry{
		UUID:      uuid.New().String(),
		Timestamp: time.Now(),
		Message:   msg,
	}
	_ = store.AppendMessage(sessionID, entry)
}

// persistSDKMessage writes an SDKMessage to the transcript log.
func persistSDKMessage(store SessionStore, sessionID string, msg types.SDKMessage) {
	if store == nil {
		return
	}
	_ = store.AppendSDKMessage(sessionID, msg)
}

// initializeSession creates or restores a session based on QueryOptions embedded in config.
func initializeSession(config *AgentConfig, state *LoopState) {
	if config.SessionStore == nil {
		return
	}

	// Check for resume/continue/fork (via stored options on config)
	// For now, create a new session if one doesn't exist
	meta := SessionMetadata{
		ID:        state.SessionID,
		CWD:       config.CWD,
		Model:     config.Model,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = config.SessionStore.Create(meta)
}

// finalizeSession updates session metadata with final stats and checkpoints accessed files.
func finalizeSession(config *AgentConfig, state *LoopState) {
	if config.SessionStore == nil {
		return
	}
	_ = config.SessionStore.UpdateMetadata(state.SessionID, func(m *SessionMetadata) {
		m.MessageCount = len(state.Messages)
		m.TurnCount = state.TurnCount
		m.TotalCostUSD = state.TotalCostUSD
		m.ExitReason = string(state.ExitReason)
		if config.AgentName != "" {
			m.AgentName = config.AgentName
		}
	})

	// Checkpoint accessed files for session rewind support
	if len(state.AccessedFiles) > 0 {
		paths := make([]string, 0, len(state.AccessedFiles))
		for p := range state.AccessedFiles {
			paths = append(paths, p)
		}
		// Best-effort: errors are non-fatal
		_ = config.SessionStore.CreateCheckpoint(state.SessionID, "session-end", paths)
	}
}

// RestoreSession loads a previous session's messages into the loop state.
// Called by the host app before RunLoop to set up resume/continue/fork.
func RestoreSession(config *AgentConfig, state *LoopState, opts types.QueryOptions) error {
	if config.SessionStore == nil {
		return nil
	}

	var sessionState *SessionState
	var err error

	switch {
	case opts.ForkSession && opts.Resume != "":
		// Fork from an existing session
		sessionState, err = config.SessionStore.Fork(opts.Resume, state.SessionID)

	case opts.Continue:
		// Resume the latest session for the CWD
		sessionState, err = config.SessionStore.LoadLatest(config.CWD)

	case opts.Resume != "":
		// Resume a specific session
		if opts.ResumeSessionAt != "" {
			var entries []MessageEntry
			entries, err = config.SessionStore.LoadMessagesUpTo(opts.Resume, opts.ResumeSessionAt)
			if err == nil {
				sessionState = &SessionState{Messages: entries}
			}
		} else {
			sessionState, err = config.SessionStore.Load(opts.Resume)
		}
	}

	if err != nil {
		return err
	}

	if sessionState != nil && len(sessionState.Messages) > 0 {
		// Restore messages into loop state
		msgs := make([]llm.ChatMessage, len(sessionState.Messages))
		for i, entry := range sessionState.Messages {
			msgs[i] = entry.Message
		}
		state.Messages = msgs
		if sessionState.Metadata.ID != "" {
			state.SessionID = sessionState.Metadata.ID
		}
	}

	return nil
}
