package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestNotebook(t *testing.T, dir string) string {
	t.Helper()
	nb := map[string]any{
		"nbformat":       4,
		"nbformat_minor": 5,
		"metadata":       map[string]any{"kernelspec": map[string]any{"name": "python3"}},
		"cells": []any{
			map[string]any{
				"cell_type": "code",
				"id":        "cell-0",
				"source":    []string{"print('hello')\n"},
				"metadata":  map[string]any{},
				"outputs":   []any{},
			},
			map[string]any{
				"cell_type": "markdown",
				"id":        "cell-1",
				"source":    []string{"# Title\n"},
				"metadata":  map[string]any{},
			},
		},
	}

	data, _ := json.MarshalIndent(nb, "", " ")
	path := filepath.Join(dir, "test.ipynb")
	os.WriteFile(path, data, 0644)
	return path
}

func readNotebookCells(t *testing.T, path string) []any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var nb map[string]any
	json.Unmarshal(data, &nb)
	cells, _ := nb["cells"].([]any)
	return cells
}

func TestNotebookEdit_ReplaceCell(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := &NotebookEditTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_number":   float64(0),
		"new_source":    "print('replaced')",
		"edit_mode":     "replace",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	cells := readNotebookCells(t, path)
	cell := cells[0].(map[string]any)
	source := cell["source"].([]any)
	if source[0] != "print('replaced')" {
		t.Errorf("expected replaced source, got %v", source)
	}
}

func TestNotebookEdit_InsertCell(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := &NotebookEditTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_number":   float64(0),
		"new_source":    "x = 1",
		"edit_mode":     "insert",
		"cell_type":     "code",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	cells := readNotebookCells(t, path)
	if len(cells) != 3 {
		t.Fatalf("expected 3 cells, got %d", len(cells))
	}
	// New cell should be at index 1 (after cell 0)
	newCell := cells[1].(map[string]any)
	if newCell["cell_type"] != "code" {
		t.Errorf("expected code cell, got %v", newCell["cell_type"])
	}
}

func TestNotebookEdit_InsertAtBeginning(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := &NotebookEditTool{}

	// No cell_number and no cell_id → insert at beginning
	out, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"new_source":    "# First cell",
		"edit_mode":     "insert",
		"cell_type":     "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	cells := readNotebookCells(t, path)
	if len(cells) != 3 {
		t.Fatalf("expected 3 cells, got %d", len(cells))
	}
	firstCell := cells[0].(map[string]any)
	if firstCell["cell_type"] != "markdown" {
		t.Errorf("expected markdown cell at beginning, got %v", firstCell["cell_type"])
	}
}

func TestNotebookEdit_DeleteCell(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := &NotebookEditTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_number":   float64(0),
		"new_source":    "",
		"edit_mode":     "delete",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	cells := readNotebookCells(t, path)
	if len(cells) != 1 {
		t.Fatalf("expected 1 cell after delete, got %d", len(cells))
	}
}

func TestNotebookEdit_InvalidCellNumber(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := &NotebookEditTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_number":   float64(99),
		"new_source":    "test",
		"edit_mode":     "replace",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for invalid cell number")
	}
}

func TestNotebookEdit_NonIpynb(t *testing.T) {
	tool := &NotebookEditTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": "/tmp/test.py",
		"new_source":    "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for non-.ipynb file")
	}
}

func TestNotebookEdit_RelativePath(t *testing.T) {
	tool := &NotebookEditTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": "relative/test.ipynb",
		"new_source":    "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for relative path")
	}
}

func TestNotebookEdit_PreservesMetadata(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := &NotebookEditTool{}

	tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_number":   float64(0),
		"new_source":    "modified",
		"edit_mode":     "replace",
	})

	data, _ := os.ReadFile(path)
	var nb map[string]any
	json.Unmarshal(data, &nb)

	// Verify notebook metadata is preserved
	meta, ok := nb["metadata"].(map[string]any)
	if !ok {
		t.Fatal("metadata lost")
	}
	kernel, ok := meta["kernelspec"].(map[string]any)
	if !ok {
		t.Fatal("kernelspec lost")
	}
	if kernel["name"] != "python3" {
		t.Error("kernelspec.name lost")
	}

	// Second cell should be unchanged
	cells := nb["cells"].([]any)
	cell1 := cells[1].(map[string]any)
	if cell1["cell_type"] != "markdown" {
		t.Error("second cell modified unexpectedly")
	}
}

func TestNotebookEdit_CellByID(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := &NotebookEditTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "cell-1",
		"new_source":    "# Modified Title",
		"edit_mode":     "replace",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	cells := readNotebookCells(t, path)
	cell := cells[1].(map[string]any)
	source := cell["source"].([]any)
	if !strings.Contains(source[0].(string), "Modified Title") {
		t.Errorf("expected modified title, got %v", source)
	}
}
