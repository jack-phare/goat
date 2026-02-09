package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const grepMaxOutput = 100000 // characters

// GrepTool searches file contents using ripgrep.
type GrepTool struct {
	CWD string
}

func (g *GrepTool) Name() string { return "Grep" }

func (g *GrepTool) Description() string {
	return `A powerful search tool built on ripgrep

  Usage:
  - ALWAYS use Grep for search tasks. NEVER invoke ` + "`grep`" + ` or ` + "`rg`" + ` as a Bash command. The Grep tool has been optimized for correct permissions and access.
  - Supports full regex syntax (e.g., "log.*Error", "function\\s+\\w+")
  - Filter files with glob parameter (e.g., "*.js", "**/*.tsx") or type parameter (e.g., "js", "py", "rust")
  - Output modes: "content" shows matching lines, "files_with_matches" shows only file paths (default), "count" shows match counts
  - Use Agent tool for open-ended searches requiring multiple rounds
  - Pattern syntax: Uses ripgrep (not grep) - literal braces need escaping (use ` + "`interface\\{\\}`" + ` to find ` + "`interface{}`" + ` in Go code)
  - Multiline matching: By default patterns match within single lines only. For cross-line patterns like ` + "`struct \\{[\\s\\S]*?field`" + `, use ` + "`multiline: true`" + ``
}

func (g *GrepTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "The regular expression pattern to search for",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to search in (default: CWD)",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Glob pattern to filter files (e.g. \"*.js\")",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"description": "Output mode: content, files_with_matches, count",
			},
			"-i": map[string]any{
				"type":        "boolean",
				"description": "Case insensitive search",
			},
			"-n": map[string]any{
				"type":        "boolean",
				"description": "Show line numbers (default true)",
			},
			"-A": map[string]any{
				"type":        "number",
				"description": "Lines to show after each match",
			},
			"-B": map[string]any{
				"type":        "number",
				"description": "Lines to show before each match",
			},
			"-C": map[string]any{
				"type":        "number",
				"description": "Lines of context around each match",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "File type filter (e.g. js, py, go)",
			},
			"head_limit": map[string]any{
				"type":        "number",
				"description": "Limit output to first N entries",
			},
			"multiline": map[string]any{
				"type":        "boolean",
				"description": "Enable multiline mode",
			},
		},
		"required": []string{"pattern"},
	}
}

func (g *GrepTool) SideEffect() SideEffectType { return SideEffectNone }

func (g *GrepTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	pattern, ok := input["pattern"].(string)
	if !ok || pattern == "" {
		return ToolOutput{Content: "Error: pattern is required", IsError: true}, nil
	}

	args := g.buildArgs(input, pattern)

	searchPath := g.CWD
	if p, ok := input["path"].(string); ok && p != "" {
		searchPath = p
	}
	args = append(args, searchPath)

	cmd := exec.CommandContext(ctx, "rg", args...)
	output, err := cmd.CombinedOutput()
	result := strings.TrimRight(string(output), "\n")

	if err != nil {
		// rg returns exit code 1 for no matches — not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return ToolOutput{Content: "No matches found."}, nil
		}
		// rg returns exit code 2 for errors
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			return ToolOutput{Content: fmt.Sprintf("Error: %s", result), IsError: true}, nil
		}
		return ToolOutput{Content: fmt.Sprintf("Error running rg: %s", err), IsError: true}, nil
	}

	if result == "" {
		return ToolOutput{Content: "No matches found."}, nil
	}

	// Apply head_limit
	if hl, ok := input["head_limit"].(float64); ok && hl > 0 {
		lines := strings.Split(result, "\n")
		limit := int(hl)
		if limit < len(lines) {
			result = strings.Join(lines[:limit], "\n")
		}
	}

	// Hard output limit as safety net
	if len(result) > grepMaxOutput {
		totalLen := len(result)
		result = result[:grepMaxOutput] + fmt.Sprintf("\n... (truncated, %d total characters)", totalLen)
	}

	return ToolOutput{Content: result}, nil
}

func (g *GrepTool) buildArgs(input map[string]any, pattern string) []string {
	var args []string

	// Output mode
	outputMode := "files_with_matches"
	if om, ok := input["output_mode"].(string); ok && om != "" {
		outputMode = om
	}

	switch outputMode {
	case "files_with_matches":
		args = append(args, "--files-with-matches")
	case "count":
		args = append(args, "--count")
	case "content":
		// Default rg behavior — show matching lines
		showLineNumbers := true
		if n, ok := input["-n"].(bool); ok {
			showLineNumbers = n
		}
		if showLineNumbers {
			args = append(args, "--line-number")
		}
	}

	// Case insensitive
	if ci, ok := input["-i"].(bool); ok && ci {
		args = append(args, "--ignore-case")
	}

	// Context lines
	if a, ok := input["-A"].(float64); ok && a > 0 {
		args = append(args, "-A", strconv.Itoa(int(a)))
	}
	if b, ok := input["-B"].(float64); ok && b > 0 {
		args = append(args, "-B", strconv.Itoa(int(b)))
	}
	if c, ok := input["-C"].(float64); ok && c > 0 {
		args = append(args, "-C", strconv.Itoa(int(c)))
	}

	// Glob filter
	if gl, ok := input["glob"].(string); ok && gl != "" {
		args = append(args, "--glob", gl)
	}

	// File type
	if ft, ok := input["type"].(string); ok && ft != "" {
		args = append(args, "--type", ft)
	}

	// Multiline
	if ml, ok := input["multiline"].(bool); ok && ml {
		args = append(args, "--multiline", "--multiline-dotall")
	}

	// Pattern
	args = append(args, "--", pattern)

	return args
}
