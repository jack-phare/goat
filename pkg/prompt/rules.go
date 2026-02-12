package prompt

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// Rule represents a single rule loaded from .claude/rules/.
type Rule struct {
	Path         string   // file path the rule was loaded from
	Content      string   // markdown body (after frontmatter)
	PathPatterns []string // glob patterns from YAML frontmatter `paths:` field
}

// IsConditional returns true if the rule has path patterns (only inject when files match).
func (r Rule) IsConditional() bool {
	return len(r.PathPatterns) > 0
}

// LoadRules recursively scans a directory for .md rule files.
// Returns nil, nil if the directory doesn't exist.
func LoadRules(dir string) ([]Rule, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var rules []Rule
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		rule, err := ParseRuleFrontmatter(string(data))
		if err != nil {
			return nil // skip parse errors
		}
		rule.Path = path
		rules = append(rules, rule)
		return nil
	})
	return rules, err
}

// ruleFrontmatter is the YAML structure for rule frontmatter.
type ruleFrontmatter struct {
	Paths []string `yaml:"paths"`
}

// ParseRuleFrontmatter extracts YAML frontmatter and markdown body from rule content.
// Frontmatter is delimited by "---" lines at the start of the file.
func ParseRuleFrontmatter(content string) (Rule, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		// No frontmatter — entire content is the body
		return Rule{Content: content}, nil
	}

	// Find the closing "---"
	rest := content[3:] // skip opening "---"
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	// Handle empty frontmatter: rest starts with "---"
	if strings.HasPrefix(rest, "---") {
		body := rest[3:]
		body = strings.TrimLeft(body, "\r\n")
		return Rule{Content: strings.TrimSpace(body)}, nil
	}

	closeIdx := strings.Index(rest, "\n---")
	if closeIdx < 0 {
		// No closing delimiter — treat entire content as body
		return Rule{Content: content}, nil
	}

	yamlBlock := rest[:closeIdx]
	body := rest[closeIdx+4:] // skip "\n---"
	body = strings.TrimLeft(body, "\r\n")

	var fm ruleFrontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		// Invalid YAML — treat entire content as body
		return Rule{Content: content}, nil
	}

	return Rule{
		Content:      strings.TrimSpace(body),
		PathPatterns: fm.Paths,
	}, nil
}

// MatchRules filters rules to those that should be active given the current file paths.
// Unconditional rules (no paths: field) are always included.
// Conditional rules are included only when at least one file path matches a pattern.
func MatchRules(rules []Rule, filePaths []string) []Rule {
	var matched []Rule
	for _, rule := range rules {
		if !rule.IsConditional() {
			matched = append(matched, rule)
			continue
		}
		if matchesAny(rule.PathPatterns, filePaths) {
			matched = append(matched, rule)
		}
	}
	return matched
}

// matchesAny returns true if any file path matches any of the glob patterns.
func matchesAny(patterns, filePaths []string) bool {
	for _, pattern := range patterns {
		for _, fp := range filePaths {
			if ok, _ := doublestar.Match(pattern, fp); ok {
				return true
			}
		}
	}
	return false
}
