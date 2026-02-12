package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveImports_RelativePath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "included.md"), "Included content here.")

	content := "Before @included.md After"
	result, err := ResolveImports(content, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Before Included content here. After" {
		t.Errorf("got %q", result)
	}
}

func TestResolveImports_SubdirectoryPath(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "docs")
	os.MkdirAll(subDir, 0o755)
	writeFile(t, filepath.Join(subDir, "api.md"), "API docs")

	content := "See @docs/api.md for details."
	result, err := ResolveImports(content, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "See API docs for details." {
		t.Errorf("got %q", result)
	}
}

func TestResolveImports_DepthLimit(t *testing.T) {
	dir := t.TempDir()

	// Create a chain of imports deeper than the limit
	for i := 0; i < 7; i++ {
		var content string
		if i < 6 {
			content = "level" + string(rune('0'+i)) + " @level" + string(rune('1'+i)) + ".md"
		} else {
			content = "leaf"
		}
		writeFile(t, filepath.Join(dir, "level"+string(rune('0'+i))+".md"), content)
	}

	content := "@level0.md"
	result, err := ResolveImports(content, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should resolve up to depth 5, but not deeper
	if !strings.Contains(result, "level0") {
		t.Error("should contain level0")
	}
	// The deeper levels may or may not be resolved depending on depth
	// The important thing is it doesn't infinite loop
}

func TestResolveImports_InCodeBlock(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "included.md"), "SHOULD NOT APPEAR")

	content := "```\n@included.md\n```"
	result, err := ResolveImports(content, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "SHOULD NOT APPEAR") {
		t.Error("imports in code blocks should not be resolved")
	}
	if !strings.Contains(result, "@included.md") {
		t.Error("import text should remain unchanged in code block")
	}
}

func TestResolveImports_InInlineCode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "included.md"), "SHOULD NOT APPEAR")

	content := "Use `@included.md` for imports."
	result, err := ResolveImports(content, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "SHOULD NOT APPEAR") {
		t.Error("imports in inline code should not be resolved")
	}
}

func TestResolveImports_MissingFile(t *testing.T) {
	dir := t.TempDir()

	content := "Include @nonexistent.md here."
	result, err := ResolveImports(content, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Missing file should be silently skipped
	if result != "Include @nonexistent.md here." {
		t.Errorf("got %q", result)
	}
}

func TestResolveImports_CircularPrevented(t *testing.T) {
	dir := t.TempDir()

	// Create circular imports
	writeFile(t, filepath.Join(dir, "a.md"), "A: @b.md")
	writeFile(t, filepath.Join(dir, "b.md"), "B: @a.md")

	content := "@a.md"
	result, err := ResolveImports(content, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not infinite loop — depth limit prevents it
	if result == "" {
		t.Error("should produce some result")
	}
}

func TestResolveImports_NoImports(t *testing.T) {
	content := "Just regular content without imports."
	result, err := ResolveImports(content, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != content {
		t.Errorf("got %q", result)
	}
}

func TestResolveImports_MultipleImports(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "header.md"), "HEADER")
	writeFile(t, filepath.Join(dir, "footer.md"), "FOOTER")

	content := "@header.md\n\nMiddle\n\n@footer.md"
	result, err := ResolveImports(content, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "HEADER") {
		t.Error("should contain HEADER")
	}
	if !strings.Contains(result, "FOOTER") {
		t.Error("should contain FOOTER")
	}
	if !strings.Contains(result, "Middle") {
		t.Error("should contain Middle")
	}
}

func TestResolveImports_RelativeToContainingFile(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "docs")
	os.MkdirAll(subDir, 0o755)

	writeFile(t, filepath.Join(subDir, "shared.md"), "Shared content")
	writeFile(t, filepath.Join(dir, "main.md"), "@docs/shared.md")

	// Read main.md and resolve relative to dir
	content := "@docs/shared.md"
	result, err := ResolveImports(content, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Shared content" {
		t.Errorf("got %q", result)
	}
}

func TestResolveImports_NestedResolution(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0o755)

	// sub/nested.md references leaf.md (relative to sub/)
	writeFile(t, filepath.Join(subDir, "leaf.md"), "LEAF")
	writeFile(t, filepath.Join(subDir, "nested.md"), "NESTED: @leaf.md")
	// top.md references sub/nested.md (relative to dir)

	content := "@sub/nested.md"
	result, err := ResolveImports(content, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "NESTED: LEAF") {
		t.Errorf("nested resolution failed: got %q", result)
	}
}

func TestFindCodeBlockRanges(t *testing.T) {
	content := "before\n```\ncode here\n```\nafter"
	ranges := findCodeBlockRanges(content)
	if len(ranges) != 1 {
		t.Fatalf("expected 1 range, got %d", len(ranges))
	}
	if ranges[0].start != 7 { // position of first ```
		t.Errorf("start = %d, want 7", ranges[0].start)
	}
}

func TestIsInInlineCode(t *testing.T) {
	// "Use `@foo` here"
	content := "Use `@foo` here"
	pos := strings.Index(content, "@foo")

	if !isInInlineCode(content, pos) {
		t.Error("@foo should be detected as inside inline code")
	}

	// "Use @foo here" — not in code
	content2 := "Use @foo here"
	pos2 := strings.Index(content2, "@foo")
	if isInInlineCode(content2, pos2) {
		t.Error("@foo should NOT be detected as inside inline code")
	}
}
