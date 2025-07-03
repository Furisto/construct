package tool

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/furisto/construct/backend/tool/codeact"
	"github.com/grafana/sobek"
	"github.com/spf13/afero"
)

const findFileDescription = `
## Description
Finds files matching a glob pattern using ripgrep for optimal performance when available, falling back to filesystem walking with doublestar. This tool is designed for discovering files by name patterns rather than content, making it ideal for locating specific files, exploring project structure, or finding files of certain types.

## Parameters
- **pattern** (string, required): Glob pattern to match files against (e.g., "*.js", "**/*.go", "test*.py"). Supports standard glob patterns including wildcards (* and ?) and recursive patterns (**).
- **path** (string, required): Absolute path to the directory to search within. Forward slashes (/) work on all platforms.
- **exclude_pattern** (string, optional): Glob pattern for files to exclude from results. Useful for ignoring build artifacts, dependencies, or other irrelevant files.
- **max_results** (number, optional): Maximum number of results to return. Defaults to 50 to prevent overwhelming output.

## Expected Output
Returns an object containing the matching file paths:
%[1]s
{
  "files": [
    "/path/to/matching/file1.js",
    "/path/to/matching/file2.js",
    "/path/to/nested/dir/file3.js"
  ],
  "total_files": 3,
  "truncated_count": 0
}
%[1]s

**Details**
- **files**: Array of absolute file paths that matched the glob pattern
- **total_files**: Total number of files that matched the pattern and are included in the results
- **truncated_count**: Number of additional matching files that were found but excluded from results due to max_results limit. 0 indicates no truncation occurred.

## IMPORTANT USAGE NOTES
- **Pattern Specificity**: Be as specific as possible with your patterns to get relevant results
  %[1]s
  // Good: Find all React components
  find_file({
    pattern: "**/*Component.jsx",
    path: "/workspace/src"
  })
  
  // Better: Find components in specific directory
  find_file({
    pattern: "*Component.jsx", 
    path: "/workspace/src/components"
  })
  %[1]s
- **Performance Considerations**: Use specific paths and patterns for faster results
- **Exclude Patterns**: Use exclude patterns to filter out unwanted files
  %[1]s
  find_file({
    pattern: "*.js",
    path: "/workspace/project",
    exclude_pattern: "**/node_modules/**"
  })
  %[1]s
- **Path Format**: Always use absolute paths starting with "/"

## When to use
- **File Discovery**: When you need to find files by name patterns across a project
- **Project Exploration**: When exploring unfamiliar codebases to understand structure
- **Type-specific Searches**: When looking for all files of a certain type (e.g., all .json config files)
- **Template/Component Finding**: When locating specific templates, components, or modules
- **Build Artifact Location**: When finding generated files or build outputs

## Usage Examples

### Find all JavaScript files
%[1]s
find_file({
  pattern: "**/*.js",
  path: "/workspace/project/src",
  exclude_pattern: "**/__tests__/**"
})
%[1]s

### Find configuration files
%[1]s
find_file({
  pattern: "*.{json,yaml,yml}",
  path: "/workspace/project",
  max_results: 50
})
%[1]s

### Find test files
%[1]s
find_file({
  pattern: "**/*test.go",
  path: "/workspace/go-project"
})
%[1]s
`

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

func NewFindFileTool() codeact.Tool {
	return codeact.NewOnDemandTool(
		"find_file",
		fmt.Sprintf(findFileDescription, "```"),
		findFileHandler,
	)
}

func findFileHandler(session *codeact.Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		inputObj := call.Argument(0).ToObject(session.VM)

		input := &FindFileInput{}
		if pattern := inputObj.Get("pattern"); pattern != nil {
			input.Pattern = pattern.String()
		}
		if path := inputObj.Get("path"); path != nil {
			input.Path = path.String()
		}
		if excludePattern := inputObj.Get("exclude_pattern"); excludePattern != nil {
			input.ExcludePattern = excludePattern.String()
		}
		if maxResults := inputObj.Get("max_results"); maxResults != nil {
			input.MaxResults = int(maxResults.ToInteger())
		}

		if input.MaxResults == 0 {
			input.MaxResults = 50
		}

		result, err := findFile(session.FS, input)
		if err != nil {
			session.Throw(err)
		}

		return session.VM.ToValue(result)
	}
}

func findFile(fsys afero.Fs, input *FindFileInput) (*FindFileResult, error) {
	if input.Pattern == "" || input.Path == "" {
		return nil, codeact.NewCustomError("pattern and path are required", []string{
			"Please provide a valid glob pattern and absolute path",
		})
	}

	if !filepath.IsAbs(input.Path) {
		return nil, codeact.NewError(codeact.PathIsNotAbsolute, "path", input.Path)
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
				return &FindFileResult{
					Files:          files,
					TotalFiles:     len(files),
					TruncatedCount: totalMatches - len(files),
				}, nil
			}
			files = append(files, filePath)
		}
	}

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
		return nil, codeact.NewCustomError("glob pattern error", []string{
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

	return &FindFileResult{
		Files:          files,
		TotalFiles:     len(files),
		TruncatedCount: truncatedCount,
	}, nil
}