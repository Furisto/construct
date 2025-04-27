package tool

import (
	"fmt"
	"os"

	"github.com/grafana/sobek"
)

const readFileDescription = `
# Description
Reads and returns the complete contents of a file at the specified path. This tool is essential for examining existing files when you need to understand, analyze, or extract information from them. The file content is returned as a string, making it suitable for text files such as code, configuration files, documentation, and structured data.

# Parameters
- **path** (string, required): Absolute path to the file you want to read (e.g., "/workspace/project/src/app.js"). Forward slashes (/) work on all platforms.

# Expected Output
Returns an object containing the file content as a string:
%[1]s
{
  "path": "The absolute path of the file",
  "content": "The complete content of the file as a string"
}
%[1]s

If the file doesn't exist or cannot be read, it will throw an exception describing the issue.

# CRITICAL REQUIREMENTS
- **Always verify file existence**: Check if a file exists before attempting operations that assume its presence
- **Handle large files appropriately**: For very large files, consider processing the content in chunks
- **Check file extensions**: Ensure you're reading appropriate file types; this tool is best suited for text files
- **Process binary files carefully**: Binary files may return unreadable content; consider specialized tools for these cases
- **Path format**: Always use absolute paths starting with "/"
%[1]s
  // Correct path format
  read_file("/workspace/project/package.json")
%[1]s

# When to use
- **Code analysis**: When you need to understand existing code structure, imports, or implementations
- **Configuration inspection**: To examine settings in config files like JSON, YAML, or .env files
- **Content extraction**: To retrieve data from text files for processing or analysis
- **Before modifications**: Read a file first to understand its structure before making changes
- **Documentation review**: To analyze README files, specifications, or documentation
- **Data gathering**: When collecting information stored in logs, CSVs, or other structured data files

# Common Errors and Solutions
- **"File not found"**: Verify the file path is correct and the file exists using appropriate tools
- **"Permission denied"**: Ensure you have read permissions for the file
- **"Path is not absolute"**: Always use paths starting with "/" (e.g., "/workspace/project/file.txt")

# Usage Examples

## Analyzing source code
%[1]s
const sourceCode = read_file("/workspace/project/src/components/Button.jsx");
if (!sourceCode.error) {
  // Count React hooks in component
  const hooksCount = (sourceCode.content.match(/use[A-Z]\w+\(/g) || []).length;
  print("This component uses ${hooksCount} React hooks");
}
%[1]s

## Reading and processing structured data
%[1]s
const csvData = read_file("/workspace/project/data/users.csv");
if (!csvData.error) {
  const rows = csvData.content.split('\n').map(row => row.split(','));
  const headers = rows.shift();
  print("Found ${rows.length} user records with fields: ${headers.join(', ')}");
}
%[1]s
`

type ReadFileResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func NewReadFileTool() CodeActTool {
	return NewOnDemandTool(
		"read_file",                             
		fmt.Sprintf(readFileDescription, "```"), 
		readFileCallback,                        
	)
}

func readFileCallback(session CodeActSession) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		path := call.Argument(0).String()

		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				panic(fmt.Errorf("file not found: %s", path))
			}
			if os.IsPermission(err) {
				panic(fmt.Errorf("permission denied: %s", path))
			}
			panic(fmt.Errorf("error reading file %s: %w", path, err))
		}

		content, err := os.ReadFile(path)
		if err != nil {
			panic(fmt.Errorf("error reading file %s: %w", path, err))
		}

		return session.VM().ToValue(ReadFileResult{
			Path:    path,
			Content: string(content),
		})
	}
}

