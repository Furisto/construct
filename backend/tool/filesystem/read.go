package filesystem

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/furisto/construct/backend/tool/base"
)

type ReadFileInput struct {
	Path      string `json:"path"`
	StartLine *int   `json:"start_line,omitempty"` // 1-based, inclusive
	EndLine   *int   `json:"end_line,omitempty"`   // 1-based, inclusive
}

func (input *ReadFileInput) Validate() error {
	if input.Path == "" {
		return base.NewCustomError("path is required", []string{
			"Please provide a valid path to the file you want to read",
		})
	}

	if !filepath.IsAbs(input.Path) {
		return base.NewCustomError("path must be absolute", []string{
			"Please provide a valid absolute path to the file you want to read",
		})
	}

	if input.StartLine != nil && *input.StartLine < 1 {
		return base.NewCustomError("start_line must be positive", []string{
			"Please provide a start_line value of 1 or greater",
		})
	}
	if input.EndLine != nil && *input.EndLine < 1 {
		return base.NewCustomError("end_line must be positive", []string{
			"Please provide an end_line value of 1 or greater",
		})
	}
	if input.StartLine != nil && input.EndLine != nil && *input.StartLine > *input.EndLine {
		return base.NewCustomError("start_line must be less than or equal to end_line", []string{
			"Please ensure start_line <= end_line",
		})
	}

	return nil
}

type ReadFileResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func ReadFile(fsys afero.Fs, input *ReadFileInput) (*ReadFileResult, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	path := input.Path

	stat, err := fsys.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("file not found", "path", path)
			return nil, base.NewError(base.FileNotFound, "path", path)
		}
		if os.IsPermission(err) {
			slog.Warn("permission denied reading file", "path", path)
			return nil, base.NewError(base.PermissionDenied, "path", path)
		}
		slog.Error("failed to stat file", "path", path, "error", err)
		return nil, base.NewError(base.CannotStatFile, "path", path)
	}

	if stat.IsDir() {
		return nil, base.NewError(base.PathIsDirectory, "path", path)
	}

	file, err := fsys.Open(path)
	if err != nil {
		slog.Error("failed to open file", "path", path, "error", err)
		return nil, base.NewCustomError("error reading file", []string{
			"Verify that you have the permission to read the file",
		}, "path", path, "error", err)
	}
	defer file.Close()

	// All reading uses range logic (entire file is just range from 1 to end)
	result, err := readFileRange(path, file, input.StartLine, input.EndLine)
	if err != nil {
		return nil, err
	}

	slog.Debug("file read successfully", "path", path, "content_size", len(result.Content), "has_range", input.StartLine != nil || input.EndLine != nil)
	return result, nil
}

func readFileRange(path string, file afero.File, startLine, endLine *int) (*ReadFileResult, error) {
	scanner := bufio.NewScanner(file)
	var builder strings.Builder

	start := 1
	if startLine != nil {
		start = *startLine
	}

	readToEnd := endLine == nil
	end := 0
	if !readToEnd {
		end = *endLine
	}

	currentLine := 1
	linesRead := 0
	contentStarted := false
	linesAfterRange := 0

	if start > 1 {
		builder.WriteString(fmt.Sprintf("// skipped %d lines", start-1))
		contentStarted = true
	}

	for scanner.Scan() {
		line := scanner.Text()

		if currentLine >= start && (readToEnd || currentLine <= end) {
			if contentStarted {
				builder.WriteByte('\n')
			}
			// builder.WriteString(strconv.Itoa(currentLine))
			// builder.WriteString(": ")
			builder.WriteString(line)
			linesRead++
			contentStarted = true
		} else if !readToEnd && currentLine > end {
			linesAfterRange++
		}

		currentLine++
	}

	if err := scanner.Err(); err != nil {
		return nil, base.NewCustomError("error reading file", []string{
			"Verify that you have the permission to read the file",
		}, "path", path, "error", err)
	}

	// Add remaining lines comment if we're not reading to the end
	if !readToEnd {
		if contentStarted {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("// %d lines remaining", linesAfterRange))
	}

	var content string
	if linesRead == 0 {
		content = ""
	} else {
		content = builder.String()
	}

	return &ReadFileResult{
		Path:    path,
		Content: content,
	}, nil
}
