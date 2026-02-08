package subagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jg-phare/goat/pkg/types"
	"gopkg.in/yaml.v3"
)

// frontmatterData represents the YAML frontmatter fields in an agent .md file.
type frontmatterData struct {
	Name             string                                  `yaml:"name"`
	Description      string                                  `yaml:"description"`
	Tools            flexStringList                          `yaml:"tools"`
	DisallowedTools  flexStringList                          `yaml:"disallowedTools"`
	Model            string                                  `yaml:"model"`
	MCPServers       []string                                `yaml:"mcpServers"`
	MaxTurns         *int                                    `yaml:"maxTurns"`
	PermissionMode   string                                  `yaml:"permissionMode"`
	Skills           []string                                `yaml:"skills"`
	Memory           string                                  `yaml:"memory"`
	Hooks            map[types.HookEvent][]types.HookCallbackMatcher `yaml:"hooks"`
	CriticalReminder string                                  `yaml:"criticalSystemReminder_EXPERIMENTAL"`
}

// flexStringList handles YAML that can be either a comma-separated string or a list.
// e.g., "Read, Glob, Grep" or ["Read", "Glob", "Grep"]
type flexStringList []string

func (f *flexStringList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*f = list
		return nil
	case yaml.ScalarNode:
		parts := strings.Split(value.Value, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		*f = result
		return nil
	default:
		return fmt.Errorf("expected string or list for tools, got YAML kind %d", value.Kind)
	}
}

// splitFrontmatter extracts YAML frontmatter and body from Markdown content.
// Frontmatter is delimited by "---" lines at the start of the file.
func splitFrontmatter(data []byte) (yamlPart []byte, body string, err error) {
	content := string(data)

	// Must start with ---
	if !strings.HasPrefix(content, "---") {
		return nil, content, nil // no frontmatter
	}

	// Find closing ---
	rest := content[3:] // skip opening ---
	// Skip newline after opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return nil, content, nil // no closing delimiter, treat as body only
	}

	yamlContent := rest[:endIdx]
	remaining := rest[endIdx+4:] // skip \n---

	// Skip newline after closing ---
	if len(remaining) > 0 && remaining[0] == '\n' {
		remaining = remaining[1:]
	} else if len(remaining) > 1 && remaining[0] == '\r' && remaining[1] == '\n' {
		remaining = remaining[2:]
	}

	return []byte(yamlContent), remaining, nil
}

// ParseFile reads an agent definition from a Markdown file with YAML frontmatter.
func ParseFile(path string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading agent file %s: %w", path, err)
	}

	return ParseContent(data, path)
}

// ParseContent parses agent definition from raw content with an associated file path.
func ParseContent(data []byte, filePath string) (*Definition, error) {
	yamlPart, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter in %s: %w", filePath, err)
	}

	if len(yamlPart) == 0 {
		return nil, fmt.Errorf("no frontmatter found in %s", filePath)
	}

	var fm frontmatterData
	if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
		return nil, fmt.Errorf("parsing YAML in %s: %w", filePath, err)
	}

	// Validate required fields
	if fm.Name == "" {
		// Derive name from filename
		base := filepath.Base(filePath)
		fm.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if fm.Description == "" {
		return nil, fmt.Errorf("missing required field 'description' in %s", filePath)
	}

	def := &Definition{
		AgentDefinition: types.AgentDefinition{
			Name:             fm.Name,
			Description:      fm.Description,
			Tools:            []string(fm.Tools),
			DisallowedTools:  []string(fm.DisallowedTools),
			Model:            fm.Model,
			MCPServers:       fm.MCPServers,
			MaxTurns:         fm.MaxTurns,
			PermissionMode:   fm.PermissionMode,
			Skills:           fm.Skills,
			Memory:           fm.Memory,
			Hooks:            fm.Hooks,
			CriticalReminder: fm.CriticalReminder,
			Prompt:           strings.TrimSpace(body),
		},
		FilePath: filePath,
	}

	return def, nil
}
