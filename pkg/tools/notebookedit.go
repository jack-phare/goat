package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NotebookEditTool edits Jupyter notebook cells.
type NotebookEditTool struct{}

func (n *NotebookEditTool) Name() string { return "NotebookEdit" }

func (n *NotebookEditTool) Description() string {
	return "Replaces, inserts, or deletes cells in a Jupyter notebook (.ipynb file)."
}

func (n *NotebookEditTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"notebook_path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the .ipynb file",
			},
			"new_source": map[string]any{
				"type":        "string",
				"description": "The new source for the cell",
			},
			"cell_number": map[string]any{
				"type":        "integer",
				"description": "0-indexed cell number",
			},
			"cell_id": map[string]any{
				"type":        "string",
				"description": "The ID of the cell to edit",
			},
			"cell_type": map[string]any{
				"type":        "string",
				"enum":        []string{"code", "markdown"},
				"description": "The type of the cell",
			},
			"edit_mode": map[string]any{
				"type":        "string",
				"enum":        []string{"replace", "insert", "delete"},
				"description": "The edit operation (default: replace)",
			},
		},
		"required": []string{"notebook_path", "new_source"},
	}
}

func (n *NotebookEditTool) SideEffect() SideEffectType { return SideEffectMutating }

func (n *NotebookEditTool) Execute(_ context.Context, input map[string]any) (ToolOutput, error) {
	nbPath, ok := input["notebook_path"].(string)
	if !ok || nbPath == "" {
		return ToolOutput{Content: "Error: notebook_path is required", IsError: true}, nil
	}

	if !filepath.IsAbs(nbPath) {
		return ToolOutput{Content: "Error: notebook_path must be an absolute path", IsError: true}, nil
	}
	if !strings.HasSuffix(nbPath, ".ipynb") {
		return ToolOutput{Content: "Error: notebook_path must end in .ipynb", IsError: true}, nil
	}

	editMode := "replace"
	if m, ok := input["edit_mode"].(string); ok && m != "" {
		editMode = m
	}

	newSource, _ := input["new_source"].(string)
	if editMode != "delete" && newSource == "" {
		return ToolOutput{Content: "Error: new_source is required for replace/insert", IsError: true}, nil
	}

	// Read the notebook
	data, err := os.ReadFile(nbPath)
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error reading notebook: %s", err),
			IsError: true,
		}, nil
	}

	// Parse as generic JSON to preserve unknown fields
	var notebook map[string]any
	if err := json.Unmarshal(data, &notebook); err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error parsing notebook: %s", err),
			IsError: true,
		}, nil
	}

	rawCells, ok := notebook["cells"].([]any)
	if !ok {
		return ToolOutput{Content: "Error: notebook has no cells array", IsError: true}, nil
	}

	// Determine target cell index
	cellIdx := -1
	if num, ok := input["cell_number"].(float64); ok {
		cellIdx = int(num)
	} else if cellID, ok := input["cell_id"].(string); ok && cellID != "" {
		for i, c := range rawCells {
			cell, _ := c.(map[string]any)
			if id, _ := cell["id"].(string); id == cellID {
				cellIdx = i
				break
			}
		}
		if cellIdx == -1 {
			return ToolOutput{
				Content: fmt.Sprintf("Error: cell with id %q not found", cellID),
				IsError: true,
			}, nil
		}
	}

	// Split source into lines for the Jupyter format
	sourceLines := splitSourceLines(newSource)

	switch editMode {
	case "replace":
		if cellIdx < 0 || cellIdx >= len(rawCells) {
			return ToolOutput{
				Content: fmt.Sprintf("Error: cell_number %d out of range (0-%d)", cellIdx, len(rawCells)-1),
				IsError: true,
			}, nil
		}
		cell, _ := rawCells[cellIdx].(map[string]any)
		cell["source"] = sourceLines
		if ct, ok := input["cell_type"].(string); ok && ct != "" {
			cell["cell_type"] = ct
		}

	case "insert":
		ct := "code"
		if cellType, ok := input["cell_type"].(string); ok && cellType != "" {
			ct = cellType
		}

		newCell := map[string]any{
			"cell_type": ct,
			"source":    sourceLines,
			"metadata":  map[string]any{},
		}
		if ct == "code" {
			newCell["outputs"] = []any{}
			newCell["execution_count"] = nil
		}

		insertAt := 0
		if cellIdx >= 0 {
			insertAt = cellIdx + 1 // insert after the referenced cell
		}
		if insertAt > len(rawCells) {
			insertAt = len(rawCells)
		}

		// Insert into slice
		rawCells = append(rawCells, nil)
		copy(rawCells[insertAt+1:], rawCells[insertAt:])
		rawCells[insertAt] = newCell

	case "delete":
		if cellIdx < 0 || cellIdx >= len(rawCells) {
			return ToolOutput{
				Content: fmt.Sprintf("Error: cell_number %d out of range (0-%d)", cellIdx, len(rawCells)-1),
				IsError: true,
			}, nil
		}
		rawCells = append(rawCells[:cellIdx], rawCells[cellIdx+1:]...)

	default:
		return ToolOutput{
			Content: fmt.Sprintf("Error: unknown edit_mode %q", editMode),
			IsError: true,
		}, nil
	}

	notebook["cells"] = rawCells

	// Write back with indentation to preserve formatting
	out, err := json.MarshalIndent(notebook, "", " ")
	if err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error marshaling notebook: %s", err),
			IsError: true,
		}, nil
	}

	if err := os.WriteFile(nbPath, append(out, '\n'), 0644); err != nil {
		return ToolOutput{
			Content: fmt.Sprintf("Error writing notebook: %s", err),
			IsError: true,
		}, nil
	}

	return ToolOutput{
		Content: fmt.Sprintf("Notebook %s updated (%s at cell %d). Total cells: %d", nbPath, editMode, cellIdx, len(rawCells)),
	}, nil
}

// splitSourceLines converts a source string to Jupyter's line array format.
// Each line (except the last) gets a trailing \n.
func splitSourceLines(source string) []string {
	if source == "" {
		return []string{}
	}
	lines := strings.Split(source, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		if i < len(lines)-1 {
			result[i] = line + "\n"
		} else {
			result[i] = line
		}
	}
	return result
}
