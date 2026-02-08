package prompt

import (
	"io/fs"
	"testing"
)

func TestEmbeddedPromptsLoad(t *testing.T) {
	tests := []struct {
		name  string
		fsys  fs.ReadFileFS
		dir   string
		count int // expected minimum file count
	}{
		{"system", systemPrompts, "prompts/system", 29},
		{"agents", agentPrompts, "prompts/agents", 29},
		{"reminders", reminderPrompts, "prompts/reminders", 37},
		{"tools", toolPrompts, "prompts/tools", 22},
		{"data", dataPrompts, "prompts/data", 3},
		{"skills", skillPrompts, "prompts/skills", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := fs.ReadDir(tt.fsys, tt.dir)
			if err != nil {
				t.Fatalf("failed to read dir %s: %v", tt.dir, err)
			}
			if len(entries) < tt.count {
				t.Errorf("expected at least %d files in %s, got %d", tt.count, tt.dir, len(entries))
			}
			for _, e := range entries {
				data, err := fs.ReadFile(tt.fsys, tt.dir+"/"+e.Name())
				if err != nil {
					t.Errorf("failed to read %s/%s: %v", tt.dir, e.Name(), err)
					continue
				}
				if len(data) == 0 {
					t.Errorf("file %s/%s is empty", tt.dir, e.Name())
				}
			}
		})
	}
}

func TestLoadPromptHelpers(t *testing.T) {
	tests := []struct {
		name     string
		loader   func(string) string
		file     string
		contains string
	}{
		{
			"loadSystemPrompt",
			loadSystemPrompt,
			"system-prompt-main-system-prompt.md",
			"interactive CLI tool",
		},
		{
			"loadAgentPrompt",
			loadAgentPrompt,
			"agent-prompt-explore.md",
			"",
		},
		{
			"loadReminderPrompt",
			loadReminderPrompt,
			"system-reminder-file-truncated.md",
			"",
		},
		{
			"loadToolPrompt",
			loadToolPrompt,
			"tool-description-bash.md",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := tt.loader(tt.file)
			if content == "" {
				t.Errorf("expected non-empty content for %s", tt.file)
			}
			if tt.contains != "" && !contains(content, tt.contains) {
				t.Errorf("expected content to contain %q", tt.contains)
			}
		})
	}
}

func TestLoadPromptCaching(t *testing.T) {
	// Load twice — second should hit cache.
	content1 := loadSystemPrompt("system-prompt-main-system-prompt.md")
	content2 := loadSystemPrompt("system-prompt-main-system-prompt.md")
	if content1 != content2 {
		t.Error("cached load returned different content")
	}
}

func TestLoadPromptMissing(t *testing.T) {
	content := loadSystemPrompt("nonexistent-file.md")
	if content != "" {
		t.Error("expected empty string for missing file")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
