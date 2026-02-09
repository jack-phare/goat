package prompt

import "github.com/jg-phare/goat/pkg/types"

// ResolveSkills merges multiple skill maps, with higher priority overwriting lower.
// The function iterates sources in order and applies priority-based overwrites.
func ResolveSkills(sources ...map[string]types.SkillEntry) map[string]types.SkillEntry {
	merged := make(map[string]types.SkillEntry)
	for _, source := range sources {
		for name, entry := range source {
			if existing, ok := merged[name]; !ok || entry.Priority >= existing.Priority {
				merged[name] = entry
			}
		}
	}
	return merged
}
