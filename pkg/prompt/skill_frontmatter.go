package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jg-phare/goat/pkg/types"
	"gopkg.in/yaml.v3"
)

// skillFrontmatter represents the YAML frontmatter fields in a SKILL.md file.
type skillFrontmatter struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	AllowedTools []string `yaml:"allowed-tools"`
	WhenToUse    string   `yaml:"when_to_use"`
	ArgumentHint string   `yaml:"argument-hint"`
	Arguments    []string `yaml:"arguments"`
	Context      string   `yaml:"context"`
}

// knownSkillKeys is the set of valid YAML field names in skill frontmatter.
var knownSkillKeys = map[string]bool{
	"name":          true,
	"description":   true,
	"allowed-tools": true,
	"when_to_use":   true,
	"argument-hint": true,
	"arguments":     true,
	"context":       true,
}

// ParseSkillFile reads a skill definition from a SKILL.md file.
func ParseSkillFile(path string) (*types.SkillEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading skill file %s: %w", path, err)
	}
	return ParseSkillContent(data, path)
}

// ParseSkillContent parses a skill definition from raw content with an associated file path.
func ParseSkillContent(data []byte, filePath string) (*types.SkillEntry, error) {
	yamlPart, body, err := splitSkillFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter in %s: %w", filePath, err)
	}

	if len(yamlPart) == 0 {
		return nil, fmt.Errorf("no frontmatter found in %s", filePath)
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
		return nil, fmt.Errorf("parsing YAML in %s: %w", filePath, err)
	}

	// Derive name from directory if not in frontmatter
	if fm.Name == "" {
		fm.Name = deriveSkillName(filePath)
	}

	// Validate required fields
	if fm.Description == "" {
		return nil, fmt.Errorf("missing required field 'description' in %s", filePath)
	}

	// Validate context field
	if fm.Context != "" && fm.Context != "inline" && fm.Context != "fork" {
		return nil, fmt.Errorf("invalid context %q in %s; must be \"inline\" or \"fork\"", fm.Context, filePath)
	}

	// Validate argument entries
	for i, arg := range fm.Arguments {
		if strings.TrimSpace(arg) == "" {
			return nil, fmt.Errorf("empty argument name at index %d in %s", i, filePath)
		}
	}

	entry := &types.SkillEntry{
		SkillDefinition: types.SkillDefinition{
			Name:         fm.Name,
			Description:  fm.Description,
			AllowedTools: fm.AllowedTools,
			WhenToUse:    fm.WhenToUse,
			ArgumentHint: fm.ArgumentHint,
			Arguments:    fm.Arguments,
			Context:      fm.Context,
			Body:         strings.TrimSpace(body),
		},
		FilePath: filePath,
	}

	return entry, nil
}

// splitSkillFrontmatter extracts YAML frontmatter and body from Markdown content.
// Frontmatter is delimited by "---" lines at the start of the file.
// This mirrors the logic in subagent/frontmatter.go:splitFrontmatter.
func splitSkillFrontmatter(data []byte) (yamlPart []byte, body string, err error) {
	content := string(data)

	if !strings.HasPrefix(content, "---") {
		return nil, content, nil
	}

	rest := content[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return nil, content, nil
	}

	yamlContent := rest[:endIdx]
	remaining := rest[endIdx+4:]

	if len(remaining) > 0 && remaining[0] == '\n' {
		remaining = remaining[1:]
	} else if len(remaining) > 1 && remaining[0] == '\r' && remaining[1] == '\n' {
		remaining = remaining[2:]
	}

	return []byte(yamlContent), remaining, nil
}

// deriveSkillName derives a skill name from the file path.
// For a path like ".claude/skills/my-skill/SKILL.md", returns "my-skill".
func deriveSkillName(filePath string) string {
	dir := filepath.Dir(filePath)
	return filepath.Base(dir)
}

// ValidateSkillWarnings returns non-fatal warnings for a skill entry.
type ValidationWarning struct {
	Field   string
	Message string
}

// ValidateSkill returns a list of non-fatal validation warnings.
func ValidateSkill(entry types.SkillEntry) []ValidationWarning {
	var warnings []ValidationWarning

	if entry.WhenToUse == "" {
		warnings = append(warnings, ValidationWarning{
			Field:   "when_to_use",
			Message: "missing 'when_to_use' field; skill may not be auto-invoked effectively",
		})
	}

	if entry.Name == "" {
		warnings = append(warnings, ValidationWarning{
			Field:   "name",
			Message: "missing 'name' field; derived from directory name",
		})
	}

	return warnings
}
