package prompt

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxImportDepth = 5

// importPattern matches @path/to/file where path contains word chars, dots, slashes, dashes.
var importPattern = regexp.MustCompile(`@([\w./_-][\w./_-]*)`)

// ResolveImports resolves @path/to/file import directives in content.
// Paths are resolved relative to basePath (the directory containing the file).
// Returns the content with imports replaced by the referenced file contents.
// Imports inside fenced code blocks and inline backticks are skipped.
// Max recursion depth is 5.
func ResolveImports(content, basePath string) (string, error) {
	return resolveImportsRecursive(content, basePath, 0)
}

func resolveImportsRecursive(content, basePath string, depth int) (string, error) {
	if depth >= maxImportDepth {
		return content, nil
	}

	// Find all code block ranges to skip
	codeRanges := findCodeBlockRanges(content)

	// Find all matches with positions
	matches := importPattern.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	// Build result by processing matches in reverse order to preserve positions
	result := content
	for i := len(matches) - 1; i >= 0; i-- {
		start, end := matches[i][0], matches[i][1]

		// Skip if inside code block or inline backtick
		if isInCodeRange(start, codeRanges) || isInInlineCode(content, start) {
			continue
		}

		match := content[start:end]
		importPath := match[1:] // strip leading @

		// Resolve relative to basePath
		fullPath := filepath.Join(basePath, importPath)

		data, err := os.ReadFile(fullPath)
		if err != nil {
			// Silently skip missing files
			continue
		}

		imported := strings.TrimSpace(string(data))

		// Recursively resolve imports in the imported content
		importDir := filepath.Dir(fullPath)
		resolved, err := resolveImportsRecursive(imported, importDir, depth+1)
		if err != nil {
			resolved = imported
		}

		result = result[:start] + resolved + result[end:]
	}

	return result, nil
}

// codeRange represents a range of bytes that are inside a fenced code block.
type codeRange struct {
	start, end int
}

// findCodeBlockRanges finds all fenced code block ranges (```...```) in content.
func findCodeBlockRanges(content string) []codeRange {
	var ranges []codeRange
	fence := "```"
	pos := 0
	for {
		start := strings.Index(content[pos:], fence)
		if start < 0 {
			break
		}
		start += pos

		// Find closing fence
		searchFrom := start + len(fence)
		// Skip to next line for opening fence
		nl := strings.Index(content[searchFrom:], "\n")
		if nl >= 0 {
			searchFrom += nl + 1
		}

		end := strings.Index(content[searchFrom:], fence)
		if end < 0 {
			// Unclosed code block — treat rest as code
			ranges = append(ranges, codeRange{start, len(content)})
			break
		}
		end += searchFrom + len(fence)

		ranges = append(ranges, codeRange{start, end})
		pos = end
	}
	return ranges
}

// isInCodeRange returns true if pos falls within any fenced code block range.
func isInCodeRange(pos int, ranges []codeRange) bool {
	for _, r := range ranges {
		if pos >= r.start && pos < r.end {
			return true
		}
	}
	return false
}

// isInInlineCode returns true if pos is inside inline backtick code (`...`).
func isInInlineCode(content string, pos int) bool {
	// Scan the line for balanced backtick pairs
	lineStart := strings.LastIndex(content[:pos], "\n")
	if lineStart < 0 {
		lineStart = 0
	} else {
		lineStart++ // skip the \n
	}

	line := content[lineStart:]
	relPos := pos - lineStart

	inCode := false
	for i := 0; i < len(line) && i < relPos; i++ {
		if line[i] == '`' {
			inCode = !inCode
		}
	}
	return inCode
}
