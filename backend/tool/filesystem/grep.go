package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/furisto/construct/backend/tool/base"
)

type GrepInput struct {
	Query          string `json:"query"`
	Path           string `json:"path"`
	IncludePattern string `json:"include_pattern"`
	ExcludePattern string `json:"exclude_pattern"`
	CaseSensitive  bool   `json:"case_sensitive"`
	MaxResults     int    `json:"max_results"`
	Context        int    `json:"context"`
}

type GrepMatch struct {
	FilePath   string `json:"file_path"`
	Value      string `json:"value"`
}

type GrepResult struct {
	Matches          []GrepMatch `json:"matches"`
	TotalMatches     int         `json:"total_matches"`
	TruncatedMatches int         `json:"truncated_matches"`
	SearchedFiles    int         `json:"searched_files"`
}

func Grep(ctx context.Context, input *GrepInput) (*GrepResult, error) {
	if input.Query == "" || input.Path == "" {
		return nil, base.NewCustomError("query and path are required", []string{
			"Provide both a search query and a path to search in",
		})
	}

	if input.MaxResults == 0 {
		input.MaxResults = 50
	}

	if input.Context == 0 {
		input.Context = 2
	}

	if isRipgrepAvailable() {
		return performRipgrep(ctx, input)
	}

	return performRegularGrep(ctx, input)
}

func isRipgrepAvailable() bool {
	_, err := exec.LookPath("rg")
	return err == nil
}

func performRipgrep(ctx context.Context, input *GrepInput) (*GrepResult, error) {
	args := []string{
		"--json",
		"--line-number",
		"--with-filename",
		"--context", strconv.Itoa(input.Context),
	}

	if !input.CaseSensitive {
		args = append(args, "--ignore-case")
	}

	if input.IncludePattern != "" {
		args = append(args, "--glob", input.IncludePattern)
	}

	if input.ExcludePattern != "" {
		args = append(args, "--glob", "!"+input.ExcludePattern)
	}

	args = append(args, input.Query, input.Path)

	cmd := exec.CommandContext(ctx, "rg", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// No matches found, return empty result
			return &GrepResult{
				Matches:       []GrepMatch{},
				TotalMatches:  0,
				TruncatedMatches: 0,
				SearchedFiles: 0,
			}, nil
		}
		return nil, base.NewCustomError("ripgrep error", []string{
			"Check that your regex pattern is valid",
			"Verify that the search path exists and is accessible",
		}, "error", err)
	}

	return parseRipgrepOutput(string(output), input.MaxResults, input.Context)
}

func performRegularGrep(ctx context.Context, input *GrepInput) (*GrepResult, error) {
	args := []string{
		"-r",
		"-n",
		"-H",
		"-C", strconv.Itoa(input.Context),
	}

	if !input.CaseSensitive {
		args = append(args, "-i")
	}

	if input.IncludePattern != "" {
		args = append(args, "--include="+input.IncludePattern)
	}

	if input.ExcludePattern != "" {
		args = append(args, "--exclude="+input.ExcludePattern)
	}

	args = append(args, input.Query, input.Path)

	cmd := exec.CommandContext(ctx, "grep", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// No matches found, return empty result
			return &GrepResult{
				Matches:       []GrepMatch{},
				TotalMatches:  0,
				TruncatedMatches: 0,
				SearchedFiles: 0,
			}, nil
		}
		return nil, base.NewCustomError("grep error", []string{
			"Check that your regex pattern is valid",
			"Verify that the search path exists and is accessible",
		}, "error", err)
	}

	return parseGrepOutput(string(output), input.MaxResults, input.Context)
}

type ripgrepEntry struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		LineNumber int `json:"line_number"`
		Lines      struct {
			Text string `json:"text"`
		} `json:"lines"`
		Submatches []struct {
			Match struct {
				Text string `json:"text"`
			} `json:"match"`
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"submatches"`
	} `json:"data"`
}

type fileGroup struct {
	filePath string
	entries  []ripgrepEntry
}

type grepEntry struct {
	filePath string
	lineNum  int
	content  string
	isMatch  bool
}

func parseRipgrepOutput(output string, maxResults int, context int) (*GrepResult, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	matches := []GrepMatch{}

	fileGroups := make(map[string]*fileGroup)
	var currentFile string

	for _, line := range lines {
		if line == "" {
			continue
		}

		var entry ripgrepEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "begin":
			currentFile = entry.Data.Path.Text
			if fileGroups[currentFile] == nil {
				fileGroups[currentFile] = &fileGroup{
					filePath: currentFile,
					entries:  []ripgrepEntry{},
				}
			}
		case "match", "context":
			if currentFile != "" && fileGroups[currentFile] != nil {
				fileGroups[currentFile].entries = append(fileGroups[currentFile].entries, entry)
			}
		case "end":
			// Process the completed file group
			if currentFile != "" && fileGroups[currentFile] != nil {
				fileMatches := processFileGroup(fileGroups[currentFile], context)
				matches = append(matches, fileMatches...)
			}
			currentFile = ""
		}
	}

	totalMatches := len(matches)
	skippedMatches := max(0, totalMatches-maxResults)
	if skippedMatches > 0 {
		matches = matches[:maxResults]
	}

	return &GrepResult{
		Matches:          matches,
		TotalMatches:     totalMatches,
		TruncatedMatches: skippedMatches,
		SearchedFiles:    len(fileGroups),
	}, nil
}

func processFileGroup(group *fileGroup, context int) []GrepMatch {
	if len(group.entries) == 0 {
		return []GrepMatch{}
	}

	// Group entries that are within context distance
	var entryGroups [][]ripgrepEntry
	currentGroup := []ripgrepEntry{group.entries[0]}

	for i := 1; i < len(group.entries); i++ {
		prevLine := group.entries[i-1].Data.LineNumber
		currentLine := group.entries[i].Data.LineNumber
		
		// If gap between lines is <= context*2, add to current group
		if currentLine-prevLine <= context*2 {
			currentGroup = append(currentGroup, group.entries[i])
		} else {
			// Start new group
			entryGroups = append(entryGroups, currentGroup)
			currentGroup = []ripgrepEntry{group.entries[i]}
		}
	}
	entryGroups = append(entryGroups, currentGroup)

	// Convert each group to a GrepMatch
	matches := []GrepMatch{}
	for _, entryGroup := range entryGroups {
		// Check if group contains at least one match
		hasMatch := false
		for _, entry := range entryGroup {
			if entry.Type == "match" {
				hasMatch = true
				break
			}
		}
		
		if !hasMatch {
			continue
		}

		// Format all entries in the group
		var formattedLines []string
		for _, entry := range entryGroup {
			lineNum := entry.Data.LineNumber
			text := entry.Data.Lines.Text
			
			if entry.Type == "match" {
				formattedLines = append(formattedLines, fmt.Sprintf(":%d:%s", lineNum, text))
			} else {
				formattedLines = append(formattedLines, fmt.Sprintf("-%d-%s", lineNum, text))
			}
		}

		value := strings.Join(formattedLines, "")
		matches = append(matches, GrepMatch{
			FilePath: group.filePath,
			Value:    value,
		})
	}

	return matches
}

func parseGrepOutput(output string, maxResults int, context int) (*GrepResult, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	searchedFiles := make(map[string]bool)

	fileGroups := make(map[string][]grepEntry)

	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		// Parse match line (contains :) or context line (contains -)
		var parts []string
		var isMatch bool

		if strings.Contains(line, ":") && !strings.Contains(line, ":-") {
			parts = strings.SplitN(line, ":", 3)
			isMatch = true
		} else {
			parts = strings.SplitN(line, "-", 3)
			isMatch = false
		}

		if len(parts) < 3 {
			continue
		}

		filePath := parts[0]
		lineNum, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		content := parts[2]

		if isMatch {
			searchedFiles[filePath] = true
		}

		fileGroups[filePath] = append(fileGroups[filePath], grepEntry{
			filePath: filePath,
			lineNum:  lineNum,
			content:  content,
			isMatch:  isMatch,
		})
	}

	// Process each file's entries and group by proximity
	var allMatches []GrepMatch
	for _, entries := range fileGroups {
		fileMatches := processGrepEntries(entries, context)
		allMatches = append(allMatches, fileMatches...)
	}

	// Apply maxResults limit
	totalMatches := len(allMatches)
	skippedMatches := max(0, totalMatches-maxResults)
	if skippedMatches > 0 {
		allMatches = allMatches[:maxResults]
	}

	return &GrepResult{
		Matches:          allMatches,
		TotalMatches:     totalMatches,
		TruncatedMatches: skippedMatches,
		SearchedFiles:    len(searchedFiles),
	}, nil
}

func processGrepEntries(entries []grepEntry, context int) []GrepMatch {
	if len(entries) == 0 {
		return []GrepMatch{}
	}

	// Sort entries by line number
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].lineNum > entries[j].lineNum {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Group entries that are within context distance
	var entryGroups [][]grepEntry
	currentGroup := []grepEntry{entries[0]}

	for i := 1; i < len(entries); i++ {
		prevLine := entries[i-1].lineNum
		currentLine := entries[i].lineNum

		// If gap between lines is <= context*2, add to current group
		if currentLine-prevLine <= context*2 {
			currentGroup = append(currentGroup, entries[i])
		} else {
			// Start new group
			entryGroups = append(entryGroups, currentGroup)
			currentGroup = []grepEntry{entries[i]}
		}
	}
	entryGroups = append(entryGroups, currentGroup)

	// Convert each group to a GrepMatch
	matches := []GrepMatch{}
	for _, entryGroup := range entryGroups {
		// Check if group contains at least one match
		hasMatch := false
		for _, entry := range entryGroup {
			if entry.isMatch {
				hasMatch = true
				break
			}
		}

		if !hasMatch {
			continue
		}

		// Format all entries in the group
		var formattedLines []string
		for _, entry := range entryGroup {
			if entry.isMatch {
				formattedLines = append(formattedLines, fmt.Sprintf(":%d:%s", entry.lineNum, entry.content))
			} else {
				formattedLines = append(formattedLines, fmt.Sprintf("-%d-%s", entry.lineNum, entry.content))
			}
		}

		value := strings.Join(formattedLines, "")
		matches = append(matches, GrepMatch{
			FilePath: entryGroup[0].filePath,
			Value:    value,
		})
	}

	return matches
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
