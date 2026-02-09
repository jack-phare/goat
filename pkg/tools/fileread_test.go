package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileRead_FullFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644)

	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	// Should contain line numbers
	if !strings.Contains(out.Content, "1\tline1") {
		t.Errorf("expected line 1, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "3\tline3") {
		t.Errorf("expected line 3, got %q", out.Content)
	}
}

func TestFileRead_OffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0o644)

	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
		"offset":    float64(2),
		"limit":     float64(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	lines := strings.Split(out.Content, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), out.Content)
	}
	if !strings.Contains(lines[0], "b") {
		t.Errorf("expected 'b' on first line, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "c") {
		t.Errorf("expected 'c' on second line, got %q", lines[1])
	}
}

func TestFileRead_NonexistentFile(t *testing.T) {
	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": "/nonexistent/path/file.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for nonexistent file")
	}
}

func TestFileRead_RelativePath(t *testing.T) {
	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": "relative/path.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for relative path")
	}
	if !strings.Contains(out.Content, "absolute") {
		t.Errorf("expected error about absolute path, got %q", out.Content)
	}
}

func TestFileRead_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	os.WriteFile(path, []byte{}, 0o644)

	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "empty") {
		t.Errorf("expected empty file message, got %q", out.Content)
	}
}

// --- PDF Tests ---

func TestFileRead_PDF_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.pdf")
	os.WriteFile(path, []byte("not a real pdf"), 0o644)

	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for invalid PDF")
	}
	if !strings.Contains(out.Content, "Error") {
		t.Errorf("expected error message, got %q", out.Content)
	}
}

func TestFileRead_PDF_NonexistentFile(t *testing.T) {
	tool := &FileReadTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"file_path": "/nonexistent/path/file.pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for nonexistent PDF")
	}
}

func TestFileRead_PDF_DetectionByExtension(t *testing.T) {
	dir := t.TempDir()
	// .pdf extension triggers PDF path even with garbage content
	path := filepath.Join(dir, "test.pdf")
	os.WriteFile(path, []byte("not pdf"), 0o644)

	tool := &FileReadTool{}
	out, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
	})
	// Should error from PDF parser, not text reader
	if !out.IsError {
		t.Error("expected error from PDF parser")
	}
	if !strings.Contains(out.Content, "PDF") {
		t.Errorf("expected PDF error, got %q", out.Content)
	}
}

func TestParsePDFPageRange(t *testing.T) {
	tests := []struct {
		input      string
		totalPages int
		wantStart  int
		wantEnd    int
		wantErr    bool
	}{
		{"1-5", 10, 1, 5, false},
		{"3", 10, 3, 3, false},
		{"10-20", 30, 10, 20, false},
		{"1-100", 10, 1, 10, false},   // clamped to totalPages
		{"5-3", 10, 0, 0, true},       // invalid range
		{"abc", 10, 0, 0, true},       // invalid number
		{"1-abc", 10, 0, 0, true},     // invalid end
		{" 2 - 4 ", 10, 2, 4, false},  // whitespace tolerance
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%d", tt.input, tt.totalPages), func(t *testing.T) {
			start, end, err := parsePDFPageRange(tt.input, tt.totalPages)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if start != tt.wantStart {
				t.Errorf("start = %d, want %d", start, tt.wantStart)
			}
			if end != tt.wantEnd {
				t.Errorf("end = %d, want %d", end, tt.wantEnd)
			}
		})
	}
}

func TestFileRead_PDF_PagesSchemaPresent(t *testing.T) {
	tool := &FileReadTool{}
	schema := tool.InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	pages, ok := props["pages"].(map[string]any)
	if !ok {
		t.Fatal("expected 'pages' property")
	}
	if pages["type"] != "string" {
		t.Errorf("pages type = %v, want string", pages["type"])
	}
}
