package filesystem

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	diff "github.com/sourcegraph/go-diff-patch"
	"github.com/spf13/afero"

	"github.com/furisto/construct/backend/tool/base"
)

type EditFileInput struct {
	Path  string     `json:"path"`
	Diffs []DiffPair `json:"diffs"`
}

type DiffPair struct {
	Old string `json:"old"`
	New string `json:"new"`
}

type DiffValidationError struct {
	DiffIndex    int    `json:"diff_index"`
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
	SuggestedFix string `json:"suggested_fix,omitempty"`
}

func (e *DiffValidationError) Error() string {
	return fmt.Sprintf("diff_index: %d, error_type: %s, error_message: %s, suggested_fix: %s", e.DiffIndex, e.ErrorType, e.ErrorMessage, e.SuggestedFix)
}

type ConflictWarning struct {
	Edit1Index   int    `json:"edit1_index"`
	Edit2Index   int    `json:"edit2_index"`
	ConflictType string `json:"conflict_type"`
	Message      string `json:"message"`
}

type PatchInfo struct {
	Patch        string `json:"patch"`
	LinesAdded   int    `json:"lines_added"`
	LinesRemoved int    `json:"lines_removed"`
}

type EditFileResult struct {
	Success              bool                  `json:"success"`
	Path                 string                `json:"path"`
	ReplacementsMade     int                   `json:"replacements_made"`
	ExpectedReplacements int                   `json:"expected_replacements"`
	FailureReason        string                `json:"failure_reason,omitempty"`
	ValidationErrors     []DiffValidationError `json:"validation_errors,omitempty"`
	ConflictWarnings     []ConflictWarning     `json:"conflict_warnings,omitempty"`
	PatchInfo            PatchInfo             `json:"patch_info,omitempty"`
}

func EditFile(fsys afero.Fs, input *EditFileInput) (*EditFileResult, error) {
	if !filepath.IsAbs(input.Path) {
		return nil, base.NewError(base.PathIsNotAbsolute, "path", input.Path)
	}
	path := input.Path

	// Check if file exists and is not a directory
	stat, err := fsys.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, base.NewError(base.FileNotFound, "path", path)
		}
		return nil, base.NewCustomError("error accessing file", []string{
			"Verify that you have the permission to access the file",
		}, "path", path, "error", err)
	}

	if stat.IsDir() {
		return nil, base.NewError(base.PathIsDirectory, "path", path)
	}

	// Read file content
	content, err := afero.ReadFile(fsys, path)
	if err != nil {
		slog.Error("failed to read file for editing", "path", path, "error", err)
		return nil, base.NewCustomError("error reading file", []string{
			"Verify that you have the permission to read the file",
		}, "path", path, "error", err)
	}

	originalContent := string(content)
	expectedReplacements := len(input.Diffs)

	conflictWarnings := detectConflicts(input.Diffs)

	newContent, replacementsMade, validationErrors := processEdits(originalContent, input.Diffs)
	if len(validationErrors) > 0 {
		slog.Warn("edit validation failed", "path", path, "error_count", len(validationErrors))
		return nil, base.NewCustomError("validation failed", []string{
			"Please fix the validation errors and try again",
		}, "path", path, "validation_errors", validationErrors)
	}

	var patchInfo PatchInfo
	if newContent != originalContent {
		filename := filepath.Base(path)
		patchInfo.Patch = diff.GeneratePatch(filename, originalContent, newContent)
		patchInfo.LinesAdded, patchInfo.LinesRemoved = parseDiffStats(patchInfo.Patch)

		err = afero.WriteFile(fsys, path, []byte(newContent), stat.Mode())
		if err != nil {
			slog.Error("failed to write file after edits", "path", path, "error", err)
			return nil, base.NewCustomError("error writing file", []string{
				"Verify that you have the permission to write to the file",
			}, "path", path, "error", err)
		}

		slog.Debug("file edits applied", "path", path, "replacements", replacementsMade, "expected", expectedReplacements, "lines_added", patchInfo.LinesAdded, "lines_removed", patchInfo.LinesRemoved)
	} else {
		slog.Debug("no changes needed", "path", path, "expected_replacements", expectedReplacements)
	}

	success := replacementsMade == expectedReplacements
	if !success {
		slog.Warn("edit partial success", "path", path, "replacements", replacementsMade, "expected", expectedReplacements)
	}

	return &EditFileResult{
		Success:              success,
		Path:                 path,
		ReplacementsMade:     replacementsMade,
		ExpectedReplacements: expectedReplacements,
		ConflictWarnings:     conflictWarnings,
		PatchInfo:            patchInfo,
	}, nil
}

// detectConflicts analyzes potential conflicts between multiple edits
func detectConflicts(diffs []DiffPair) []ConflictWarning {
	var warnings []ConflictWarning

	for i := 0; i < len(diffs)-1; i++ {
		for j := i + 1; j < len(diffs); j++ {
			edit1 := diffs[i]
			edit2 := diffs[j]

			// Skip empty diffs
			if (edit1.Old == "" && edit1.New == "") || (edit2.Old == "" && edit2.New == "") {
				continue
			}

			// Check if edit j depends on the result of edit i
			if edit2.Old != "" && edit1.New != "" && strings.Contains(edit2.Old, edit1.New) {
				warnings = append(warnings, ConflictWarning{
					Edit1Index:   i + 1,
					Edit2Index:   j + 1,
					ConflictType: "dependency",
					Message:      fmt.Sprintf("Edit %d depends on result of edit %d", j+1, i+1),
				})
			}

			// Check if both edits try to modify overlapping text regions
			if edit1.Old != "" && edit2.Old != "" {
				// Simple overlap detection: check if one old text contains the other
				if strings.Contains(edit1.Old, edit2.Old) || strings.Contains(edit2.Old, edit1.Old) {
					warnings = append(warnings, ConflictWarning{
						Edit1Index:   i + 1,
						Edit2Index:   j + 1,
						ConflictType: "overlap",
						Message:      fmt.Sprintf("Edits %d and %d may affect overlapping text regions", i+1, j+1),
					})
				}

				// Check if both edits target the same exact text
				if edit1.Old == edit2.Old {
					warnings = append(warnings, ConflictWarning{
						Edit1Index:   i + 1,
						Edit2Index:   j + 1,
						ConflictType: "duplicate_target",
						Message:      fmt.Sprintf("Edits %d and %d target the same text", i+1, j+1),
					})
				}
			}

			// Check for potential line-level conflicts by examining line boundaries
			if edit1.Old != "" && edit2.Old != "" {
				edit1Lines := strings.Split(edit1.Old, "\n")
				edit2Lines := strings.Split(edit2.Old, "\n")

				// If both edits span multiple lines and share common line content
				if len(edit1Lines) > 1 && len(edit2Lines) > 1 {
					for _, line1 := range edit1Lines {
						for _, line2 := range edit2Lines {
							if strings.TrimSpace(line1) != "" && strings.TrimSpace(line1) == strings.TrimSpace(line2) {
								warnings = append(warnings, ConflictWarning{
									Edit1Index:   i + 1,
									Edit2Index:   j + 1,
									ConflictType: "line_overlap",
									Message:      fmt.Sprintf("Edits %d and %d may affect the same line", i+1, j+1),
								})
								break
							}
						}
					}
				}
			}
		}
	}

	return warnings
}

func processEdits(fileContent string, diffs []DiffPair) (string, int, []DiffValidationError) {
	var validationErrors []DiffValidationError
	workingContent := fileContent
	replacementsMade := 0

	for i, diff := range diffs {
		if diff.Old == "" && diff.New == "" {
			continue
		}

		// Normalize escape sequences in diff.Old before any matching
		// This converts actual newlines/tabs inside strings to their literal forms (\\n, \\t)
		// while preserving real line breaks between code lines
		// diffs[i].Old = normalizeEscapesInString(diff.Old)
		// diffs[i].New = normalizeEscapesInString(diff.New)
		// diff.Old = diffs[i].Old

		if diff.Old == diff.New {
			validationErrors = append(validationErrors, DiffValidationError{
				DiffIndex:    i,
				ErrorType:    "no_op",
				ErrorMessage: "old and new text are identical",
				SuggestedFix: "Remove this diff or provide different new text",
			})

			continue
		}

		if diff.Old == "" {
			workingContent = workingContent + diff.New
			replacementsMade++
			continue
		}

		if strings.Contains(workingContent, diff.Old) {
			// Exact match found
			if diff.New == "" {
				// Delete operation
				workingContent = strings.Replace(workingContent, diff.Old, "", 1)
			} else {
				// Replace operation
				workingContent = strings.Replace(workingContent, diff.Old, diff.New, 1)
			}
			replacementsMade++

		} else {
			startIdx, endIdx := lineTrimmedFallbackMatch(workingContent, diff.Old, 0)
			if startIdx != -1 && endIdx != -1 {
				// Fallback match found
				if diff.New == "" {
					// Delete operation
					workingContent = workingContent[:startIdx] + workingContent[endIdx:]
				} else {
					// Replace operation
					workingContent = workingContent[:startIdx] + diff.New + workingContent[endIdx:]
				}
				replacementsMade++
			} else {
				validationErrors = append(validationErrors, DiffValidationError{
					DiffIndex:    i,
					ErrorType:    "not_found",
					ErrorMessage: "old text not found in file",
					SuggestedFix: "Check that the old text exactly matches the file content, including whitespace and indentation",
				})
			}
		}
	}

	return workingContent, replacementsMade, validationErrors
}

func lineTrimmedFallbackMatch(originalContent, searchContent string, startIndex int) (int, int) {
	originalLines := strings.Split(originalContent, "\n")
	searchLines := strings.Split(searchContent, "\n")

	// Trim trailing empty line if exists (from the trailing \n in searchContent)
	if len(searchLines) > 0 && searchLines[len(searchLines)-1] == "" {
		searchLines = searchLines[:len(searchLines)-1]
	}

	if len(searchLines) == 0 {
		return -1, -1
	}

	// Find the line number where startIndex falls
	startLineNum := 0
	currentIndex := 0
	for currentIndex < startIndex && startLineNum < len(originalLines) {
		currentIndex += len(originalLines[startLineNum]) + 1 // +1 for \n
		startLineNum++
	}

	// For each possible starting position in original content
	for i := startLineNum; i <= len(originalLines)-len(searchLines); i++ {
		matches := true

		// Try to match all search lines from this position
		for j := 0; j < len(searchLines); j++ {
			originalTrimmed := strings.TrimSpace(originalLines[i+j])
			searchTrimmed := strings.TrimSpace(searchLines[j])

			if originalTrimmed != searchTrimmed {
				matches = false
				break
			}
		}

		// If we found a match, calculate the exact character positions
		if matches {
			// Find start character index
			matchStartIndex := 0
			for k := 0; k < i; k++ {
				matchStartIndex += len(originalLines[k]) + 1 // +1 for \n
			}

			// Find end character index
			matchEndIndex := matchStartIndex
			for k := 0; k < len(searchLines); k++ {
				matchEndIndex += len(originalLines[i+k]) + 1 // +1 for \n
			}

			return matchStartIndex, matchEndIndex
		}
	}

	return -1, -1
}

func parseDiffStats(patch string) (linesAdded, linesRemoved int) {
	if patch == "" {
		return 0, 0
	}

	lines := strings.Split(patch, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		switch line[0] {
		case '+':
			// Skip the +++ header line
			if !strings.HasPrefix(line, "+++") {
				linesAdded++
			}
		case '-':
			// Skip the --- header line
			if !strings.HasPrefix(line, "---") {
				linesRemoved++
			}
		}
	}

	return linesAdded, linesRemoved
}

func normalizeEscapesInString(s string) string {
	var result strings.Builder
	inString := false
	var stringDelim rune
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		ch := runes[i]

		// Handle escape sequences (backslash followed by another char)
		if ch == '\\' && i+1 < len(runes) && inString {
			// Inside a string, backslash escapes the next character
			// Copy both the backslash and next char as-is (don't process further)
			result.WriteRune(ch)
			result.WriteRune(runes[i+1])
			i += 2
			continue
		}

		// Handle string delimiters
		if ch == '"' || ch == '\'' || ch == '`' {
			if !inString {
				inString = true
				stringDelim = ch
			} else if ch == stringDelim {
				inString = false
			}
			result.WriteRune(ch)
			i++
			continue
		}

		// Convert actual escape characters to literals ONLY inside strings
		if inString {
			if ch == '\n' {
				result.WriteString("\\n")
			} else if ch == '\t' {
				result.WriteString("\\t")
			} else if ch == '\r' {
				result.WriteString("\\r")
			} else {
				result.WriteRune(ch)
			}
		} else {
			result.WriteRune(ch)
		}
		i++
	}

	return result.String()
}
