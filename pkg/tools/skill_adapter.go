package tools

import "github.com/jg-phare/goat/pkg/types"

// SkillEntryProvider is the interface that the agent's SkillProvider satisfies.
// This allows tools to use skills without importing the agent package.
type SkillEntryProvider interface {
	GetSkill(name string) (types.SkillEntry, bool)
}

// SkillProviderAdapter adapts a SkillEntryProvider (from agent/prompt) to the tools.SkillProvider interface.
type SkillProviderAdapter struct {
	Inner SkillEntryProvider
}

// GetSkillInfo converts a types.SkillEntry to a tools.SkillInfo.
func (a *SkillProviderAdapter) GetSkillInfo(name string) (SkillInfo, bool) {
	entry, ok := a.Inner.GetSkill(name)
	if !ok {
		return SkillInfo{}, false
	}
	return SkillInfo{
		Name:         entry.Name,
		Description:  entry.Description,
		Body:         entry.Body,
		AllowedTools: entry.AllowedTools,
		Arguments:    entry.Arguments,
		Context:      entry.Context,
	}, true
}
