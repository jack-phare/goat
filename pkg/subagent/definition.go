package subagent

import "github.com/jg-phare/goat/pkg/types"

// AgentSource identifies where an agent definition came from.
type AgentSource int

const (
	SourceBuiltIn AgentSource = iota // hard-coded in Go
	SourceCLIFlag                    // --agents JSON flag
	SourceProject                    // .claude/agents/*.md
	SourceUser                       // ~/.claude/agents/*.md
	SourcePlugin                     // plugin-provided
)

// String returns a human-readable label for the source.
func (s AgentSource) String() string {
	switch s {
	case SourceBuiltIn:
		return "built-in"
	case SourceCLIFlag:
		return "cli"
	case SourceProject:
		return "project"
	case SourceUser:
		return "user"
	case SourcePlugin:
		return "plugin"
	default:
		return "unknown"
	}
}

// Definition wraps types.AgentDefinition with loader metadata.
type Definition struct {
	types.AgentDefinition

	// Loader metadata (not serialized in frontmatter)
	Source   AgentSource // where this definition was loaded from
	Priority int        // higher = overrides lower; BuiltIn < CLI < Plugin < User < Project
	FilePath string     // path to the source file (empty for built-in/CLI)
}

// FromTypesDefinition converts a types.AgentDefinition into a Definition
// with the given source and priority.
func FromTypesDefinition(name string, ad types.AgentDefinition, source AgentSource, priority int) Definition {
	d := Definition{
		AgentDefinition: ad,
		Source:          source,
		Priority:        priority,
	}
	if d.Name == "" {
		d.Name = name
	}
	return d
}
