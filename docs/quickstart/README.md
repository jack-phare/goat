# Goat Quick Start

Goat is a Go library that implements the Claude Code agentic loop. It's not a standalone CLI — you embed it in your own Go program.

This guide shows you how to:
1. Run the test suite to verify everything works
2. Write a minimal program that calls an LLM through the agentic loop
3. Connect to different LLM providers (Groq, OpenAI, Anthropic, LiteLLM)
4. Run a local LiteLLM proxy for development

## Prerequisites

- Go 1.24+ installed (`go version`)
- An API key for at least one LLM provider
- A `.env` file with your keys (the example auto-loads it)

## 1. Run the Test Suite

The fastest way to verify the project works — no API keys needed:

```bash
cd /path/to/goat

# Run all ~800 tests (no network calls, fully self-contained)
go test ./...

# Run with race detector
go test -race ./...

# Run a specific package
go test ./pkg/llm/...
go test ./pkg/agent/...
go test ./pkg/tools/...

# Run with verbose output
go test -v ./pkg/agent/... -run TestRunLoop
```

All tests use mocked HTTP servers and pre-recorded SSE streams. No external API calls are made.

## 2. Run the Example

A runnable example is included at `cmd/example/main.go`. It auto-loads `.env` from the project root.

```bash
# Groq (fast, free tier available)
go run ./cmd/example/ -provider groq

# OpenAI
go run ./cmd/example/ -provider openai

# Anthropic
go run ./cmd/example/ -provider anthropic

# LiteLLM proxy (for Azure, multi-provider, etc.)
go run ./cmd/example/ -provider litellm

# Custom prompt
go run ./cmd/example/ -provider groq -prompt "List files in this directory"

# With tool use (default) vs pure chat
go run ./cmd/example/ -provider openai -prompt "What files are here?" -max-turns 3
go run ./cmd/example/ -provider openai -no-tools -prompt "Tell me a joke"

# Show all flags
go run ./cmd/example/ -help
```

### .env format

```
GROQ_API_KEY=gsk_...
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...

# LiteLLM (any one of these — checked in order)
EXECUTOR_LITELLM_KEY=sk-...
LITELLM_MASTER_KEY=sk-dev-key
LITELLM_API_KEY=sk-...
```

The example auto-detects which provider to use based on available env vars (priority: groq > openai > anthropic > litellm). For LiteLLM, it checks three env vars in order: `EXECUTOR_LITELLM_KEY` (production) > `LITELLM_MASTER_KEY` (dev) > `LITELLM_API_KEY` (generic).

### Testing all providers

Copy-paste commands to test each provider via direct access and via LiteLLM proxy:

```bash
# === Direct providers ===

# Groq direct
go run ./cmd/example/ -provider groq -no-tools -prompt "What is 2+2?"
go run ./cmd/example/ -provider groq -prompt "List files here" -max-turns 3

# OpenAI direct
go run ./cmd/example/ -provider openai -no-tools -prompt "What is 2+2?"
go run ./cmd/example/ -provider openai -prompt "List files here" -max-turns 3

# Anthropic direct
go run ./cmd/example/ -provider anthropic -no-tools -prompt "What is 2+2?"
go run ./cmd/example/ -provider anthropic -prompt "List files here" -max-turns 3

# === Via LiteLLM proxy ===
# (requires proxy running at localhost:4000)

# gpt-5-nano (default litellm model)
go run ./cmd/example/ -provider litellm -no-tools -prompt "What is 2+2?"
go run ./cmd/example/ -provider litellm -prompt "List files here" -max-turns 3

# gpt-5-mini
go run ./cmd/example/ -provider litellm -model gpt-5-mini -no-tools -prompt "What is 2+2?"
go run ./cmd/example/ -provider litellm -model gpt-5-mini -prompt "Read go.mod" -max-turns 3

# gpt-4o-mini
go run ./cmd/example/ -provider litellm -model gpt-4o-mini -no-tools -prompt "What is 2+2?"
go run ./cmd/example/ -provider litellm -model gpt-4o-mini -prompt "List files here" -max-turns 3

# llama-3.3-70b (via Groq through proxy)
go run ./cmd/example/ -provider litellm -model llama-3.3-70b -no-tools -prompt "What is 2+2?"
go run ./cmd/example/ -provider litellm -model llama-3.3-70b -prompt "List files here" -max-turns 3
```

## 3. Minimal Code Example

Here's the core pattern for embedding the agentic loop in your own program:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/types"
)

func main() {
	// 1. Create LLM client
	client := llm.NewClient(llm.ClientConfig{
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  os.Getenv("GROQ_API_KEY"),
		Model:   "llama-3.3-70b-versatile",
	})

	// 2. Create tool registry (all 21 built-in tools)
	cwd, _ := os.Getwd()
	registry := agent.DefaultRegistry(cwd, nil) // nil = no MCP servers

	// 3. Build agent config
	config := agent.DefaultConfig()
	config.LLMClient = client
	config.Model = "llama-3.3-70b-versatile" // must match LLM client model
	config.ToolRegistry = registry
	config.CWD = cwd
	config.MaxTurns = 5

	// 4. Run the loop
	ctx := context.Background()
	query := agent.RunLoop(ctx, "What is 2 + 2?", config)

	// 5. Consume messages
	for msg := range query.Messages() {
		switch m := msg.(type) {
		case types.AssistantMessage:
			for _, block := range m.Message.Content {
				if block.Type == "text" {
					fmt.Println(block.Text)
				}
			}
		case *types.AssistantMessage:
			for _, block := range m.Message.Content {
				if block.Type == "text" {
					fmt.Println(block.Text)
				}
			}
		}
	}

	query.Wait()
	fmt.Printf("Turns: %d, Cost: $%.6f\n", query.TurnCount(), query.TotalCostUSD())
}
```

**Important:** Set `config.Model` to match the LLM client model. The `DefaultConfig()` defaults to a Claude model, which won't work with non-Anthropic providers.

## 4. Provider Configuration

Goat's LLM client speaks the OpenAI-compatible `/v1/chat/completions` SSE protocol. It works with any provider that implements this API.

### Groq

```go
client := llm.NewClient(llm.ClientConfig{
    BaseURL: "https://api.groq.com/openai/v1",
    APIKey:  os.Getenv("GROQ_API_KEY"),
    Model:   "llama-3.3-70b-versatile",
})
```

### OpenAI

```go
client := llm.NewClient(llm.ClientConfig{
    BaseURL: "https://api.openai.com/v1",
    APIKey:  os.Getenv("OPENAI_API_KEY"),
    Model:   "gpt-4o-mini",
})
```

### Anthropic (direct)

```go
client := llm.NewClient(llm.ClientConfig{
    BaseURL: "https://api.anthropic.com/v1",
    APIKey:  os.Getenv("ANTHROPIC_API_KEY"),
    Model:   "claude-sonnet-4-5-20250929",
})
```

### LiteLLM Proxy (recommended for Azure, multi-provider)

If you run a [LiteLLM](https://docs.litellm.ai/) proxy, all providers are unified behind one endpoint:

```go
client := llm.NewClient(llm.ClientConfig{
    BaseURL: "http://localhost:4000/v1",
    APIKey:  os.Getenv("EXECUTOR_LITELLM_KEY"),
    Model:   "gpt-5-nano", // use model names as configured in your LiteLLM proxy
})
```

Model IDs must match what your LiteLLM proxy has configured. Check available models with:
```bash
curl http://localhost:4000/v1/models -H "Authorization: Bearer $EXECUTOR_LITELLM_KEY"
```

Available models on the production (agent-hub) proxy:
- `gpt-4o-mini` — Azure OpenAI
- `gpt-5-nano` — Azure OpenAI (default for litellm provider)
- `gpt-5-mini` — Azure OpenAI
- `llama-3.3-70b` — Groq
- `text-embedding-3-small` — OpenAI (embeddings only)

**Note:** Azure OpenAI uses a different URL and auth pattern. Use it via LiteLLM rather than direct access.

## 5. Dev LiteLLM Proxy (Optional)

A self-contained LiteLLM proxy is included for contributors who don't have a production proxy running. It mirrors the agent-hub production config (Azure OpenAI + Groq) so the same env vars work locally.

```bash
# 1. Copy env template and fill in your keys
cp dev/.env.example dev/.env
# Edit dev/.env — at minimum set AZURE_API_KEY + AZURE_API_BASE, or GROQ_API_KEY

# 2. Start the proxy (LiteLLM + Postgres)
docker compose -f dev/docker-compose.yml up -d

# 3. Verify it's running
curl http://localhost:4000/v1/models -H "Authorization: Bearer sk-dev-key"

# 4. Run the example against the dev proxy
LITELLM_MASTER_KEY=sk-dev-key go run ./cmd/example/ -provider litellm -prompt "What is 2+2?"

# 5. Tear down when done
docker compose -f dev/docker-compose.yml down
```

The dev proxy includes:
- **Azure OpenAI** models: `gpt-4o-mini`, `gpt-5-nano`, `gpt-5-mini`
- **Groq**: `llama-3.3-70b`
- **Langfuse** observability (optional — set keys in `.env` to enable)
- **Postgres** for model storage and budget tracking

The example's `-provider litellm` checks env vars in order: `EXECUTOR_LITELLM_KEY` (production), `LITELLM_MASTER_KEY` (dev), `LITELLM_API_KEY` (generic).

## 6. Multi-Turn Conversation

For interactive chat sessions, enable multi-turn mode:

```go
config := agent.DefaultConfig()
config.LLMClient = client
config.Model = "llama-3.3-70b-versatile"
config.ToolRegistry = registry
config.MultiTurn = true // loop waits for input after each response

query := agent.RunLoop(ctx, "Hello! What can you help with?", config)

// Read messages in a goroutine
go func() {
    for msg := range query.Messages() {
        switch m := msg.(type) {
        case types.AssistantMessage:
            fmt.Println(extractText(m))
        case *types.AssistantMessage:
            fmt.Println(extractText(*m))
        }
    }
}()

// Send follow-up messages
query.SendUserMessage([]byte("Now list the files in this directory"))
query.SendUserMessage([]byte("Read the go.mod file"))

// When done
query.Close()
query.Wait()
```

## 7. Key Concepts

### SDKMessage Types

The `query.Messages()` channel emits these message types:

| Type | When |
|------|------|
| `types.SystemInitMessage` | Once at start, contains session config |
| `types.AssistantMessage` | Each LLM response (may contain text + tool calls) |
| `types.ToolProgressMessage` | During tool execution |
| `types.ResultMessage` | Once at end, contains exit reason + stats |
| `types.StreamEvent` | Per-SSE-chunk (only if `IncludePartial=true`) |

### Exit Reasons

| Reason | Meaning |
|--------|---------|
| `end_turn` | Model finished naturally |
| `max_turns` | Hit `MaxTurns` limit |
| `max_budget` | Hit `MaxBudgetUSD` limit |
| `max_tokens` | Context window full |
| `interrupted` | `query.Interrupt()` called |

### Permission Modes

By default, `AllowAllChecker` permits all tools. For production use, wire in the permission system:

```go
import "github.com/jg-phare/goat/pkg/permission"

checker := permission.NewChecker(permission.CheckerConfig{
    Mode: types.PermissionModeDefault,
    // ... rules, prompter, etc.
})
config.Permissions = checker
```

### Tool Registry

The `DefaultRegistry` includes all 21 built-in tools (Bash, Read, Write, Edit, Glob, Grep, etc.). You can also create a custom registry:

```go
registry := tools.NewRegistry(
    tools.WithAllowed("Read", "Glob", "Grep"), // auto-allowed (no permission check)
    tools.WithDisabled("Bash"),                 // explicitly blocked
)
registry.Register(&tools.FileReadTool{})
registry.Register(&tools.GlobTool{CWD: cwd})
registry.Register(&tools.GrepTool{CWD: cwd})
```

## 8. Running Tests for a Specific Package

```bash
# LLM client (SSE parsing, accumulation, cost tracking)
go test -v ./pkg/llm/...

# Agentic loop (state machine, tool execution, multi-turn)
go test -v ./pkg/agent/...

# Tool implementations (Bash, file ops, web fetch, etc.)
go test -v ./pkg/tools/...

# All packages with race detection
go test -race ./...

# Golden test regeneration (if you modify SSE parsing)
go test ./pkg/llm/... -run TestGolden -update
```

## 9. Project Structure

```
pkg/
├── agent/      Core agentic loop, Query type, config, stubs
├── llm/        LLM client, SSE parser, stream accumulator, cost tracker
├── types/      SDKMessage types, content blocks, control protocol
├── tools/      Tool interface + 21 implementations + task manager
├── prompt/     System prompt assembly from 133 embedded prompt files
├── permission/ Permission checker with rules, modes, and hooks
├── hooks/      Hook system (shell + Go callbacks on lifecycle events)
├── context/    Context compaction (LLM-powered summarization)
├── session/    Session persistence (JSONL + checkpoints)
├── subagent/   Subagent spawning and lifecycle management
├── teams/      Multi-agent team coordination (feature-gated)
├── mcp/        Model Context Protocol client (stdio + HTTP transports)
└── transport/  Transport layer (channel, stdio, WebSocket, SSE)
```
