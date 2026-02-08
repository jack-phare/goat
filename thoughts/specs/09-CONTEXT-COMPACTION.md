# Spec 09: Context & Compaction

**Go Package**: `pkg/context/`
**Source References**:
- `sdk.d.ts:1162-1171` — SDKCompactBoundaryMessage (type, subtype, compact_metadata with trigger + pre_tokens, uuid, session_id)
- `sdk.d.ts:927-931` — PreCompactHookInput (trigger, custom_instructions)
- `sdk.d.ts:1606-1612` — SessionStartHookInput with source: "compact"
- `sdk.d.ts:578` — Options.betas (context-1m-2025-08-07)
- Piebald-AI: Compact/summary utility prompts

---

## 1. Purpose

Context window management prevents the conversation from exceeding the model's token limit. When the conversation approaches the limit, compaction summarizes earlier messages while preserving recent context and critical information.

---

## 2. Context Window Limits

```go
// ModelContextLimits maps models to their context window sizes.
var ModelContextLimits = map[string]int{
    "claude-sonnet-4-5-20250929": 200_000,
    "claude-opus-4-5-20250514":   200_000,
    "claude-haiku-4-5-20251001":  200_000,
    // With beta context-1m-2025-08-07:
    // "claude-sonnet-4-5-20250929": 1_000_000,
}

// TokenBudget calculates available tokens for the next LLM call.
type TokenBudget struct {
    ContextLimit     int // model's total context window
    SystemPromptTkns int // estimated system prompt tokens
    MaxOutputTkns    int // reserved for output (default 16384)
    MessageTkns      int // current message history tokens
    Available        int // remaining for new content
}

func (b *TokenBudget) IsOverflow() bool {
    return b.MessageTkns + b.SystemPromptTkns + b.MaxOutputTkns > b.ContextLimit
}

func (b *TokenBudget) UtilizationPct() float64 {
    used := b.SystemPromptTkns + b.MessageTkns + b.MaxOutputTkns
    return float64(used) / float64(b.ContextLimit)
}
```

---

## 3. Compaction Triggers

```go
type CompactTrigger string

const (
    TriggerAuto   CompactTrigger = "auto"   // context utilization > threshold
    TriggerManual CompactTrigger = "manual" // user /compact command
)

// ShouldCompact checks if compaction is needed.
func ShouldCompact(budget TokenBudget) bool {
    return budget.UtilizationPct() > 0.80 // 80% threshold
}

// MustCompact checks if compaction is critical (would overflow on next call).
func MustCompact(budget TokenBudget) bool {
    return budget.UtilizationPct() > 0.95
}
```

---

## 4. Compaction Algorithm

### 4.1 Overview

```
┌──────────────────────────────────────────────────────────┐
│                  Full Conversation                         │
│ [sys_init] [user1] [asst1] [user2] [asst2] ... [userN]   │
│                                                            │
│ ┌─────────────────────┐ ┌──────────────────────────────┐ │
│ │   COMPACT ZONE       │ │      PRESERVE ZONE           │ │
│ │ (summarized)         │ │ (kept verbatim)              │ │
│ │ messages 1..K        │ │ messages K+1..N              │ │
│ └─────────────────────┘ └──────────────────────────────┘ │
│                                                            │
│ ┌─────────────────────┐ ┌──────────────────────────────┐ │
│ │  SUMMARY MESSAGE     │ │      PRESERVE ZONE           │ │
│ │ (replaces compact    │ │ (unchanged)                  │ │
│ │  zone)               │ │                              │ │
│ └─────────────────────┘ └──────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

### 4.2 Step-by-Step

```go
func Compact(state *LoopState, trigger CompactTrigger, customInstructions *string) error {
    // 1. Fire PreCompact hook
    hookResults := hooks.Fire(HookPreCompact, &PreCompactHookInput{
        Trigger:            string(trigger),
        CustomInstructions: customInstructions,
    })
    // Hook can modify custom_instructions

    // 2. Calculate split point
    splitIdx := calculateSplitPoint(state.Messages)

    // 3. Extract messages to compact
    compactZone := state.Messages[:splitIdx]
    preserveZone := state.Messages[splitIdx:]

    // 4. Generate summary via LLM call
    summary, err := generateSummary(compactZone, customInstructions)
    if err != nil {
        return fmt.Errorf("compaction summary failed: %w", err)
    }

    // 5. Record pre-compaction token count
    preTokens := estimateTokens(state.Messages)

    // 6. Replace compact zone with summary message
    compactBoundary := SDKCompactBoundaryMessage{
        Type:    "system",
        Subtype: "compact_boundary",
        CompactMetadata: CompactMetadata{
            Trigger:   string(trigger),
            PreTokens: preTokens,
        },
        UUID:      uuid.New().String(),
        SessionID: state.SessionID,
    }

    summaryMessage := SystemMessage{
        Content: summary,
        Role:    "system", // injected as context
    }

    // 7. Rebuild message history
    state.Messages = append(
        []Message{summaryMessage},
        preserveZone...,
    )

    // 8. Emit compact boundary message
    emit(compactBoundary)

    // 9. Fire SessionStart hook with source="compact"
    hooks.Fire(HookSessionStart, &SessionStartHookInput{
        Source: "compact",
    })

    return nil
}
```

### 4.3 Split Point Calculation

```go
// calculateSplitPoint determines where to split the conversation.
// Strategy: keep the most recent K messages that fit within the preserve budget.
func calculateSplitPoint(messages []Message) int {
    // Target: preserve zone should be ~40% of context window
    // Compact zone: everything before that
    preserveBudget := getContextLimit() * 40 / 100

    tokens := 0
    for i := len(messages) - 1; i >= 0; i-- {
        tokens += estimateTokens(messages[i])
        if tokens > preserveBudget {
            return i + 1 // split here
        }
    }
    return 0 // compact everything (shouldn't happen in practice)
}
```

---

## 5. Summary Generation

The compaction summary is generated by an LLM call with a specialized prompt:

```go
func generateSummary(messages []Message, customInstructions *string) (string, error) {
    summaryPrompt := buildCompactionPrompt(messages, customInstructions)

    // Use a cheaper/faster model for summarization
    request := llm.CompletionRequest{
        Model:    "claude-haiku-4-5-20251001", // faster for summarization
        Messages: []llm.Message{
            {Role: "user", Content: summaryPrompt},
        },
        MaxTokens: 4096,
        Stream:    false,
    }

    response, err := llmClient.CompleteSync(ctx, request)
    if err != nil {
        return "", err
    }

    return response.Content, nil
}

func buildCompactionPrompt(messages []Message, customInstructions *string) string {
    // Based on Piebald compact utility prompt
    prompt := `Summarize the following conversation, preserving:
1. Key decisions and their rationale
2. File paths and code changes made
3. Unresolved questions or pending tasks
4. User preferences and constraints mentioned
5. Tool outputs that are still relevant

Be concise but complete. Use structured format with sections.`

    if customInstructions != nil {
        prompt += "\n\nAdditional instructions: " + *customInstructions
    }

    prompt += "\n\n--- CONVERSATION TO SUMMARIZE ---\n"
    for _, msg := range messages {
        prompt += fmt.Sprintf("[%s]: %s\n\n", msg.Role, truncateContent(msg.Content, 2000))
    }

    return prompt
}
```

---

## 6. Token Estimation

```go
// EstimateTokens provides a rough token count.
// For accurate counts, use tiktoken or the Anthropic token counting API.
type TokenEstimator interface {
    Estimate(text string) int
    EstimateMessages(messages []Message) int
}

// SimpleEstimator uses character-based heuristic (~4 chars/token for English).
type SimpleEstimator struct{}

func (e *SimpleEstimator) Estimate(text string) int {
    return len(text) / 4
}

func (e *SimpleEstimator) EstimateMessages(messages []Message) int {
    total := 0
    for _, msg := range messages {
        total += e.Estimate(msg.ContentString())
        total += 4 // overhead per message (role, separators)
    }
    return total
}

// TiktokenEstimator uses the cl100k_base tokenizer (more accurate).
type TiktokenEstimator struct {
    encoder *tiktoken.Encoding
}
```

---

## 7. Compact Boundary Message (from `sdk.d.ts:1162-1171`)

```go
// SDKCompactBoundaryMessage is emitted when compaction occurs.
type SDKCompactBoundaryMessage struct {
    Type            string          `json:"type"`    // "system"
    Subtype         string          `json:"subtype"` // "compact_boundary"
    CompactMetadata CompactMetadata `json:"compact_metadata"`
    UUID            string          `json:"uuid"`
    SessionID       string          `json:"session_id"`
}

type CompactMetadata struct {
    Trigger   string `json:"trigger"`    // "manual" | "auto"
    PreTokens int    `json:"pre_tokens"` // token count before compaction
}
```

---

## 8. Beta Context Window Extension

```go
// When beta "context-1m-2025-08-07" is enabled, context limit increases to 1M tokens.
func getContextLimit(model string, betas []string) int {
    base := ModelContextLimits[model]
    for _, beta := range betas {
        if beta == "context-1m-2025-08-07" {
            // Only supported for Sonnet 4/4.5
            if strings.Contains(model, "sonnet") {
                return 1_000_000
            }
        }
    }
    return base
}
```

---

## 9. Conversation History Pruning

Beyond compaction, the context manager handles incremental pruning:

```go
// PruneOldToolResults replaces verbose tool outputs with summaries.
// This is less aggressive than full compaction.
func PruneOldToolResults(messages []Message, preserveRecent int) []Message {
    result := make([]Message, len(messages))
    copy(result, messages)

    for i := 0; i < len(result)-preserveRecent; i++ {
        if result[i].Role == "tool" {
            content := result[i].ContentString()
            if len(content) > 1000 {
                result[i].SetContent(truncateToolOutput(content, 200))
            }
        }
    }
    return result
}
```

---

## 10. Verification Checklist

- [ ] **Trigger threshold**: Auto-compact fires at 80% context utilization
- [ ] **Split point**: Preserves ~40% of context as recent messages
- [ ] **Summary quality**: Compaction summary retains key decisions, file paths, pending tasks
- [ ] **CompactBoundary emission**: Message emitted with correct trigger and pre_tokens
- [ ] **Hook firing**: PreCompact fires before, SessionStart(compact) fires after
- [ ] **Token estimation**: Estimates within 20% of actual token counts
- [ ] **Beta support**: 1M context window activated with correct beta flag
- [ ] **Message integrity**: Preserved zone messages unchanged after compaction
- [ ] **Multi-compact**: Multiple compactions in one session work correctly
- [ ] **Cost efficiency**: Summary generation uses cheaper model (Haiku)
- [ ] **Graceful degradation**: If summary generation fails, fall back to simple truncation
