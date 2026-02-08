# Spec 02: Message Types — Implementation Plan

## Overview
Implement the complete `pkg/types/` package as defined in Spec 02. This gives us the canonical message taxonomy for the entire system: all 16 SDKMessage variants, the SDKMessage interface, JSON marshaling/unmarshaling, control protocol types, QueryOptions, and all supporting enums.

## Current State
- `pkg/types/messages.go` — Minimal stub with `MessageType` (2 values), `PartialAssistantMessage`, `AssistantMessage`
- `pkg/llm/types_internal.go` — `BetaMessage`, `ContentBlock`, `BetaUsage`, `CompletionResponse` defined locally
- `pkg/llm/emit.go` — bridges `pkg/llm` → `pkg/types` for 2 message types
- 6 production files + 5 test files in `pkg/llm/` reference the types to move

## Desired End State
- `pkg/types/` is the single source of truth for all protocol types
- `BetaMessage`, `ContentBlock`, `BetaUsage` live in `pkg/types/` (moved from `pkg/llm/`)
- All 16 SDKMessage variants implemented with `SDKMessage` interface
- JSON round-trip works for all message types via `UnmarshalSDKMessage`
- `ContentBlock.MarshalJSON()` produces clean type-specific JSON
- Control protocol types (`ControlRequest`/`ControlResponse`) defined
- `QueryOptions` and all supporting config types defined
- All existing `pkg/llm/` tests still pass after the migration
- New tests for JSON round-trip, ContentBlock marshaling, discriminator routing

## Phases

### Phase 1: Move Shared Types to pkg/types/
**Goal**: Relocate `BetaMessage`, `ContentBlock`, `BetaUsage` from `pkg/llm/` to `pkg/types/` and update all references. Zero new functionality — pure refactor.

**Changes**:

1. **`pkg/types/content.go`** (NEW) — Define moved types:
   - `ContentBlock` struct (from `pkg/llm/types_internal.go:18-31`)
   - `ContentBlock.MarshalJSON()` — custom marshaling per block type (spec §5.2)
   - `BetaUsage` struct (from `pkg/llm/types_internal.go:35-40`)
   - `BetaMessage` struct (from `pkg/llm/types_internal.go:5-14`) — references `ContentBlock` and `BetaUsage`

2. **`pkg/llm/types_internal.go`** — Remove `BetaMessage`, `ContentBlock`, `BetaUsage` definitions. Keep `CompletionResponse` (it references `ToolCall` from wire types). Update `CompletionResponse` fields to reference `types.ContentBlock` and `types.BetaUsage`.

3. **`pkg/llm/translate.go`** — Add `types` import, update `translateUsage` return type to `types.BetaUsage`, update `ToBetaMessage()` return type to `types.BetaMessage`.

4. **`pkg/llm/stream.go`** — Add `types` import, update `ContentBlock` and `BetaUsage` references to `types.ContentBlock` and `types.BetaUsage`.

5. **`pkg/llm/cost.go`** — Add `types` import, update `BetaUsage` parameter types.

6. **`pkg/llm/request.go`** — Add `types` import, update `ContentBlock` parameter type.

7. **`pkg/llm/emit.go`** — Already imports `types`. Update `AssistantMessage.Message` field type from `any` to `types.BetaMessage` (spec §4.3). Adjust `chunkToStreamEvent` if needed.

8. **`pkg/types/messages.go`** — Update `AssistantMessage.Message` field from `any` to `BetaMessage`. Update `AssistantMessage.Error` from `*string` to `*AssistantError` (spec §4.3).

9. **Test files** (5 files in `pkg/llm/`) — Add `types` import, prefix type references with `types.`.

**Success Criteria**:
- Automated:
  - [x] `go build ./...` compiles
  - [x] `go test ./pkg/llm/... -race` — all existing tests pass
  - [x] `go test ./pkg/types/... -race` — passes (even if no new tests yet)
  - [x] `go vet ./...` clean

### Phase 2: SDKMessage Interface & Core Message Types
**Goal**: Define the `SDKMessage` interface, `BaseMessage`, and all 16 concrete message types from spec §4.

**Changes**:

1. **`pkg/types/messages.go`** — Expand significantly:
   - `SDKMessage` interface: `GetType()`, `GetUUID()`, `GetSessionID()` (spec §4.2)
   - `BaseMessage` struct with `UUID uuid.UUID`, `SessionID string` and method impls (spec §4.2)
   - Expand `MessageType` enum: add `user`, `result`, `system`, `tool_progress`, `auth_status`, `tool_use_summary` (spec §4.1)
   - `SystemSubtype` enum (8 values) (spec §4.1)
   - `ResultSubtype` enum (5 values) (spec §4.1)
   - `AssistantError` type + 6 constants (spec §4.3)

2. **`pkg/types/messages.go`** — Refactor existing types:
   - `AssistantMessage` — embed `BaseMessage`, implement `SDKMessage`, add `Message BetaMessage` (typed), add `ParentToolUseID *string`, `Error *AssistantError` (spec §4.3)
   - `PartialAssistantMessage` — embed `BaseMessage`, implement `SDKMessage` (spec §4.5)

3. **`pkg/types/message_user.go`** (NEW):
   - `UserMessage` (spec §4.6)
   - `UserMessageReplay` (spec §4.6)
   - `MessageParam` (spec §4.6)

4. **`pkg/types/message_result.go`** (NEW):
   - `ResultMessage` with all fields (spec §4.7)
   - `ModelUsage` (spec §4.7)
   - `PermissionDenial` (spec §4.7)

5. **`pkg/types/message_system.go`** (NEW):
   - `SystemInitMessage` (spec §4.8)
   - `StatusMessage` (spec §4.9)
   - `CompactBoundaryMessage`, `CompactMetadata` (spec §4.10)
   - `McpServerInfo`, `PluginInfo` (spec §4.8)

6. **`pkg/types/message_hooks.go`** (NEW):
   - `HookStartedMessage` (spec §4.11)
   - `HookProgressMessage` (spec §4.11)
   - `HookResponseMessage` (spec §4.11)

7. **`pkg/types/message_misc.go`** (NEW):
   - `ToolProgressMessage` (spec §4.12)
   - `AuthStatusMessage` (spec §4.13)
   - `TaskNotificationMessage` (spec §4.14)
   - `FilesPersistedEvent`, `PersistedFile`, `FailedFile` (spec §4.15)
   - `ToolUseSummaryMessage` (spec §4.16)

8. **`pkg/llm/emit.go`** — Update emission functions to use `BaseMessage` + `uuid.New()` pattern. `AssistantMessage.Message` now typed as `BetaMessage` instead of `any`.

**Success Criteria**:
- Automated:
  - [x] `go build ./...` compiles
  - [x] `go test ./pkg/llm/... -race` — existing tests pass
  - [x] `go vet ./...` clean
- Manual:
  - [ ] All 16 message types implement `SDKMessage` interface (verified by compiler)

### Phase 3: JSON Marshaling & Unmarshaling
**Goal**: Implement `UnmarshalSDKMessage` and custom ContentBlock marshaling for wire-format fidelity.

**Changes**:

1. **`pkg/types/unmarshal.go`** (NEW):
   - `RawSDKMessage` discriminator struct (spec §5.1)
   - `UnmarshalSDKMessage(data []byte) (SDKMessage, error)` — two-pass deserialization (spec §5.1)
   - `unmarshalSystemMessage(data []byte, subtype *SystemSubtype) (SDKMessage, error)` (spec §5.1)

2. **`pkg/types/content.go`** — Add `ContentBlock.MarshalJSON()` (spec §5.2):
   - `type="text"`: only `type` + `text` fields
   - `type="tool_use"`: only `type` + `id` + `name` + `input` fields
   - `type="thinking"`: only `type` + `thinking` fields
   - default: fallback marshal all fields

3. **`pkg/types/unmarshal_test.go`** (NEW):
   - Round-trip tests for all 16 message types
   - `UserMessageReplay` detection (isReplay probe)
   - System message subtype routing (all 8 subtypes)
   - Unknown type / missing subtype error cases
   - ContentBlock marshal tests (verify clean JSON per type)

4. **`pkg/types/content_test.go`** (NEW):
   - `ContentBlock.MarshalJSON()` tests for each block type
   - Verify no `null`/`""` leakage for irrelevant fields

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/types/... -race` — all round-trip tests pass
  - [x] `go test ./pkg/llm/... -race` — existing tests still pass
  - [x] `go vet ./...` clean
- Manual:
  - [ ] `ContentBlock` JSON output matches Anthropic wire format exactly (spot-check text, tool_use, thinking)

### Phase 4: Message Constructors
**Goal**: Add convenience constructors from spec §8 for correct field population and UUID generation.

**Changes**:

1. **`pkg/types/constructors.go`** (NEW):
   - `NewAssistantMessage(msg BetaMessage, parentToolUseID *string, sessionID string) *AssistantMessage` (spec §8)
   - `NewUserMessage(content string, sessionID string) *UserMessage` (spec §8)
   - `NewResultSuccess(...)  *ResultMessage` (spec §8)
   - `NewResultError(...) *ResultMessage` (spec §8)
   - `NewSystemInit(config QueryOptions, version string, sessionID string) *SystemInitMessage` (spec §8)
   - `NewCompactBoundary(trigger string, preTokens int, sessionID string) *CompactBoundaryMessage` (spec §8)

2. **`pkg/types/constructors_test.go`** (NEW):
   - Verify each constructor produces valid UUIDs
   - Verify mandatory fields are populated
   - Verify `GetType()` returns correct `MessageType`

**Success Criteria**:
- Automated:
  - [x] `go test ./pkg/types/... -race -run TestNew` — constructor tests pass
  - [x] `go vet ./...` clean

### Phase 5: Configuration & Control Protocol Types
**Goal**: Define `QueryOptions`, control protocol, and all supporting configuration types from spec §6-7.

**Changes**:

1. **`pkg/types/options.go`** (NEW):
   - `QueryOptions` struct with all ~35 fields (spec §7.1)
   - `PermissionMode` enum (6 values) (spec §7.2)
   - `SettingSource` enum (3 values) (spec §7.2)
   - `OutputFormat` (spec §7.2)
   - `SystemPromptConfig` + custom `MarshalJSON` (spec §7.2)
   - `ExitReason` enum (5 values) (spec §7.2)
   - `ModelInfo` (spec §7.2)
   - `HookEvent` enum (15 values) (spec §7.2)
   - `HookCallbackMatcher` (spec §7.2)
   - `CanUseToolFunc`, `PermissionResult`, `PermissionUpdate`, `PermissionRuleValue` (spec §7.2)
   - `AgentDefinition` (spec §7.5)

2. **`pkg/types/mcp.go`** (NEW):
   - `McpServerConfig` (spec §7.3)
   - `PluginConfig` (spec §7.3)
   - `SandboxSettings`, `SandboxNetworkConfig` (spec §7.4)

3. **`pkg/types/control.go`** (NEW):
   - `ControlRequest`, `ControlRequestInner` (spec §6.1)
   - `ControlResponse`, `ControlSuccessResponse`, `ControlErrorResponse` (spec §6.2)
   - `ControlCancelRequest` (spec §6.3)
   - Control subtype constants (13 values) (spec §6.1)

4. **`pkg/types/options_test.go`** (NEW):
   - `SystemPromptConfig.MarshalJSON()` tests (raw string vs preset object)
   - `QueryOptions` JSON round-trip smoke test

**Success Criteria**:
- Automated:
  - [x] `go build ./...` compiles
  - [x] `go test ./pkg/types/... -race` — all tests pass
  - [x] `go vet ./...` clean
- Manual:
  - [ ] `SystemPromptConfig` marshals as plain string for Raw, as `{"type":"preset",...}` for Preset

### Phase 6: Verification & Cleanup
**Goal**: Run the full verification checklist from spec §9 and clean up.

**Changes**:

1. **`pkg/types/verify_test.go`** (NEW) — Compile-time interface conformance checks:
   ```go
   var _ SDKMessage = (*AssistantMessage)(nil)
   var _ SDKMessage = (*UserMessage)(nil)
   // ... all 16 types
   ```

2. **Review all spec §9 checklist items** — fix any gaps found:
   - Type completeness (16 variants)
   - JSON round-trip fidelity (all types)
   - ContentBlock clean marshaling
   - Discriminator completeness
   - UserMessageReplay detection
   - Options completeness
   - Enum coverage
   - Control protocol coverage
   - Null handling (pointer vs zero-value)
   - UUID format
   - BetaUsage invariant
   - ModelUsage camelCase
   - Constructor correctness

3. **`pkg/llm/types_internal.go`** — Clean up: should now only contain `CompletionResponse`

**Success Criteria**:
- Automated:
  - [x] `go build ./...` compiles
  - [x] `go test ./... -race` — ALL tests pass
  - [x] `go vet ./...` clean
- Manual:
  - [ ] All 15 items in spec §9 verification checklist confirmed

## Out of Scope
- Transport layer implementation (Spec 12) — uses these types but is a separate concern
- Session storage / JSONL persistence (Spec 11) — will consume `UnmarshalSDKMessage`
- Agentic loop (Spec 03) — will construct and emit these message types
- Tool definitions / schemas (Spec 04) — referenced in `QueryOptions.Tools` as `any`
- Hook system implementation (Spec 07) — hook message types are defined here, execution logic is separate

## Open Questions
None — all resolved through spec review and user decisions.

## References
- Spec: `thoughts/specs/02-MESSAGE-TYPES.md`
- Existing types: `pkg/llm/types_internal.go`, `pkg/types/messages.go`
- Existing emission: `pkg/llm/emit.go`
- LLM client spec: `thoughts/specs/01-LLM-CLIENT.md`
- Prior plan: `thoughts/plans/2026-02-08-llm-client.md`

## File Inventory (New & Modified)

### New files (pkg/types/):
| File | Contents |
|------|----------|
| `content.go` | `ContentBlock` + `MarshalJSON`, `BetaUsage`, `BetaMessage` |
| `message_user.go` | `UserMessage`, `UserMessageReplay`, `MessageParam` |
| `message_result.go` | `ResultMessage`, `ModelUsage`, `PermissionDenial` |
| `message_system.go` | `SystemInitMessage`, `StatusMessage`, `CompactBoundaryMessage` |
| `message_hooks.go` | `HookStartedMessage`, `HookProgressMessage`, `HookResponseMessage` |
| `message_misc.go` | `ToolProgressMessage`, `AuthStatusMessage`, `TaskNotificationMessage`, `FilesPersistedEvent`, `ToolUseSummaryMessage` |
| `unmarshal.go` | `RawSDKMessage`, `UnmarshalSDKMessage`, `unmarshalSystemMessage` |
| `constructors.go` | `New*` constructor functions |
| `options.go` | `QueryOptions`, enums, config types |
| `mcp.go` | `McpServerConfig`, `PluginConfig`, `SandboxSettings` |
| `control.go` | `ControlRequest`, `ControlResponse`, control subtypes |
| `unmarshal_test.go` | JSON round-trip tests |
| `content_test.go` | ContentBlock marshaling tests |
| `constructors_test.go` | Constructor tests |
| `options_test.go` | SystemPromptConfig + QueryOptions tests |
| `verify_test.go` | Interface conformance compile checks |

### Modified files (pkg/llm/):
| File | Change |
|------|--------|
| `types_internal.go` | Remove moved types, keep `CompletionResponse` with `types.` imports |
| `translate.go` | Add `types` import, update return types |
| `stream.go` | Add `types` import, update type references |
| `cost.go` | Add `types` import, update parameter types |
| `request.go` | Add `types` import, update parameter type |
| `emit.go` | Update to use `BaseMessage`, typed `Message` field |
| `translate_test.go` | Add `types` import, prefix type refs |
| `emit_test.go` | Add `types` import, prefix type refs |
| `golden_test.go` | Add `types` import, prefix type refs |
| `cost_test.go` | Add `types` import, prefix type refs |
| `request_test.go` | Add `types` import, prefix type refs |
