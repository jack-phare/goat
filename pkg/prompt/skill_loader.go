package prompt

import (
	"os"
	"path/filepath"

	"github.com/jg-phare/goat/pkg/types"
)

// SkillLoader discovers and loads skill definitions from the filesystem.
type SkillLoader struct {
	cwd        string
	userDir    string
	pluginDirs []string
}

// NewSkillLoader creates a SkillLoader that scans the given directories.
// cwd is used to find project skills at {cwd}/.claude/skills/.
// userDir is used to find user skills at {userDir}/skills/.
// Optional pluginDirs are scanned for plugin-provided skills.
func NewSkillLoader(cwd, userDir string, pluginDirs ...string) *SkillLoader {
	return &SkillLoader{
		cwd:        cwd,
		userDir:    userDir,
		pluginDirs: pluginDirs,
	}
}

// LoadAll discovers and loads all skill definitions from configured directories.
// Returns a map keyed by skill name. Higher-priority sources overwrite lower.
// Priority: Plugin (10) < User (20) < Project (30).
func (l *SkillLoader) LoadAll() (map[string]types.SkillEntry, error) {
	skills := make(map[string]types.SkillEntry)

	// Plugin dirs (priority 10)
	for _, dir := range l.pluginDirs {
		entries, err := l.scanDir(dir, types.SkillSourcePlugin, 10)
		if err != nil {
			return nil, err
		}
		for name, entry := range entries {
			skills[name] = entry
		}
	}

	// User dir: {userDir}/skills/
	if l.userDir != "" {
		userSkillsDir := filepath.Join(l.userDir, "skills")
		entries, err := l.scanDir(userSkillsDir, types.SkillSourceUser, 20)
		if err != nil {
			return nil, err
		}
		for name, entry := range entries {
			if existing, ok := skills[name]; !ok || entry.Priority >= existing.Priority {
				skills[name] = entry
			}
		}
	}

	// Project dir: {cwd}/.claude/skills/
	if l.cwd != "" {
		projectSkillsDir := filepath.Join(l.cwd, ".claude", "skills")
		entries, err := l.scanDir(projectSkillsDir, types.SkillSourceProject, 30)
		if err != nil {
			return nil, err
		}
		for name, entry := range entries {
			if existing, ok := skills[name]; !ok || entry.Priority >= existing.Priority {
				skills[name] = entry
			}
		}
	}

	return skills, nil
}

// scanDir scans a directory for skill subdirectories containing SKILL.md files.
// Missing directories are silently skipped.
func (l *SkillLoader) scanDir(dir string, source types.SkillSource, priority int) (map[string]types.SkillEntry, error) {
	skills := make(map[string]types.SkillEntry)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return skills, nil // missing directory is fine
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue // no SKILL.md in this subdirectory
		}

		skill, err := ParseSkillFile(skillFile)
		if err != nil {
			return nil, err
		}

		skill.Source = source
		skill.Priority = priority
		skills[skill.Name] = *skill
	}

	return skills, nil
}
