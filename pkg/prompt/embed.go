package prompt

import (
	"embed"
	"io/fs"
	"sync"
)

//go:embed prompts/system/*.md
var systemPrompts embed.FS

//go:embed prompts/agents/*.md
var agentPrompts embed.FS

//go:embed prompts/reminders/*.md
var reminderPrompts embed.FS

//go:embed prompts/tools/*.md
var toolPrompts embed.FS

//go:embed prompts/data/*.md
var dataPrompts embed.FS

//go:embed prompts/skills/*.md
var skillPrompts embed.FS

var (
	promptCache   = make(map[string]string)
	promptCacheMu sync.RWMutex
)

// loadPrompt reads and caches a prompt from an embedded filesystem.
// name should be the full path within the FS (e.g., "prompts/system/system-prompt-main-system-prompt.md").
func loadPrompt(fsys embed.FS, name string) string {
	promptCacheMu.RLock()
	if v, ok := promptCache[name]; ok {
		promptCacheMu.RUnlock()
		return v
	}
	promptCacheMu.RUnlock()

	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return ""
	}
	content := string(data)

	promptCacheMu.Lock()
	promptCache[name] = content
	promptCacheMu.Unlock()

	return content
}

// loadSystemPrompt loads a prompt from the system prompts directory.
func loadSystemPrompt(name string) string {
	return loadPrompt(systemPrompts, "prompts/system/"+name)
}

// loadAgentPrompt loads a prompt from the agents directory.
func loadAgentPrompt(name string) string {
	return loadPrompt(agentPrompts, "prompts/agents/"+name)
}

// loadReminderPrompt loads a prompt from the reminders directory.
func loadReminderPrompt(name string) string {
	return loadPrompt(reminderPrompts, "prompts/reminders/"+name)
}

// loadToolPrompt loads a prompt from the tools directory.
func loadToolPrompt(name string) string {
	return loadPrompt(toolPrompts, "prompts/tools/"+name)
}

// loadDataPrompt loads a prompt from the data directory.
func loadDataPrompt(name string) string {
	return loadPrompt(dataPrompts, "prompts/data/"+name)
}

// loadSkillPrompt loads a prompt from the skills directory.
func loadSkillPrompt(name string) string {
	return loadPrompt(skillPrompts, "prompts/skills/"+name)
}
