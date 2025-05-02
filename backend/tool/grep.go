package tool

import (
	"fmt"

	"github.com/grafana/sobek"
)

var grepDescription = `
# Description
The regex_search tool performs fast text-based regex searches to find exact pattern matches within files or directories. It leverages efficient searching algorithms to quickly scan through your codebase and locate specific patterns.

# Parameters
- **query** (*string*, required): The regex pattern to search for. Must be a valid regex pattern; special characters must be escaped appropriately.
- **path** (*string*, required): Absolute path to the directory or file to search within. Forward slashes (/) work on all platforms.
- **include_pattern** (*string*, optional): Glob pattern for files to include in the search (e.g., "*.js" for JavaScript files only). Allows focusing your search on specific file types.
- **exclude_pattern** (*string*, optional): Glob pattern for files to exclude from the search. Useful for ignoring build artifacts, dependencies, or other irrelevant files.
- **case_sensitive** (*boolean*, optional): Whether the search should be case sensitive. Defaults to false.
- **max_results** (*number*, optional): Maximum number of results to return. Defaults to 50 to prevent overwhelming output.

# Expected Output
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

# CRITICAL REQUIREMENTS
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

# When to use
- **Finding Symbol Definitions**: When you need to locate specific function, class, or variable definitions.
- **Code Pattern Discovery**: When identifying patterns across multiple files (error handling, API calls, etc.).
- **API Usage Exploration**: When discovering how specific APIs or functions are used throughout the codebase.
- **Error Text Location**: When tracking down where specific error messages are defined or thrown.
- **Dependency Identification**: When finding all imports or requires of a specific module.
- **Configuration Search**: When locating specific configuration patterns across multiple files.

# Common Errors and Solutions
- **"Invalid regex pattern"**: Ensure special characters (., +, *, ?, ^, $, (, ), [, ], {, }, |, \) are properly escaped with backslashes.
  %[1]s
  // Incorrect: regex_search({ query: "function doSomething()", ... })
  // Correct: regex_search({ query: "function doSomething\\(\\)", ... })
  %[1]s
- **"Too many matches"**: Narrow your search with a more specific query or use include/exclude patterns.
- **"No matches found"**: 
  - Verify your regex pattern is correct
  - Check if include/exclude patterns are too restrictive
  - Try a simpler pattern first and then refine
- **"Path not found"**: Ensure the search path exists and you have access permissions.

# Usage Examples

## Finding Function Definitions
%[1]s
regex_search({
  query: "function\\s+getUserData\\s*\\(",
  path: "/workspace/src",
  include_pattern: "*.js",
  exclude_pattern: "**/node_modules/**"
})
%[1]s
`

type GrepResult struct {
	Output string `json:"output"`
}

func NewGrepTool() CodeActTool {
	return NewOnDemandTool(
		"regex_search",
		fmt.Sprintf(grepDescription, "```"),
		grepHandler,
	)
}

func grepHandler(session CodeActSession) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		// query := call.Argument(0).String()
		// path := call.Argument(1).String()
		// includePattern := call.Argument(2).String()
		// excludePattern := call.Argument(3).String()
		// caseSensitive := call.Argument(4).ToBoolean()
		// maxResults := call.Argument(5).ToInteger()

		return sobek.Undefined()

	}
}
