package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/prompt"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

// TestSkillE2E_LLMInvokesSkill runs the full agent loop with a real LLM and
// verifies that the Skill tool is actually called. Uses a "secret-word" skill
// that tells the LLM to include "BANANA-TRUMPET-42" in its response -- this
// phrase only exists in the skill body, so its presence proves the skill was
// invoked and its content was used.
//
// Requires OPENAI_API_KEY (or ANTHROPIC_API_KEY with OPENAI_BASE_URL pointed
// at an Anthropic-compatible endpoint). Set E2E_MODEL to override the model.
//
// Skip with: go test -short ./cmd/eval/
func TestSkillE2E_LLMInvokesSkill(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode (requires live LLM)")
	}

	// Resolve LLM credentials from environment.
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping e2e test")
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := os.Getenv("E2E_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	// Load the secret-word skill from testdata.
	root := projectRoot(t)
	skillsDir := filepath.Join(root, "cmd", "eval", "testdata")
	loader := prompt.NewSkillLoader(skillsDir, "")
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("no skills loaded from testdata")
	}
	t.Logf("Loaded %d skill(s): %v", len(skills), skillNamesFromMap(skills))

	// Build registry + adapter (same wiring as eval binary).
	skillRegistry := prompt.NewSkillRegistry()
	for _, entry := range skills {
		skillRegistry.Register(entry)
	}
	adapter := &tools.SkillProviderAdapter{Inner: skillRegistry}

	// Build tool registry: only Skill tool (minimize cost, no side effects).
	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(&tools.SkillTool{
		Skills:         adapter,
		ArgSubstituter: prompt.SubstituteArgs,
	})

	// Build LLM client.
	client := llm.NewClient(llm.ClientConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
	})

	// Build agent config.
	config := agent.DefaultConfig()
	config.LLMClient = client
	config.Model = model
	config.MaxTurns = 5
	config.CWD = t.TempDir()
	config.OS = "linux"
	config.CurrentDate = time.Now().Format("2006-01-02")
	config.ToolRegistry = toolRegistry
	config.Permissions = &agent.AllowAllChecker{}
	config.Hooks = &agent.NoOpHookRunner{}
	config.Compactor = &agent.NoOpCompactor{}
	config.Skills = skillRegistry
	config.Prompter = &prompt.Assembler{}

	// Run the agent loop with a prompt that should trigger skill invocation.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	promptText := "What is the secret word? Use the secret-word skill to find out."
	t.Logf("Prompt: %s", promptText)
	t.Logf("Model: %s", model)
	t.Logf("LLM endpoint: %s", baseURL)

	query := agent.RunLoop(ctx, promptText, config)

	// Collect all messages and look for Skill tool invocations.
	var (
		lastText       string
		skillToolCalls []string
		allMessages    []string
	)

	processContent := func(content []types.ContentBlock) {
		for _, block := range content {
			if block.Type == "text" && block.Text != "" {
				lastText = block.Text
				truncated := block.Text
				if len(truncated) > 100 {
					truncated = truncated[:100]
				}
				allMessages = append(allMessages, "[text] "+truncated)
			}
			if block.Type == "tool_use" {
				allMessages = append(allMessages, "[tool_use] "+block.Name)
				if block.Name == "Skill" {
					skillInput, _ := block.Input["skill"].(string)
					skillToolCalls = append(skillToolCalls, skillInput)
					t.Logf("Skill tool invoked with: %v", block.Input)
				}
			}
		}
	}

	for msg := range query.Messages() {
		msgType := msg.GetType()
		switch m := msg.(type) {
		case types.AssistantMessage:
			processContent(m.Message.Content)
		case *types.AssistantMessage:
			processContent(m.Message.Content)
		case types.ResultMessage:
			t.Logf("ResultMessage: subtype=%s result=%q errors=%v",
				m.Subtype, truncate(m.Result, 200), m.Errors)
		case *types.ResultMessage:
			t.Logf("ResultMessage: subtype=%s result=%q errors=%v",
				m.Subtype, truncate(m.Result, 200), m.Errors)
		default:
			allMessages = append(allMessages, "["+string(msgType)+"]")
		}
	}
	query.Wait()

	t.Logf("All messages: %v", allMessages)
	t.Logf("Final output: %s", truncate(lastText, 500))

	// Assertion 1: The Skill tool was invoked.
	if len(skillToolCalls) == 0 {
		t.Error("FAIL: Skill tool was never invoked by the LLM")
		t.Logf("  All messages: %v", allMessages)
	} else {
		t.Logf("PASS: Skill tool invoked %d time(s): %v", len(skillToolCalls), skillToolCalls)
	}

	// Assertion 2: The secret word from the skill body appears in the final output.
	if !strings.Contains(lastText, "BANANA-TRUMPET-42") {
		t.Error("FAIL: Secret word 'BANANA-TRUMPET-42' not found in final output")
		t.Logf("  Final output was: %s", lastText)
	} else {
		t.Log("PASS: Secret word 'BANANA-TRUMPET-42' found in final output")
	}
}

// skillNamesFromMap returns the keys of a skill map for debug output.
func skillNamesFromMap(m map[string]types.SkillEntry) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	return names
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
