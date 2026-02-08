package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileWriteTool creates or overwrites files.
type FileWriteTool struct{}

func (f *FileWriteTool) Name() string { return "Write" }

func (f *FileWriteTool) Description() string {
	return "Writes a file to the local filesystem. This tool will overwrite the existing file if there is one at the provided path."
}

func (f *FileWriteTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "The absolute path to the file to write",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content to write to the file",
			},
		},
		"required": []string{"file_path", "content"},
	}
}

func (f *FileWriteTool) SideEffect() SideEffectType { return SideEffectMutating }

func (f *FileWriteTool) Execute(_ context.Context, input map[string]any) (ToolOutput, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return ToolOutput{Content: "Error: file_path is required", IsError: true}, nil
	}

	if !filepath.IsAbs(filePath) {
		return ToolOutput{Content: "Error: file_path must be an absolute path", IsError: true}, nil
	}

	content, ok := input["content"].(string)
	if !ok {
		return ToolOutput{Content: "Error: content is required", IsError: true}, nil
	}

	// Create parent directories
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolOutput{Content: fmt.Sprintf("Error creating directories: %s", err), IsError: true}, nil
	}

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return ToolOutput{Content: fmt.Sprintf("Error writing file: %s", err), IsError: true}, nil
	}

	lineCount := strings.Count(content, "\n")
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		lineCount++ // count the last line without trailing newline
	}

	return ToolOutput{Content: fmt.Sprintf("File written successfully at: %s (%d lines)", filePath, lineCount)}, nil
}
