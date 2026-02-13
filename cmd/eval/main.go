// Headless eval binary for running Goat's agentic loop in benchmark sandboxes.
//
// The binary reads a prompt, runs the full agentic loop with 6 core tools,
// and writes the final assistant text to stdout.
//
// Environment variables:
//
//	OPENAI_BASE_URL  - LLM endpoint (default http://localhost:13131/v1)
//	OPENAI_API_KEY   - API key (default "inspect")
//	EVAL_MODEL       - Model ID (default "inspect")
//
// Flags:
//
//	-prompt   Prompt text (if empty, reads all of stdin)
//	-cwd      Working directory for tools (default: current directory)
//	-max-turns Maximum agentic loop turns (default: 100)
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/prompt"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

func main() {
	promptFlag := flag.String("prompt", "", "Prompt text (reads stdin if empty)")
	cwdFlag := flag.String("cwd", "", "Working directory for tools (default: current directory)")
	maxTurns := flag.Int("max-turns", 100, "Maximum agentic loop turns")
	flag.Parse()

	// Resolve prompt: flag > stdin
	promptText := *promptFlag
	if promptText == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		promptText = string(data)
	}
	if promptText == "" {
		fmt.Fprintln(os.Stderr, "error: no prompt provided (use -prompt flag or pipe to stdin)")
		os.Exit(1)
	}

	// Resolve CWD
	cwd := *cwdFlag
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Resolve LLM config from env vars
	baseURL := envOr("OPENAI_BASE_URL", "http://localhost:13131/v1")
	apiKey := envOr("OPENAI_API_KEY", "inspect")
	model := envOr("EVAL_MODEL", "inspect")

	// Create LLM client
	client := llm.NewClient(llm.ClientConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
	})

	// Build tool registry with 6 core tools
	registry := buildToolRegistry(cwd)

	// Build agent config with full prompt assembly
	config := agent.DefaultConfig()
	config.LLMClient = client
	config.Model = model
	config.MaxTurns = *maxTurns
	config.CWD = cwd
	config.OS = runtime.GOOS
	config.CurrentDate = time.Now().Format("2006-01-02")
	config.ToolRegistry = registry
	config.Permissions = &agent.AllowAllChecker{}
	config.Hooks = &agent.NoOpHookRunner{}
	config.Compactor = &agent.NoOpCompactor{}

	// Groq/Llama-specific tuning: use a concise system prompt and compact tool
	// descriptions to reduce "Failed to call a function" errors.
	// See: thoughts/tickets/BUG-groq-tool-calling-failures.md
	if llm.IsGroqLlama(model) {
		config.Prompter = &agent.StaticPromptAssembler{Prompt: groqSystemPrompt}
		config.CompactTools = true
	} else {
		config.Prompter = &prompt.Assembler{}
	}

	// Run with Ctrl+C support
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	query := agent.RunLoop(ctx, promptText, config)

	// Consume messages, capture final assistant text
	var lastText string
	for msg := range query.Messages() {
		switch m := msg.(type) {
		case types.AssistantMessage:
			lastText = extractText(m)
		case *types.AssistantMessage:
			lastText = extractText(*m)
		}
	}
	query.Wait()

	if lastText != "" {
		fmt.Print(lastText)
	}
}

// extractText pulls the text content from an AssistantMessage.
func extractText(m types.AssistantMessage) string {
	for _, block := range m.Message.Content {
		if block.Type == "text" && block.Text != "" {
			return block.Text
		}
	}
	return ""
}

// buildToolRegistry creates a registry with the 6 core eval tools.
func buildToolRegistry(cwd string) *tools.Registry {
	tm := tools.NewTaskManager()
	registry := tools.NewRegistry(
		tools.WithAllowed("Read", "Glob", "Grep"),
	)
	registry.Register(&tools.BashTool{CWD: cwd, TaskManager: tm})
	registry.Register(&tools.FileReadTool{})
	registry.Register(&tools.FileWriteTool{})
	registry.Register(&tools.FileEditTool{})
	registry.Register(&tools.GlobTool{CWD: cwd})
	registry.Register(&tools.GrepTool{CWD: cwd})
	return registry
}

// envOr returns the value of an environment variable, or the fallback if unset.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// groqSystemPrompt is a concise system prompt (~400 tokens) optimized for Groq-hosted
// Llama models. Groq recommends keeping system prompts to 300-600 tokens for best results.
// This replaces the full Claude Code-style prompt (~3000+ tokens) that overwhelms Llama's
// instruction-following capacity during tool calling.
const groqSystemPrompt = `You are a coding assistant with access to tools for running commands and editing files.

# Rules
- Use the provided tools to complete tasks. Call tools with correct JSON arguments.
- Use dedicated tools for file operations: Read for reading, Write for creating, Edit for modifying, Glob for finding files, Grep for searching content.
- Use Bash only for system commands (git, npm, docker, etc.), NOT for file operations.
- You may call multiple tools in a single response when they are independent.
- When a tool call fails, adjust your approach rather than retrying the exact same call.
- Read a file before editing it.
- Prefer editing existing files over creating new ones.
- Be concise in your responses. Focus on completing the task.`
