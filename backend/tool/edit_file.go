package tool

import (
	"fmt"

	"github.com/grafana/sobek"
)

const editFileDescription = `
# Description
Performs targeted modifications to existing files by replacing specific text sections with new content. This tool enables precise code changes without affecting surrounding content.

# Parameters
- **path** (string, required): Absolute path to the file to modify (e.g., "/workspace/project/src/components/Button.jsx").
- **diffs** (array, required): Array of diff objects, each containing:
  - **old** (string, required): The exact text to find and replace
  - **new** (string, required): The new text to replace it with

# Expected Output
Returns an object indicating success and details about changes made:
%[1]s
{
  "success": true,
  "path": "/path/to/file",
  "replacements_made": 2,
  "expected_replacements": 2
}
%[1]s

# CRITICAL REQUIREMENTS
- **Exact matching**: The "old" content must match file content exactly (whitespace, indentation, line endings)
- **Whitespace preservation**: Maintain proper indentation and formatting in new_text
- **Sufficient context**: Include 3-5 surrounding lines in each "old" text for unique matching
- **Multiple changes**: For multiple changes, add separate objects to the diffs array in file order
- **Concise blocks**: Keep diff blocks focused on specific changes; break large edits into smaller blocks
- **Special operations**:
  - To move code: Use two diffs (one to delete from original (empty "new") + one to insert at new location (empty "old"))
  - To delete code: Use empty string for "new" property
- **File path validation**: Always use absolute paths (starting with "/")

# When to use
- Refactoring code (changing variables, updating functions)
- Bug fixes requiring precise changes
- Feature implementation in existing files
- Configuration changes
- Any targeted code modifications

# Common Errors and Solutions
- **"No matches found"**: Verify "old" text matches file content exactly
- **"Multiple matches found"**: Add more context lines for unique matching
- **"Unexpected replacements"**: Make "old" patterns more specific
- **"File not found"**: Verify file path before modifying
`

func NewEditFileTool() CodeActTool {
	return NewOnDemandTool(
		"edit_file",                             
		fmt.Sprintf(editFileDescription, "```"), 
		editFileCallback,                        
	)
}

func editFileCallback(session CodeActSession) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) != 2 {
			session.Throw("edit_file requires exactly 2 arguments: path and diffs array")
		}
		return sobek.Undefined() 
	}
}

