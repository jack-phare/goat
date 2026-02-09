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
	Color            string                                  `yaml:"color"`
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

// knownFrontmatterKeys is the set of valid YAML field names in agent frontmatter.
var knownFrontmatterKeys = map[string]bool{
	"name":                                true,
	"description":                         true,
	"tools":                               true,
	"disallowedTools":                     true,
	"model":                               true,
	"mcpServers":                          true,
	"maxTurns":                            true,
	"permissionMode":                      true,
	"skills":                              true,
	"memory":                              true,
	"hooks":                               true,
	"criticalSystemReminder_EXPERIMENTAL": true,
	"color":                               true,
}

// typoSuggestions maps common typos to the correct field name.
var typoSuggestions = map[string]string{
	"tols":            "tools",
	"tool":            "tools",
	"desc":            "description",
	"desciption":      "description",
	"permissionmode":  "permissionMode",
	"permission_mode": "permissionMode",
	"maxturns":        "maxTurns",
	"max_turns":       "maxTurns",
	"disallowedtools": "disallowedTools",
	"mcpservers":      "mcpServers",
}

// detectUnknownFields performs a first-pass YAML unmarshal into a raw map
// and checks for keys not in knownFrontmatterKeys.
func detectUnknownFields(yamlPart []byte) []string {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(yamlPart, &raw); err != nil {
		return nil // if we can't parse raw, the main parse will catch it
	}

	var warnings []string
	for key := range raw {
		if !knownFrontmatterKeys[key] {
			msg := fmt.Sprintf("unknown field %q", key)
			if suggestion, ok := typoSuggestions[strings.ToLower(key)]; ok {
				msg += fmt.Sprintf(" (did you mean %q?)", suggestion)
			}
			warnings = append(warnings, msg)
		}
	}
	return warnings
}

// ParseFile reads an agent definition from a Markdown file with YAML frontmatter.
func ParseFile(path string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading agent file %s: %w", path, err)
	}

	def, _, err := ParseContentWithWarnings(data, path)
	return def, err
}

// ParseContent parses agent definition from raw content with an associated file path.
// For backward compatibility, this does not return warnings. Use ParseContentWithWarnings
// to get unknown field warnings.
func ParseContent(data []byte, filePath string) (*Definition, error) {
	def, _, err := ParseContentWithWarnings(data, filePath)
	return def, err
}

// ParseContentWithWarnings parses agent definition and also returns warnings
// about unknown YAML fields.
func ParseContentWithWarnings(data []byte, filePath string) (*Definition, []string, error) {
	yamlPart, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing frontmatter in %s: %w", filePath, err)
	}

	if len(yamlPart) == 0 {
		return nil, nil, fmt.Errorf("no frontmatter found in %s", filePath)
	}

	// First pass: detect unknown fields
	fieldWarnings := detectUnknownFields(yamlPart)

	// Second pass: unmarshal into typed struct
	var fm frontmatterData
	if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
		return nil, fieldWarnings, fmt.Errorf("parsing YAML in %s: %w", filePath, err)
	}

	// Validate required fields
	if fm.Name == "" {
		// Derive name from filename
		base := filepath.Base(filePath)
		fm.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if fm.Description == "" {
		return nil, fieldWarnings, fmt.Errorf("missing required field 'description' in %s", filePath)
	}

	// Validate permissionMode
	if fm.PermissionMode != "" {
		if !validPermissionModes[fm.PermissionMode] {
			return nil, fieldWarnings, fmt.Errorf("invalid permissionMode %q in %s; valid modes: default, acceptEdits, bypassPermissions, plan, delegate, dontAsk", fm.PermissionMode, filePath)
		}
	}

	// Validate maxTurns
	if fm.MaxTurns != nil && *fm.MaxTurns <= 0 {
		return nil, fieldWarnings, fmt.Errorf("maxTurns must be positive in %s, got %d", filePath, *fm.MaxTurns)
	}

	// Validate model
	if fm.Model != "" {
		if !isValidModelValue(fm.Model) {
			return nil, fieldWarnings, fmt.Errorf("invalid model %q in %s; use a known alias (haiku, sonnet, opus) or a full model ID", fm.Model, filePath)
		}
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
			Color:            fm.Color,
			Prompt:           strings.TrimSpace(body),
		},
		FilePath: filePath,
	}

	return def, fieldWarnings, nil
}

// validPermissionModes is the set of accepted permission mode values.
var validPermissionModes = map[string]bool{
	"default":           true,
	"acceptEdits":       true,
	"bypassPermissions": true,
	"plan":              true,
	"delegate":          true,
	"dontAsk":           true,
}

// isValidModelValue checks whether a model string is a known alias or looks like a full model ID.
func isValidModelValue(model string) bool {
	// Known aliases
	if _, ok := modelAliases[model]; ok {
		return true
	}
	// Full model IDs typically contain '-' or '/' (e.g., "claude-sonnet-4-5-20250929" or "anthropic/claude-3")
	return strings.ContainsAny(model, "-/")
}
