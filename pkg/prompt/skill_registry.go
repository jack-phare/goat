package prompt

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/jg-phare/goat/pkg/types"
)

// SkillRegistry holds all available skills, merging embedded and filesystem-loaded skills.
// It is safe for concurrent use.
type SkillRegistry struct {
	mu     sync.RWMutex
	skills map[string]types.SkillEntry
}

// NewSkillRegistry creates an empty SkillRegistry.
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills: make(map[string]types.SkillEntry),
	}
}

// Register adds or overwrites a skill entry by name.
func (r *SkillRegistry) Register(entry types.SkillEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[entry.Name] = entry
}

// Unregister removes a skill by name.
func (r *SkillRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.skills, name)
}

// Get retrieves a skill by name.
func (r *SkillRegistry) Get(name string) (types.SkillEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.skills[name]
	return entry, ok
}

// GetSkill retrieves a skill by name (satisfies SkillProvider interface).
func (r *SkillRegistry) GetSkill(name string) (types.SkillEntry, bool) {
	return r.Get(name)
}

// List returns all skill entries sorted by name.
func (r *SkillRegistry) List() []types.SkillEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]types.SkillEntry, 0, len(r.skills))
	for _, e := range r.skills {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// ListSkills returns all skill entries sorted by name (satisfies SkillProvider interface).
func (r *SkillRegistry) ListSkills() []types.SkillEntry {
	return r.List()
}

// Names returns all registered skill names in sorted order.
func (r *SkillRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SkillNames returns all skill names in sorted order (satisfies SkillProvider interface).
func (r *SkillRegistry) SkillNames() []string {
	return r.Names()
}

// SlashCommands returns skill names formatted as slash commands.
func (r *SkillRegistry) SlashCommands() []string {
	names := r.Names()
	cmds := make([]string, len(names))
	for i, name := range names {
		cmds[i] = name
	}
	return cmds
}

// FormatSkillsList generates a formatted string listing all skills for system prompt injection.
func (r *SkillRegistry) FormatSkillsList() string {
	entries := r.List()
	if len(entries) == 0 {
		return ""
	}

	var lines []string
	for _, e := range entries {
		line := fmt.Sprintf("- %s: %s", e.Name, e.Description)
		if e.WhenToUse != "" {
			line += fmt.Sprintf(". %s", e.WhenToUse)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
