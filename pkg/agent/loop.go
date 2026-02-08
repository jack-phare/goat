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
		messages: ch,
		done:     make(chan struct{}),
		state:    state,
		cancel:   cancel,
	}

	go runLoop(loopCtx, prompt, &config, state, ch, q)

	return q
}

func runLoop(ctx context.Context, prompt string, config *AgentConfig, state *LoopState, ch chan<- types.SDKMessage, q *Query) {
	defer close(ch)
	defer close(q.done)

	startTime := time.Now()
	var apiDuration time.Duration

	// 1. Fire SessionStart hook and collect additional context
	sessionStartResults, _ := config.Hooks.Fire(ctx, types.HookEventSessionStart, map[string]any{
		"source": "startup",
	})
	collectAdditionalContext(state, sessionStartResults)

	// 2. Emit system init message
	emitInit(ch, config, state)

	// 3. Build initial messages
	state.Messages = []llm.ChatMessage{
		{Role: "user", Content: prompt},
	}

	// 4. Assemble system prompt
	systemPrompt := config.Prompter.Assemble(config)

	// 5. Main loop
	for {
		// Check termination conditions
		if reason := checkTermination(ctx, config, state); reason != "" {
			state.ExitReason = reason
			break
		}

		// 5.5 Proactive compaction check
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

		req := llm.BuildCompletionRequest(
			llm.ClientConfig{
				Model:             config.Model,
				MaxTokens:         16384,
				MaxThinkingTokens: 0,
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
			// LLM error — emit and break
			state.ExitReason = ExitReason("error")
			break
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

		// 10. Emit assistant message
		emitAssistant(ch, resp, state)

		// 11. Check stop reason
		switch resp.StopReason {
		case "end_turn":
			// Fire Stop hook and check if any hook wants to continue
			stopResults, _ := config.Hooks.Fire(ctx, types.HookEventStop, nil)
			if shouldContinue(stopResults) {
				collectAdditionalContext(state, stopResults)
				continue
			}
			state.ExitReason = ExitEndTurn
			goto done

		case "max_tokens":
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

			// Append tool results as messages
			toolMsgs := llm.ConvertToToolMessages(toolResults)
			state.Messages = append(state.Messages, toolMsgs...)

			if interrupted {
				state.ExitReason = ExitInterrupted
				goto done
			}

			continue

		default:
			// Unknown stop reason — treat as end_turn
			state.ExitReason = ExitEndTurn
			goto done
		}
	}

done:
	// 12. Emit result message
	emitResult(ch, state, startTime, apiDuration)

	// 13. Fire SessionEnd hook
	config.Hooks.Fire(ctx, types.HookEventSessionEnd, map[string]any{
		"reason": string(state.ExitReason),
	})
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

	contextLimit := 200_000 // default for Claude models
	return TokenBudget{
		ContextLimit:     contextLimit,
		SystemPromptTkns: sysTokens,
		MaxOutputTkns:    16384,
		MessageTkns:      msgTokens,
	}
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
		return overhead
	}
}
