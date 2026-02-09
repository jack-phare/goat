// Example program demonstrating the goat agentic loop.
//
// Usage:
//
//	# Source your .env file first
//	source .env
//
//	# Groq
//	go run ./cmd/example/ -provider groq -prompt "What is 2+2?"
//
//	# Anthropic (direct)
//	go run ./cmd/example/ -provider anthropic -prompt "What is 2+2?"
//
//	# OpenAI
//	go run ./cmd/example/ -provider openai -prompt "What is 2+2?"
//
//	# LiteLLM proxy
//	go run ./cmd/example/ -provider litellm -model "anthropic/claude-sonnet-4-5-20250929" -prompt "What is 2+2?"
//
//	# Custom base URL
//	go run ./cmd/example/ -base-url "http://localhost:8080/v1" -api-key "..." -model "my-model" -prompt "Hello"
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

func main() {
	// Flags
	provider := flag.String("provider", "", "LLM provider: groq, openai, anthropic, litellm (or use -base-url)")
	baseURL := flag.String("base-url", "", "Custom base URL (overrides -provider)")
	apiKey := flag.String("api-key", "", "API key (overrides env var)")
	model := flag.String("model", "", "Model ID (overrides provider default)")
	prompt := flag.String("prompt", "What is 2 + 2? Reply in one short sentence.", "Prompt to send")
	maxTurns := flag.Int("max-turns", 5, "Maximum agentic loop turns")
	noTools := flag.Bool("no-tools", false, "Run without tools (pure chat)")
	streaming := flag.Bool("stream", false, "Show streaming chunks")
	envFile := flag.String("env", ".env", "Path to .env file (empty to skip)")
	flag.Parse()

	// Load .env file
	if *envFile != "" {
		loadEnvFile(*envFile)
	}

	// Resolve provider config
	rc, err := resolveConfig(*provider, *baseURL, *apiKey, *model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		fmt.Fprintln(os.Stderr, "Usage: source .env && go run ./cmd/example/ -provider groq")
		fmt.Fprintln(os.Stderr, "Providers: groq, openai, anthropic, litellm (Azure via litellm)")
		os.Exit(1)
	}

	fmt.Printf("Provider: %s\n", rc.Provider)
	fmt.Printf("Base URL: %s\n", rc.BaseURL)
	fmt.Printf("Model:    %s\n", rc.Model)
	fmt.Printf("Prompt:   %s\n", *prompt)
	fmt.Println(strings.Repeat("-", 60))

	// Create LLM client
	client := llm.NewClient(rc.ClientConfig)

	// Build agent config
	config := agent.DefaultConfig()
	config.LLMClient = client
	config.Model = rc.Model // use the provider's model, not the default Claude model
	config.MaxTurns = *maxTurns
	config.IncludePartial = *streaming

	cwd, _ := os.Getwd()
	config.CWD = cwd

	// Use a concise system prompt — the DefaultConfig's StaticPromptAssembler
	// returns "You are a helpful assistant." which is fine for most models.
	// For tool-enabled runs, tell the model it has tools available.
	if !*noTools {
		config.ToolRegistry = buildToolRegistry(cwd)
		config.Prompter = &agent.StaticPromptAssembler{
			Prompt: "You are a helpful coding assistant. You have access to tools for running commands, reading/writing files, and searching. Use tools when the user asks you to interact with the filesystem or run commands. Answer directly when no tools are needed.",
		}
	}

	// Run with Ctrl+C support
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	query := agent.RunLoop(ctx, *prompt, config)

	// Consume messages
	for msg := range query.Messages() {
		switch m := msg.(type) {
		case types.AssistantMessage:
			printAssistant(m)
		case *types.AssistantMessage:
			printAssistant(*m)
		case types.ResultMessage:
			printResult(m, query)
		case *types.ResultMessage:
			printResult(*m, query)
		case types.SystemInitMessage:
			fmt.Printf("[init] session=%s\n", m.SessionID)
		case *types.SystemInitMessage:
			fmt.Printf("[init] session=%s\n", m.SessionID)
		default:
			if *streaming {
				fmt.Printf("[%s] ...\n", msg.GetType())
			}
		}
	}

	query.Wait()
}

func printAssistant(m types.AssistantMessage) {
	for _, block := range m.Message.Content {
		switch block.Type {
		case "text":
			fmt.Printf("\n%s\n", block.Text)
		case "tool_use":
			fmt.Printf("[tool_use] %s(%v)\n", block.Name, block.Input)
		case "thinking":
			fmt.Printf("[thinking] %s\n", truncate(block.Thinking, 100))
		}
	}
}

func printResult(m types.ResultMessage, q *agent.Query) {
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Exit: %s | Turns: %d | Cost: $%.6f\n",
		m.Subtype, q.TurnCount(), q.TotalCostUSD())
	usage := q.TotalUsage()
	fmt.Printf("Tokens: %d input, %d output\n",
		usage.InputTokens, usage.OutputTokens)
	if m.IsError && len(m.Errors) > 0 {
		fmt.Printf("Errors: %s\n", strings.Join(m.Errors, "; "))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// buildToolRegistry creates a slim registry with just the core tools.
// The full DefaultRegistry has 21 tools which overwhelms smaller models.
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

// loadEnvFile reads a .env file and sets environment variables (won't overwrite existing).
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // silently skip if no .env
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Strip surrounding quotes
		val = strings.Trim(val, `"'`)
		// Don't overwrite existing env vars
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

type providerConfig struct {
	baseURL    string
	baseURLEnv string   // env var for base URL override
	envKey     string   // primary env var for API key
	envKeys    []string // fallback env vars for API key
	model      string
}

var providers = map[string]providerConfig{
	"groq": {
		baseURL:    "https://api.groq.com/openai/v1",
		baseURLEnv: "GROQ_API_BASE",
		envKey:     "GROQ_API_KEY",
		model:      "llama-3.3-70b-versatile",
	},
	"anthropic": {
		baseURL: "https://api.anthropic.com/v1",
		envKey:  "ANTHROPIC_API_KEY",
		model:   "claude-sonnet-4-5-20250929",
	},
	"openai": {
		baseURL: "https://api.openai.com/v1",
		envKey:  "OPENAI_API_KEY",
		model:   "gpt-4o-mini",
	},
	// Azure OpenAI has a different URL pattern (/openai/deployments/{name}/...)
	// and uses api-key header. Use via LiteLLM proxy instead of direct access.
	// "azure": { ... },
	"litellm": {
		baseURL:    "http://localhost:4000/v1",
		baseURLEnv: "LITELLM_BASE_URL",
		envKey:     "EXECUTOR_LITELLM_KEY",
		envKeys:    []string{"LITELLM_MASTER_KEY", "LITELLM_API_KEY"},
		model:      "gpt-5-nano",
	},
}

// resolvedConfig wraps llm.ClientConfig with the detected provider name.
type resolvedConfig struct {
	llm.ClientConfig
	Provider string
}

func resolveConfig(provider, baseURL, apiKey, model string) (resolvedConfig, error) {
	rc := resolvedConfig{}

	// Custom base URL takes priority
	if baseURL != "" {
		rc.BaseURL = baseURL
		rc.APIKey = apiKey
		rc.Model = model
		rc.Provider = "custom"
		if rc.Model == "" {
			return rc, fmt.Errorf("-model is required when using -base-url")
		}
		return rc, nil
	}

	if provider == "" {
		// Auto-detect from env vars (prefer groq first)
		for _, name := range []string{"groq", "openai", "anthropic", "litellm"} {
			pc := providers[name]
			if key := lookupKey(pc); key != "" {
				provider = name
				break
			}
		}
		if provider == "" {
			return rc, fmt.Errorf("no provider specified and no API key found in environment.\n" +
				"Set one of: GROQ_API_KEY, OPENAI_API_KEY, ANTHROPIC_API_KEY, EXECUTOR_LITELLM_KEY")
		}
	}

	pc, ok := providers[provider]
	if !ok {
		return rc, fmt.Errorf("unknown provider %q (use: groq, openai, anthropic, litellm)", provider)
	}

	rc.Provider = provider
	rc.BaseURL = pc.baseURL
	// Allow base URL override from env
	if pc.baseURLEnv != "" {
		if envBase := os.Getenv(pc.baseURLEnv); envBase != "" {
			rc.BaseURL = envBase
		}
	}
	rc.Model = pc.model

	if apiKey != "" {
		rc.APIKey = apiKey
	} else {
		rc.APIKey = lookupKey(pc)
	}

	if rc.APIKey == "" {
		allKeys := append([]string{pc.envKey}, pc.envKeys...)
		return rc, fmt.Errorf("no API key: set one of %s or use -api-key", strings.Join(allKeys, ", "))
	}

	if model != "" {
		rc.Model = model
	}

	return rc, nil
}

// lookupKey checks the primary env key, then falls back to envKeys.
func lookupKey(pc providerConfig) string {
	if v := os.Getenv(pc.envKey); v != "" {
		return v
	}
	for _, k := range pc.envKeys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
