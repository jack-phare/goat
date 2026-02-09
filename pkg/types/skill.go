package types

// SkillDefinition describes a reusable skill that can be invoked by the LLM.
type SkillDefinition struct {
	Name         string   `json:"name" yaml:"name"`
	Description  string   `json:"description" yaml:"description"`
	AllowedTools []string `json:"allowed-tools,omitempty" yaml:"allowed-tools"`
	WhenToUse    string   `json:"when_to_use,omitempty" yaml:"when_to_use"`
	ArgumentHint string   `json:"argument-hint,omitempty" yaml:"argument-hint"`
	Arguments    []string `json:"arguments,omitempty" yaml:"arguments"`
	Context      string   `json:"context,omitempty" yaml:"context"` // "inline" (default) or "fork"
	Body         string   `json:"body,omitempty" yaml:"-"`          // markdown body after frontmatter
}

// SkillSource identifies where a skill definition was loaded from.
type SkillSource int

const (
	SkillSourceEmbedded SkillSource = iota
	SkillSourcePlugin
	SkillSourceUser
	SkillSourceProject
)

// String returns a human-readable name for the skill source.
func (s SkillSource) String() string {
	switch s {
	case SkillSourceEmbedded:
		return "embedded"
	case SkillSourcePlugin:
		return "plugin"
	case SkillSourceUser:
		return "user"
	case SkillSourceProject:
		return "project"
	default:
		return "unknown"
	}
}

// SkillEntry wraps a SkillDefinition with loader metadata.
type SkillEntry struct {
	SkillDefinition
	Source   SkillSource
	Priority int
	FilePath string
}
