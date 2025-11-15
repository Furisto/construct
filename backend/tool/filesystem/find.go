package filesystem

import (
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/spf13/afero"
)

type FindFileInput struct {
	Pattern        string `json:"pattern"`
	Path           string `json:"path"`
	ExcludePattern string `json:"exclude_pattern"`
	MaxResults     int    `json:"max_results"`
}

type FindFileResult struct {
	Files          []string `json:"files"`
	TotalFiles     int      `json:"total_files"`
	TruncatedCount int      `json:"truncated_count"`
}

func FindFile(fsys afero.Fs, input *FindFileInput) (*FindFileResult, error) {
	if input.Pattern == "" || input.Path == "" {
		return nil, base.NewCustomError("pattern and path are required", []string{
			"Please provide a valid glob pattern and absolute path",
		})
	}

	if !filepath.IsAbs(input.Path) {
		return nil, base.NewError(base.PathIsNotAbsolute, "path", input.Path)
	}

	if isRipgrepAvailable() {
		return performRipgrepFind(input)
	}

	return performDoublestarFind(fsys, input)
}

func performRipgrepFind(input *FindFileInput) (*FindFileResult, error) {
	args := []string{
		"--files",
		"--null",
		"--glob", input.Pattern,
	}

	if input.ExcludePattern != "" {
		args = append(args, "--glob", "!"+input.ExcludePattern)
	}

	args = append(args, input.Path)

	cmd := exec.Command("rg", args...)
	output, err := cmd.Output()
	if err != nil {
		// no matching files
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return &FindFileResult{
				Files:          []string{},
				TotalFiles:     0,
				TruncatedCount: 0,
			}, nil
		}
		slog.Error("ripgrep failed", "pattern", input.Pattern, "path", input.Path, "error", err)
		return nil, fmt.Errorf("ripgrep error: %v", err)
	}

	outputStr := strings.TrimRight(string(output), "\x00")
	var filePaths []string
	if outputStr != "" {
		filePaths = strings.Split(outputStr, "\x00")
	}

	files := []string{}
	for _, filePath := range filePaths {
		if filePath != "" {
			if len(files) >= input.MaxResults {
				totalMatches := len(filePaths)
				truncated := totalMatches - len(files)
				slog.Debug("ripgrep find completed", "pattern", input.Pattern, "path", input.Path, "results", len(files), "truncated", truncated)
				return &FindFileResult{
					Files:          files,
					TotalFiles:     len(files),
					TruncatedCount: truncated,
				}, nil
			}
			files = append(files, filePath)
		}
	}

	slog.Debug("ripgrep find completed", "pattern", input.Pattern, "path", input.Path, "results", len(files), "truncated", 0)
	return &FindFileResult{
		Files:          files,
		TotalFiles:     len(files),
		TruncatedCount: 0,
	}, nil
}

func performDoublestarFind(fsys afero.Fs, input *FindFileInput) (*FindFileResult, error) {
	searchPattern := filepath.Join(input.Path, input.Pattern)
	validFiles := []string{}

	matches, err := doublestar.Glob(afero.NewIOFS(fsys), searchPattern)
	if err != nil {
		slog.Warn("glob pattern error", "pattern", input.Pattern, "path", input.Path, "error", err)
		return nil, base.NewCustomError("glob pattern error", []string{
			"Check that your glob pattern is valid",
		}, "pattern", input.Pattern, "error", err)
	}

	// First pass: collect all valid files that match criteria
	for _, match := range matches {
		if stat, err := fsys.Stat(match); err == nil && !stat.IsDir() {
			if input.ExcludePattern != "" {
				excluded, err := doublestar.Match(input.ExcludePattern, match)
				if err == nil && excluded {
					continue
				}
			}
			validFiles = append(validFiles, match)
		}
	}

	// Second pass: limit results and calculate truncation
	var files []string
	truncatedCount := 0
	if len(validFiles) > input.MaxResults {
		files = validFiles[:input.MaxResults]
		truncatedCount = len(validFiles) - input.MaxResults
	} else {
		files = validFiles
	}

	slog.Debug("doublestar find completed", "pattern", input.Pattern, "path", input.Path, "results", len(files), "truncated", truncatedCount)
	return &FindFileResult{
		Files:          files,
		TotalFiles:     len(files),
		TruncatedCount: truncatedCount,
	}, nil
}
