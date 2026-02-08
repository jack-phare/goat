package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// GlobTool finds files by glob pattern.
type GlobTool struct {
	CWD string
}

func (g *GlobTool) Name() string { return "Glob" }

func (g *GlobTool) Description() string {
	return "Fast file pattern matching tool that works with any codebase size. Supports glob patterns like \"**/*.js\" or \"src/**/*.ts\"."
}

func (g *GlobTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "The glob pattern to match files against",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "The directory to search in (default: CWD)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (g *GlobTool) SideEffect() SideEffectType { return SideEffectNone }

func (g *GlobTool) Execute(_ context.Context, input map[string]any) (ToolOutput, error) {
	pattern, ok := input["pattern"].(string)
	if !ok || pattern == "" {
		return ToolOutput{Content: "Error: pattern is required", IsError: true}, nil
	}

	searchDir := g.CWD
	if p, ok := input["path"].(string); ok && p != "" {
		searchDir = p
	}

	// Resolve the full pattern
	fullPattern := filepath.Join(searchDir, pattern)

	matches, err := doublestar.FilepathGlob(fullPattern)
	if err != nil {
		return ToolOutput{Content: fmt.Sprintf("Error: %s", err), IsError: true}, nil
	}

	sort.Strings(matches)

	if len(matches) == 0 {
		return ToolOutput{Content: "No files matched the pattern."}, nil
	}

	return ToolOutput{Content: strings.Join(matches, "\n")}, nil
}
