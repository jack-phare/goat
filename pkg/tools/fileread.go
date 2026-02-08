package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	fileReadDefaultLimit   = 2000 // default max lines to read
	fileReadMaxLineLength  = 2000 // truncate lines longer than this
)

// FileReadTool reads file contents with line numbers.
type FileReadTool struct{}

func (f *FileReadTool) Name() string { return "Read" }

func (f *FileReadTool) Description() string {
	return "Reads a file from the local filesystem. The file_path parameter must be an absolute path."
}

func (f *FileReadTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "The absolute path to the file to read",
			},
			"offset": map[string]any{
				"type":        "number",
				"description": "The line number to start reading from (1-indexed)",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "The number of lines to read",
			},
		},
		"required": []string{"file_path"},
	}
}

func (f *FileReadTool) SideEffect() SideEffectType { return SideEffectNone }

func (f *FileReadTool) Execute(_ context.Context, input map[string]any) (ToolOutput, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return ToolOutput{Content: "Error: file_path is required", IsError: true}, nil
	}

	if !filepath.IsAbs(filePath) {
		return ToolOutput{Content: "Error: file_path must be an absolute path", IsError: true}, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return ToolOutput{Content: fmt.Sprintf("Error: %s", err), IsError: true}, nil
	}
	defer file.Close()

	offset := 1 // 1-indexed
	if o, ok := input["offset"].(float64); ok && o > 0 {
		offset = int(o)
	}

	limit := fileReadDefaultLimit
	if l, ok := input["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if linesRead >= limit {
			break
		}

		line := scanner.Text()
		if len(line) > fileReadMaxLineLength {
			line = line[:fileReadMaxLineLength]
		}
		lines = append(lines, fmt.Sprintf("%6d\t%s", lineNum, line))
		linesRead++
	}

	if err := scanner.Err(); err != nil {
		return ToolOutput{Content: fmt.Sprintf("Error reading file: %s", err), IsError: true}, nil
	}

	if len(lines) == 0 {
		return ToolOutput{Content: "(empty file)"}, nil
	}

	return ToolOutput{Content: strings.Join(lines, "\n")}, nil
}
