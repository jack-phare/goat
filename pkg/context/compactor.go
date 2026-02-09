package context

import (
	"context"

	"github.com/google/uuid"
	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

// Compile-time verification that Compactor implements ContextCompactor.
var _ agent.ContextCompactor = (*Compactor)(nil)

// CompactorConfig holds configuration for the Compactor.
type CompactorConfig struct {
	LLMClient     llm.Client
	HookRunner    agent.HookRunner
	Estimator     TokenEstimator // default: SimpleEstimator
	SummaryModel  string         // default: "claude-haiku-4-5-20251001"
	ThresholdPct  float64        // default: 0.80
	CriticalPct   float64        // default: 0.95
	PreserveRatio float64        // default: 0.40
}

// Compactor implements context window management via conversation summarization.
type Compactor struct {
	client        llm.Client
	hooks         agent.HookRunner
	estimator     TokenEstimator
	summaryModel  string
	thresholdPct  float64
	criticalPct   float64
	preserveRatio float64
}

// NewCompactor creates a Compactor with sensible defaults for any unset config fields.
func NewCompactor(cfg CompactorConfig) *Compactor {
	c := &Compactor{
		client:        cfg.LLMClient,
		hooks:         cfg.HookRunner,
		estimator:     cfg.Estimator,
		summaryModel:  cfg.SummaryModel,
		thresholdPct:  cfg.ThresholdPct,
		criticalPct:   cfg.CriticalPct,
		preserveRatio: cfg.PreserveRatio,
	}
	if c.estimator == nil {
		c.estimator = &SimpleEstimator{}
	}
	if c.summaryModel == "" {
		c.summaryModel = "claude-haiku-4-5-20251001"
	}
	if c.thresholdPct == 0 {
		c.thresholdPct = 0.80
	}
	if c.criticalPct == 0 {
		c.criticalPct = 0.95
	}
	if c.preserveRatio == 0 {
		c.preserveRatio = 0.40
	}
	if c.hooks == nil {
		c.hooks = &agent.NoOpHookRunner{}
	}
	return c
}

// ShouldCompact returns true if the context utilization exceeds the threshold.
func (c *Compactor) ShouldCompact(budget agent.TokenBudget) bool {
	return budget.UtilizationPct() > c.thresholdPct
}

// MustCompact returns true if the context utilization exceeds the critical threshold (0.95).
// This is used for mandatory compaction on max_tokens stop reason.
func (c *Compactor) MustCompact(budget agent.TokenBudget) bool {
	return budget.UtilizationPct() > c.criticalPct
}

// Compact summarizes older messages, keeping the most recent messages verbatim.
// On summary generation failure, it falls back to simple truncation.
func (c *Compactor) Compact(ctx context.Context, req agent.CompactRequest) ([]llm.ChatMessage, error) {
	if len(req.Messages) <= 1 {
		return req.Messages, nil
	}

	// 1. Fire PreCompact hook and capture custom instructions
	var customInstructions *string
	if c.hooks != nil {
		results, _ := c.hooks.Fire(ctx, types.HookEventPreCompact, map[string]any{
			"trigger": req.Trigger,
		})
		for _, hr := range results {
			if hr.HookSpecificOutput != nil {
				if output, ok := hr.HookSpecificOutput.(map[string]any); ok {
					if ci, ok := output["custom_instructions"].(string); ok && ci != "" {
						customInstructions = &ci
						break
					}
				}
			}
		}
	}

	// 2. Record pre-compaction token count
	preTokens := c.estimator.EstimateMessages(req.Messages)

	// 3. Calculate split point
	preserveBudget := int(float64(req.Budget.ContextLimit) * c.preserveRatio)
	splitIdx := calculateSplitPoint(req.Messages, preserveBudget, c.estimator)

	if splitIdx <= 0 || splitIdx >= len(req.Messages) {
		// Nothing to compact
		return req.Messages, nil
	}

	compactZone := req.Messages[:splitIdx]
	preserveZone := req.Messages[splitIdx:]

	// 4. Generate summary via LLM call
	var compacted []llm.ChatMessage
	if c.client != nil {
		summary, err := generateSummary(ctx, compactZone, c.client, c.summaryModel, customInstructions)
		if err != nil {
			// Fallback: simple truncation (drop oldest messages)
			compacted = preserveZone
		} else {
			summaryMsg := llm.ChatMessage{
				Role:    "user",
				Content: "[Previous conversation summary]\n\n" + summary,
			}
			compacted = append([]llm.ChatMessage{summaryMsg}, preserveZone...)
		}
	} else {
		// No LLM client — simple truncation fallback
		compacted = preserveZone
	}

	// 5. Emit CompactBoundaryMessage
	if req.EmitCh != nil {
		msg := &types.CompactBoundaryMessage{
			BaseMessage: types.BaseMessage{UUID: uuid.New(), SessionID: req.SessionID},
			Type:        types.MessageTypeSystem,
			Subtype:     types.SystemSubtypeCompactBoundary,
			CompactMetadata: types.CompactMetadata{
				Trigger:   req.Trigger,
				PreTokens: preTokens,
			},
		}
		req.EmitCh <- msg
	}

	// 6. Fire SessionStart hook with source="compact"
	if c.hooks != nil {
		c.hooks.Fire(ctx, types.HookEventSessionStart, map[string]any{
			"source": "compact",
		})
	}

	return compacted, nil
}
