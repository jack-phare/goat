package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileEditTool performs find-and-replace in files.
type FileEditTool struct{}

func (f *FileEditTool) Name() string { return "Edit" }

func (f *FileEditTool) Description() string {
	return "Performs exact string replacements in files. The edit will FAIL if old_string is not unique in the file unless replace_all is set."
}

func (f *FileEditTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "The absolute path to the file to modify",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "The text to replace",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "The text to replace it with",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace all occurrences of old_string (default false)",
			},
		},
		"required": []string{"file_path", "old_string", "new_string"},
	}
}

func (f *FileEditTool) SideEffect() SideEffectType { return SideEffectMutating }

func (f *FileEditTool) Execute(_ context.Context, input map[string]any) (ToolOutput, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return ToolOutput{Content: "Error: file_path is required", IsError: true}, nil
	}

	if !filepath.IsAbs(filePath) {
		return ToolOutput{Content: "Error: file_path must be an absolute path", IsError: true}, nil
	}

	oldString, ok := input["old_string"].(string)
	if !ok {
		return ToolOutput{Content: "Error: old_string is required", IsError: true}, nil
	}

	newString, ok := input["new_string"].(string)
	if !ok {
		return ToolOutput{Content: "Error: new_string is required", IsError: true}, nil
	}

	if oldString == newString {
		return ToolOutput{Content: "Error: old_string and new_string must be different", IsError: true}, nil
	}

	replaceAll := false
	if ra, ok := input["replace_all"].(bool); ok {
		replaceAll = ra
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return ToolOutput{Content: fmt.Sprintf("Error: %s", err), IsError: true}, nil
	}
	content := string(data)

	count := strings.Count(content, oldString)
	if count == 0 {
		return ToolOutput{
			Content: "Error: old_string not found in file",
			IsError: true,
		}, nil
	}

	if !replaceAll && count > 1 {
		return ToolOutput{
			Content: fmt.Sprintf("Error: old_string found %d times in file. Use replace_all or provide a more unique string.", count),
			IsError: true,
		}, nil
	}

	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(content, oldString, newString)
	} else {
		newContent = strings.Replace(content, oldString, newString, 1)
	}

	if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
		return ToolOutput{Content: fmt.Sprintf("Error writing file: %s", err), IsError: true}, nil
	}

	return ToolOutput{
		Content: fmt.Sprintf("Replaced %d occurrence(s) in %s", count, filePath),
	}, nil
}
