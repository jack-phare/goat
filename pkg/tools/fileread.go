package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	gopdf "github.com/ledongthuc/pdf"
)

const (
	fileReadDefaultLimit   = 2000 // default max lines to read
	fileReadMaxLineLength  = 2000 // truncate lines longer than this
	fileReadMaxPDFPages    = 20   // max pages per PDF read
)

// FileReadTool reads file contents with line numbers.
type FileReadTool struct{}

func (f *FileReadTool) Name() string { return "Read" }

func (f *FileReadTool) Description() string {
	return `Reads a file from the local filesystem. You can access any file directly by using this tool.
Assume this tool is able to read all files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error will be returned.

Usage:
- The file_path parameter must be an absolute path, not a relative path
- By default, it reads up to 2000 lines starting from the beginning of the file
- You can optionally specify a line offset and limit (especially handy for long files), but it's recommended to read the whole file by not providing these parameters
- Any lines longer than 2000 characters will be truncated
- Results are returned using cat -n format, with line numbers starting at 1
- This tool allows Claude Code to read images (eg PNG, JPG, etc). When reading an image file the contents are presented visually as Claude Code is a multimodal LLM.
- This tool can read PDF files (.pdf). For large PDFs (more than 10 pages), you MUST provide the pages parameter to read specific page ranges (e.g., pages: "1-5"). Reading a large PDF without the pages parameter will fail. Maximum 20 pages per request.
- This tool can read Jupyter notebooks (.ipynb files) and returns all cells with their outputs, combining code, text, and visualizations.
- This tool can only read files, not directories. To read a directory, use an ls command via the Bash tool.
- You can call multiple tools in a single response. It is always better to speculatively read multiple potentially useful files in parallel.
- You will regularly be asked to read screenshots. If the user provides a path to a screenshot, ALWAYS use this tool to view the file at the path. This tool will work with all temporary file paths.
- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents.`
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
			"pages": map[string]any{
				"type":        "string",
				"description": "Page range for PDF files (e.g. \"1-5\", \"3\", \"10-20\"). Max 20 pages per request.",
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

	// Handle PDF files
	if strings.ToLower(filepath.Ext(filePath)) == ".pdf" {
		return f.readPDF(filePath, input)
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

// readPDF extracts text from a PDF file with optional page range.
func (f *FileReadTool) readPDF(filePath string, input map[string]any) (ToolOutput, error) {
	pdfFile, reader, err := gopdf.Open(filePath)
	if err != nil {
		return ToolOutput{Content: fmt.Sprintf("Error opening PDF: %s", err), IsError: true}, nil
	}
	defer pdfFile.Close()

	totalPages := reader.NumPage()
	if totalPages == 0 {
		return ToolOutput{Content: "(empty PDF)"}, nil
	}

	startPage, endPage := 1, totalPages

	if pagesStr, ok := input["pages"].(string); ok && pagesStr != "" {
		s, e, parseErr := parsePDFPageRange(pagesStr, totalPages)
		if parseErr != nil {
			return ToolOutput{Content: fmt.Sprintf("Error: %s", parseErr), IsError: true}, nil
		}
		startPage, endPage = s, e
	} else if totalPages > fileReadMaxPDFPages {
		return ToolOutput{
			Content: fmt.Sprintf("Error: PDF has %d pages (max %d). Use the 'pages' parameter to specify a page range (e.g. \"1-5\").", totalPages, fileReadMaxPDFPages),
			IsError: true,
		}, nil
	}

	pageCount := endPage - startPage + 1
	if pageCount > fileReadMaxPDFPages {
		return ToolOutput{
			Content: fmt.Sprintf("Error: requested %d pages (max %d per request)", pageCount, fileReadMaxPDFPages),
			IsError: true,
		}, nil
	}

	var b strings.Builder
	lineNum := 0
	for p := startPage; p <= endPage; p++ {
		page := reader.Page(p)
		if page.V.IsNull() {
			continue
		}

		text, extractErr := page.GetPlainText(nil)
		if extractErr != nil {
			b.WriteString(fmt.Sprintf("[Page %d: error extracting text: %s]\n", p, extractErr))
			continue
		}

		// Number each line within the page
		for _, line := range strings.Split(text, "\n") {
			lineNum++
			if len(line) > fileReadMaxLineLength {
				line = line[:fileReadMaxLineLength]
			}
			b.WriteString(fmt.Sprintf("%6d\t%s\n", lineNum, line))
		}
	}

	if b.Len() == 0 {
		return ToolOutput{Content: "(no text extracted from PDF)"}, nil
	}

	return ToolOutput{Content: strings.TrimRight(b.String(), "\n")}, nil
}

// parsePDFPageRange parses a page range string like "1-5", "3", or "10-20".
func parsePDFPageRange(pages string, totalPages int) (start, end int, err error) {
	pages = strings.TrimSpace(pages)

	if strings.Contains(pages, "-") {
		parts := strings.SplitN(pages, "-", 2)
		start, err = strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid page range start: %s", parts[0])
		}
		end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid page range end: %s", parts[1])
		}
	} else {
		start, err = strconv.Atoi(pages)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid page number: %s", pages)
		}
		end = start
	}

	if start < 1 {
		start = 1
	}
	if end > totalPages {
		end = totalPages
	}
	if start > end {
		return 0, 0, fmt.Errorf("invalid page range: %d-%d", start, end)
	}
	return start, end, nil
}
