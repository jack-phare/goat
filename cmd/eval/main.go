// Headless eval binary for running Goat's agentic loop in benchmark sandboxes.
//
// The binary reads a prompt, runs the full agentic loop with core tools,
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
//	-prompt      Prompt text (if empty, reads all of stdin)
//	-cwd         Working directory for tools (default: current directory)
//	-max-turns   Maximum agentic loop turns (default: 100)
//	-skills-dir  Directory containing skill subdirs with SKILL.md files (optional)
//	-mcp-config  Path to JSON file with MCP server configurations (optional)
//	-multi-turn  Enable multi-turn REPL mode (read follow-up prompts from stdin)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/mcp"
	"github.com/jg-phare/goat/pkg/prompt"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

func main() {
	promptFlag := flag.String("prompt", "", "Prompt text (reads stdin if empty)")
	cwdFlag := flag.String("cwd", "", "Working directory for tools (default: current directory)")
	maxTurns := flag.Int("max-turns", 100, "Maximum agentic loop turns")
	skillsDir := flag.String("skills-dir", "", "Directory containing skill subdirs with SKILL.md files (enables skill-augmented eval)")
	mcpConfig := flag.String("mcp-config", "", "Path to JSON file with MCP server configurations")
	multiTurn := flag.Bool("multi-turn", false, "Enable multi-turn REPL mode (read follow-up prompts from stdin)")
	flag.Parse()

	// Resolve prompt: flag > stdin
	// In multi-turn mode, only read one line from stdin (keep it open for follow-ups).
	var stdinScanner *bufio.Scanner
	promptText := *promptFlag
	if promptText == "" {
		if *multiTurn {
			stdinScanner = bufio.NewScanner(os.Stdin)
			if stdinScanner.Scan() {
				promptText = stdinScanner.Text()
			}
		} else {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
				os.Exit(1)
			}
			promptText = string(data)
		}
	}
	if promptText == "" {
		fmt.Fprintln(os.Stderr, "error: no prompt provided (use -prompt flag or pipe to stdin)")
		os.Exit(1)
	}
	// In multi-turn mode with -prompt flag, create scanner for follow-up reading.
	if *multiTurn && stdinScanner == nil {
		stdinScanner = bufio.NewScanner(os.Stdin)
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

	// Build tool registry with core tools
	registry := buildToolRegistry(cwd)

	// Set up Ctrl+C support early (needed for MCP connect timeouts).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Connect MCP servers if -mcp-config is provided.
	var mcpServers map[string]types.McpServerConfig
	if *mcpConfig != "" {
		var err error
		mcpServers, err = loadMCPConfig(*mcpConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading MCP config: %v\n", err)
			os.Exit(1)
		}
		mcpClient := mcp.NewClient(registry)
		defer mcpClient.Close()
		for name, cfg := range mcpServers {
			if err := mcpClient.Connect(ctx, name, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to connect MCP server %q: %v\n", name, err)
			} else {
				fmt.Fprintf(os.Stderr, "connected MCP server: %s\n", name)
			}
		}
		registry.Register(&tools.ListMcpResourcesTool{Client: mcpClient})
		registry.Register(&tools.ReadMcpResourceTool{Client: mcpClient})
	}

	// Load skills if -skills-dir is provided (for skill-augmented benchmarks).
	// The loader scans {skillsDir}/.claude/skills/{name}/SKILL.md following
	// the standard skill directory convention.
	var skillRegistry *prompt.SkillRegistry
	if *skillsDir != "" {
		loader := prompt.NewSkillLoader(*skillsDir, "")
		skills, err := loader.LoadAll()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading skills from %s: %v\n", *skillsDir, err)
			os.Exit(1)
		}
		if len(skills) == 0 {
			fmt.Fprintf(os.Stderr, "warning: no skills found in %s/.claude/skills/\n", *skillsDir)
		} else {
			fmt.Fprintf(os.Stderr, "loaded %d skill(s) from %s\n", len(skills), *skillsDir)
		}
		skillRegistry = prompt.NewSkillRegistry()
		for _, entry := range skills {
			skillRegistry.Register(entry)
		}
		adapter := &tools.SkillProviderAdapter{Inner: skillRegistry}
		registry.Register(&tools.SkillTool{
			Skills:         adapter,
			ArgSubstituter: prompt.SubstituteArgs,
		})
	}

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
	if skillRegistry != nil {
		config.Skills = skillRegistry
	}
	if mcpServers != nil {
		config.MCPServers = mcpServers
	}
	if *multiTurn {
		config.MultiTurn = true
	}

	// Groq/Llama-specific tuning: use a concise system prompt and compact tool
	// descriptions to reduce "Failed to call a function" errors.
	// See: thoughts/tickets/BUG-groq-tool-calling-failures.md
	if llm.IsGroqLlama(model) {
		config.Prompter = &agent.StaticPromptAssembler{Prompt: groqSystemPrompt}
		config.CompactTools = true
	} else {
		config.Prompter = &prompt.Assembler{}
	}

	query := agent.RunLoop(ctx, promptText, config)

	if *multiTurn {
		runMultiTurn(query, stdinScanner)
	} else {
		runSingleShot(query)
	}
}

// runSingleShot consumes all messages and prints the final assistant text (existing behavior).
func runSingleShot(query *agent.Query) {
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

// runMultiTurn runs a REPL loop: after each turn completes, it reads the next
// line from stdinScanner and sends it as a follow-up message. EOF or empty line
// terminates the session.
func runMultiTurn(query *agent.Query, stdinScanner *bufio.Scanner) {
	turnDone := make(chan struct{}, 1)
	loopDone := make(chan struct{})
	var lastText string

	// Message consumer goroutine
	go func() {
		defer close(loopDone)
		for msg := range query.Messages() {
			switch m := msg.(type) {
			case types.AssistantMessage:
				lastText = extractText(m)
			case *types.AssistantMessage:
				lastText = extractText(*m)
			case types.ResultMessage:
				if m.Subtype == types.ResultSubtypeSuccessTurn {
					if lastText != "" {
						fmt.Println(lastText)
						lastText = ""
					}
					printTurnMeta(m)
					select {
					case turnDone <- struct{}{}:
					default:
					}
				}
			case *types.ResultMessage:
				if m.Subtype == types.ResultSubtypeSuccessTurn {
					if lastText != "" {
						fmt.Println(lastText)
						lastText = ""
					}
					printTurnMeta(*m)
					select {
					case turnDone <- struct{}{}:
					default:
					}
				}
			}
		}
	}()

	// REPL: wait for turn to complete, read next line, send follow-up.
	for {
		select {
		case <-turnDone:
			// Ready for next input
		case <-loopDone:
			goto exit
		}
		if !stdinScanner.Scan() {
			break // EOF
		}
		line := stdinScanner.Text()
		if line == "" {
			break // empty line = done
		}
		if err := query.SendUserMessage([]byte(line)); err != nil {
			fmt.Fprintf(os.Stderr, "error sending message: %v\n", err)
			break
		}
	}

exit:
	query.Close()
	query.Wait()

	// Print any remaining text from the final turn
	if lastText != "" {
		fmt.Print(lastText)
	}
}

// printTurnMeta writes machine-readable turn metadata to stderr.
func printTurnMeta(m types.ResultMessage) {
	fmt.Fprintf(os.Stderr, `{"turn":%d,"cost_usd":%.6f}`+"\n", m.NumTurns, m.TotalCostUSD)
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

// loadMCPConfig reads a JSON file containing MCP server configurations.
// The file must contain a non-empty JSON object mapping server names to configs.
func loadMCPConfig(path string) (map[string]types.McpServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading MCP config: %w", err)
	}
	var servers map[string]types.McpServerConfig
	if err := json.Unmarshal(data, &servers); err != nil {
		return nil, fmt.Errorf("parsing MCP config: %w", err)
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("MCP config is empty (no servers defined)")
	}
	return servers, nil
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
