package tool

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/furisto/construct/backend/tool/codeact"
	"github.com/grafana/sobek"
)

var grepDescription = `
## Description
The regex_search tool performs fast text-based regex searches to find exact pattern matches within files or directories. It leverages efficient searching algorithms to quickly scan through your codebase and locate specific patterns.

## Parameters
- **query** (*string*, required): The regex pattern to search for. Must be a valid regex pattern; special characters must be escaped appropriately.
- **path** (*string*, required): Absolute path to the directory or file to search within. Forward slashes (/) work on all platforms.
- **include_pattern** (*string*, optional): Glob pattern for files to include in the search (e.g., "*.js" for JavaScript files only). Allows focusing your search on specific file types.
- **exclude_pattern** (*string*, optional): Glob pattern for files to exclude from the search. Useful for ignoring build artifacts, dependencies, or other irrelevant files.
- **case_sensitive** (*boolean*, optional): Whether the search should be case sensitive. Defaults to false.
- **max_results** (*number*, optional): Maximum number of results to return. Defaults to 50 to prevent overwhelming output.

## Expected Output
Returns an object containing the search results:
%[1]s
{
  "matches": [
    {
      "file_path": "/path/to/file.js",
      "line_number": 42,
      "line_content": "const searchPattern = /regex/;",
      "context": [
        { "line_number": 40, "content": "// Previous lines for context" },
        { "line_number": 41, "content": "// More context" },
        { "line_number": 42, "content": "const searchPattern = /regex/;" },
        { "line_number": 43, "content": "// Context after match" }
      ]
    },
    // Additional matches...
  ],
  "total_matches": 3,
  "searched_files": 125
}
%[1]s

## CRITICAL REQUIREMENTS
- **Precise Pattern Specification**: Your regex pattern must be properly escaped for accurate matching.
  %[1]s
  // To search for "user.login()", escape special characters:
  regex_search({
    query: "user\\.login\\(\\)",
    path: "/workspace/src"
  })
  %[1]s
- **Search Path Verification**: Always use absolute paths starting with "/" for consistent results.
- **Scope Management**: Use include/exclude patterns to control search scope and improve performance:
  %[1]s
  // Only search JavaScript files, exclude tests
  regex_search({
    query: "function init",
    path: "/workspace/project",
    include_pattern: "*.js",
    exclude_pattern: "**/__tests__/**"
  })
  %[1]s
- **Performance Considerations**:
  - Narrow your search scope with specific paths and patterns for faster results
  - Be specific with your regex to avoid excessive matches
  - Use reasonable max_results limits for large codebases
- **Complex Pattern Handling**: For complex patterns, test iteratively:
  %[1]s
  // First search broadly
  regex_search({
    query: "api\\.connect",
    path: "/workspace/src"
  })
  
  // Then refine with more specific pattern
  regex_search({
    query: "api\\.connect\\(['\"]production['\"]\\)",
    path: "/workspace/src/services"
  })
  %[1]s

## When to use
- **Finding Symbol Definitions**: When you need to locate specific function, class, or variable definitions.
- **Code Pattern Discovery**: When identifying patterns across multiple files (error handling, API calls, etc.).
- **API Usage Exploration**: When discovering how specific APIs or functions are used throughout the codebase.
- **Error Text Location**: When tracking down where specific error messages are defined or thrown.
- **Dependency Identification**: When finding all imports or requires of a specific module.
- **Configuration Search**: When locating specific configuration patterns across multiple files.

## Usage Examples

### Finding Function Definitions
%[1]s
regex_search({
  query: "function\\s+getUserData\\s*\\(",
  path: "/workspace/src",
  include_pattern: "*.js",
  exclude_pattern: "**/node_modules/**"
})
%[1]s
`

type GrepInput struct {
	Query          string `json:"query"`
	Path           string `json:"path"`
	IncludePattern string `json:"include_pattern"`
	ExcludePattern string `json:"exclude_pattern"`
	CaseSensitive  bool   `json:"case_sensitive"`
	MaxResults     int    `json:"max_results"`
}

type GrepMatch struct {
	FilePath    string        `json:"file_path"`
	LineNumber  int           `json:"line_number"`
	LineContent string        `json:"line_content"`
	Context     []ContextLine `json:"context"`
}

type ContextLine struct {
	LineNumber int    `json:"line_number"`
	Content    string `json:"content"`
}

type GrepResult struct {
	Matches       []GrepMatch `json:"matches"`
	TotalMatches  int         `json:"total_matches"`
	SearchedFiles int         `json:"searched_files"`
}

func NewGrepTool() codeact.Tool {
	return codeact.NewOnDemandTool(
		ToolNameGrep,
		fmt.Sprintf(grepDescription, "```"),
		grepHandler,
	)
}

func grepHandler(session *codeact.Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		inputObj := call.Argument(0).ToObject(session.VM)

		input := &GrepInput{}
		if query := inputObj.Get("query"); query != nil {
			input.Query = query.String()
		}
		if path := inputObj.Get("path"); path != nil {
			input.Path = path.String()
		}
		if includePattern := inputObj.Get("include_pattern"); includePattern != nil {
			input.IncludePattern = includePattern.String()
		}
		if excludePattern := inputObj.Get("exclude_pattern"); excludePattern != nil {
			input.ExcludePattern = excludePattern.String()
		}
		if caseSensitive := inputObj.Get("case_sensitive"); caseSensitive != nil {
			input.CaseSensitive = caseSensitive.ToBoolean()
		}
		if maxResults := inputObj.Get("max_results"); maxResults != nil {
			input.MaxResults = int(maxResults.ToInteger())
		}

		if input.MaxResults == 0 {
			input.MaxResults = 50
		}

		result, err := grep(input)
		if err != nil {
			session.Throw(err)
		}

		return session.VM.ToValue(result)
	}
}

func grep(input *GrepInput) (*GrepResult, error) {
	if input.Query == "" || input.Path == "" {
		return nil, fmt.Errorf("query and path are required")
	}

	if isRipgrepAvailable() {
		return performRipgrep(input)
	}

	return performRegularGrep(input)
}

func isRipgrepAvailable() bool {
	_, err := exec.LookPath("rg")
	return err == nil
}

func performRipgrep(input *GrepInput) (*GrepResult, error) {
	args := []string{
		"--json",
		"--line-number",
		"--with-filename",
		"--context", "2",
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

	cmd := exec.Command("rg", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return &GrepResult{
				Matches:       []GrepMatch{},
				TotalMatches:  0,
				SearchedFiles: 0,
			}, nil
		}
		return nil, fmt.Errorf("ripgrep error: %v", err)
	}

	return parseRipgrepOutput(string(output), input.MaxResults)
}

func performRegularGrep(input *GrepInput) (*GrepResult, error) {
	args := []string{
		"-r",
		"-n",
		"-H",
		"-C", "2",
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

	cmd := exec.Command("grep", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return &GrepResult{
				Matches:       []GrepMatch{},
				TotalMatches:  0,
				SearchedFiles: 0,
			}, nil
		}
		return nil, fmt.Errorf("grep error: %v", err)
	}

	return parseGrepOutput(string(output), input.MaxResults)
}

func parseRipgrepOutput(output string, maxResults int) (*GrepResult, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	matches := []GrepMatch{}
	searchedFiles := make(map[string]bool)

	type rgMatch struct {
		Type string `json:"type"`
		Data struct {
			Path struct {
				Text string `json:"text"`
			} `json:"path"`
			LineNumber int `json:"line_number"`
			Lines      struct {
				Text string `json:"text"`
			} `json:"lines"`
		} `json:"data"`
	}

	for _, line := range lines {
		if line == "" {
			continue
		}

		var match rgMatch
		if err := json.Unmarshal([]byte(line), &match); err != nil {
			continue
		}

		if match.Type == "match" {
			if len(matches) >= maxResults {
				break
			}

			filePath := match.Data.Path.Text
			searchedFiles[filePath] = true

			grepMatch := GrepMatch{
				FilePath:    filePath,
				LineNumber:  match.Data.LineNumber,
				LineContent: match.Data.Lines.Text,
				Context:     []ContextLine{},
			}

			matches = append(matches, grepMatch)
		}
	}

	return &GrepResult{
		Matches:       matches,
		TotalMatches:  len(matches),
		SearchedFiles: len(searchedFiles),
	}, nil
}

func parseGrepOutput(output string, maxResults int) (*GrepResult, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	matches := []GrepMatch{}
	searchedFiles := make(map[string]bool)

	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		if len(matches) >= maxResults {
			break
		}

		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}

		filePath := parts[0]
		lineNumStr := parts[1]
		content := parts[2]

		lineNum, err := strconv.Atoi(lineNumStr)
		if err != nil {
			continue
		}

		searchedFiles[filePath] = true

		grepMatch := GrepMatch{
			FilePath:    filePath,
			LineNumber:  lineNum,
			LineContent: content,
			Context:     []ContextLine{},
		}

		matches = append(matches, grepMatch)
	}

	return &GrepResult{
		Matches:       matches,
		TotalMatches:  len(matches),
		SearchedFiles: len(searchedFiles),
	}, nil
}
