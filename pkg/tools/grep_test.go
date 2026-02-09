package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrep_BasicSearch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}\n"), 0o644)

	tool := &GrepTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern":     "hello",
		"output_mode": "content",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "hello") {
		t.Errorf("expected 'hello' in output, got %q", out.Content)
	}
}

func TestGrep_FilesWithMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("func foo() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("func bar() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("func baz() {}\n"), 0o644)

	tool := &GrepTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern":     "func",
		"output_mode": "files_with_matches",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	// Should list files
	if !strings.Contains(out.Content, "a.go") {
		t.Errorf("expected a.go in output, got %q", out.Content)
	}
}

func TestGrep_NoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world\n"), 0o644)

	tool := &GrepTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "zzz_nonexistent_zzz",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "No matches") {
		t.Errorf("expected 'No matches', got %q", out.Content)
	}
}

func TestGrep_GlobFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("func main() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("func main() {}\n"), 0o644)

	tool := &GrepTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern":     "func",
		"glob":        "*.go",
		"output_mode": "files_with_matches",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "a.go") {
		t.Errorf("expected a.go, got %q", out.Content)
	}
	if strings.Contains(out.Content, "b.txt") {
		t.Error("b.txt should be filtered out by glob")
	}
}

func TestGrep_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("Hello World\nhello world\n"), 0o644)

	tool := &GrepTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern":     "HELLO",
		"-i":          true,
		"output_mode": "content",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	// Should match both lines
	lines := strings.Split(strings.TrimSpace(out.Content), "\n")
	if len(lines) < 2 {
		t.Errorf("expected 2 matches with case insensitive, got %d: %q", len(lines), out.Content)
	}
}

func TestGrep_HeadLimit(t *testing.T) {
	dir := t.TempDir()
	var content strings.Builder
	for i := 0; i < 20; i++ {
		content.WriteString("matching line\n")
	}
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content.String()), 0o644)

	tool := &GrepTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern":     "matching",
		"output_mode": "content",
		"head_limit":  float64(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	lines := strings.Split(strings.TrimSpace(out.Content), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines with head_limit, got %d", len(lines))
	}
}

func TestGrep_MissingPattern(t *testing.T) {
	tool := &GrepTool{CWD: "/tmp"}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing pattern")
	}
}

func TestGrepTool_OutputTruncation(t *testing.T) {
	dir := t.TempDir()

	// Create a file with many matching lines to produce >100K chars of output
	var content strings.Builder
	line := strings.Repeat("matching_pattern_abcdefghij", 5) + "\n" // ~130 chars per line
	for i := 0; i < 1000; i++ {
		content.WriteString(line)
	}
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(content.String()), 0o644)

	tool := &GrepTool{CWD: dir}
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern":     "matching_pattern",
		"output_mode": "content",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	// Verify the output was truncated and contains the indicator
	if !strings.Contains(out.Content, "truncated") {
		t.Fatal("expected output to be truncated")
	}
	if !strings.Contains(out.Content, "total characters") {
		t.Error("truncation message should include total character count")
	}

	// The actual content (before suffix) should be at the limit
	// Total output = grepMaxOutput + suffix length, which is fine
	suffixIdx := strings.Index(out.Content, "\n... (truncated")
	if suffixIdx > grepMaxOutput {
		t.Errorf("content before suffix should be at most %d chars, got %d", grepMaxOutput, suffixIdx)
	}
}
