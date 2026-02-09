package prompt

import "github.com/jg-phare/goat/pkg/types"

// embeddedSkillDef describes an embedded skill's metadata.
// Embedded skills don't have YAML frontmatter; their metadata is defined here.
type embeddedSkillDef struct {
	Name        string
	Description string
	WhenToUse   string
	FileName    string // filename in prompts/skills/ directory
}

// embeddedSkillDefs lists all built-in embedded skills.
var embeddedSkillDefs = []embeddedSkillDef{
	{
		Name:        "debugging",
		Description: "Help debug issues in the current session",
		WhenToUse:   "Use when the user encounters errors or needs help debugging their session",
		FileName:    "skill-debugging.md",
	},
	{
		Name:        "update-claude-code-config",
		Description: "Modify Claude Code configuration by updating settings.json files",
		WhenToUse:   "Use when the user wants to update Claude Code settings, hooks, or permissions",
		FileName:    "skill-update-claude-code-config.md",
	},
	{
		Name:        "verification-specialist",
		Description: "Verify that code changes actually work and fix what they're supposed to fix",
		WhenToUse:   "Use when the user wants to verify code changes, run verification, or test functionality",
		FileName:    "skill-verification-specialist.md",
	},
	{
		Name:        "skillify",
		Description: "Capture the current session's repeatable process as a reusable skill",
		WhenToUse:   "Use when the user wants to save a workflow as a reusable skill. Examples: '/skillify', 'save this as a skill', 'create a skill from this session'",
		FileName:    "skill-skillify.md",
	},
}

// LoadEmbeddedSkills returns all built-in embedded skills as SkillEntry map.
func LoadEmbeddedSkills() map[string]types.SkillEntry {
	skills := make(map[string]types.SkillEntry, len(embeddedSkillDefs))
	for _, def := range embeddedSkillDefs {
		body := loadSkillPrompt(def.FileName)
		skills[def.Name] = types.SkillEntry{
			SkillDefinition: types.SkillDefinition{
				Name:        def.Name,
				Description: def.Description,
				WhenToUse:   def.WhenToUse,
				Body:        body,
			},
			Source:   types.SkillSourceEmbedded,
			Priority: 0,
		}
	}
	return skills
}
