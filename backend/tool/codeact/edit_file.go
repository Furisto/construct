package codeact

import (
	"fmt"

	"github.com/grafana/sobek"

	"github.com/furisto/construct/backend/tool/filesystem"
)

const editFileDescription = `
## Description
Performs targeted modifications to existing files by replacing specific text sections with new content. This tool enables precise code changes without affecting surrounding content.

## Parameters
- **path** (string, required): Absolute path to the file to modify (e.g., "/workspace/project/src/components/Button.jsx").
- **diffs** (array, required): Array of diff objects, each containing:
  - **old** (string, required): The exact text to find and replace
  - **new** (string, required): The new text to replace it with

## Expected Output
Returns an object indicating success and details about changes made:
%[1]s
{
  "success": true,
  "path": "/path/to/file",
  "replacements_made": 2,
  "expected_replacements": 2,
  "patch_info": {
    "patch": "--- filename\n+++ filename\n@@ -1,3 +1,3 @@\n line1\n-old content\n+new content\n line3",
    "lines_added": 1,
    "lines_removed": 1
  },
  "validation_errors": [],
  "conflict_warnings": []
}
%[1]s

**Details:**
- success: Boolean indicating whether all replacements were successfully made.
- path: The absolute path of the file that was edited (same as input parameter).
- replacements_made: Number of text replacements that were actually performed.
- expected_replacements: Number of diff objects provided in the input array.
- patch_info: An object containing patch details (only present when changes were made):
  - patch: A unified diff patch string showing the exact changes made to the file.
  - lines_added: Number of lines added by the edits.
  - lines_removed: Number of lines removed by the edits.
- validation_errors: Array of validation errors for individual diffs (only present when validation fails). A validation error will not fail the edit but it 
indicates that the edit may not have been applied as expected. Each error contains:
  - diff_index: Index of the diff that failed.
  - error_type: Type of error ("not_found", "no_op", etc.).
  - error_message: Human-readable description of the error.
  - suggested_fix: Suggested fix for the error.
- conflict_warnings: Array of potential conflicts detected between multiple edits (only present when conflicts exist). A conlict will prevent the edit from being applied. Each warning contains:
  - edit1_index: Index of the first conflicting edit.
  - edit2_index: Index of the second conflicting edit.
  - conflict_type: Type of conflict ("dependency", "overlap", "duplicate_target", "line_overlap").
  - message: Human-readable description of the conflict.

## CRITICAL REQUIREMENTS
- **Exact matching**: The "old" content must match file content exactly (whitespace, indentation, line endings)
- **Whitespace preservation**: Maintain proper indentation and formatting in new_text
- **Sufficient context**: Include 3-5 surrounding lines in each "old" text for unique matching
- **Multiple changes**: For multiple changes, add separate objects to the diffs array in file order
- **Concise blocks**: Keep diff blocks focused on specific changes; break large edits into smaller blocks
- **Special operations**:
  - To move code: Use two diffs (one to delete from original (empty "new") + one to insert at new location (empty "old"))
  - To delete code: Use empty string for "new" property
- **File path validation**: Always use absolute paths (starting with "/")
- **Escape sequences**: You need to ensure that the "old" and "new" text are properly escaped to match the file content exactly e.g if the file contains "Starting Agent Runtime...\\n", you need to ensure that you match that in the old text.
- **Ensuring that the edit has been applied successfully**: You need to ensure that the edit is successful by checking the return value of the edit_file tool. If the edit is not successful, you need to review the validation_errors and conflict_warnings and fix them before retrying the edit.

## When to use
- Refactoring code (changing variables, updating functions)
- Bug fixes requiring precise changes
- Feature implementation in existing files
- Configuration changes
- Any targeted code modifications

## Usage Examples

### Single modification
%[1]s
edit_file("/workspace/project/src/utils.js", [
  {
    "old": "function calculateTax(amount) {\n  return amount * 0.08;\n}",
    "new": "function calculateTax(amount, rate = 0.08) {\n  return amount * rate;\n}"
  }
]);
%[1]s

### Multiple modifications
%[1]s
edit_file("/workspace/project/src/components/Button.jsx", [
  {
    "old": "import React from 'react';",
    "new": ""
  },
  {
    "old": "function Button({ text, onClick }) {",
    "new": "function Button({ text, onClick, disabled = false }) {"
  },
  {
    "old": "<button className=\"primary-button\" onClick={onClick}>",
    "new": "<button className=\"primary-button\" onClick={onClick} disabled={disabled}>"
  },
  {
    "old": "",
    "new": "}"
  }
]);
%[1]s
`

func NewEditFileTool() Tool {
	return NewOnDemandTool(
		"edit_file",
		fmt.Sprintf(editFileDescription, "```"),
		editFileInput,
		editFileHandler,
	)
}

func editFileInput(session *Session, args []sobek.Value) (any, error) {
	if len(args) < 2 {
		return nil, NewCustomError("invalid arguments", []string{
			"The edit_file tool requires exactly two arguments: path and diffs",
		}, "arguments", args)
	}

	path := args[0].String()
	diffsArg := args[1]

	// Parse diffs array
	var diffs []filesystem.DiffPair
	if diffsObj := diffsArg.ToObject(session.VM); diffsObj != nil && diffsObj != sobek.Undefined() {
		if lengthVal := diffsObj.Get("length"); lengthVal != nil {
			length := int(lengthVal.ToInteger())
			for i := 0; i < length; i++ {
				if diffVal := diffsObj.Get(fmt.Sprintf("%d", i)); diffVal != nil {
					if diffObj := diffVal.ToObject(session.VM); diffObj != nil {
						oldText := ""
						newText := ""
						if oldVal := diffObj.Get("old"); oldVal != nil {
							oldText = oldVal.String()
						}
						if newVal := diffObj.Get("new"); newVal != nil {
							newText = newVal.String()
						}
						diffs = append(diffs, filesystem.DiffPair{Old: oldText, New: newText})
					}
				}
			}
		}
	}

	return &filesystem.EditFileInput{
		Path:  path,
		Diffs: diffs,
	}, nil
}

func editFileHandler(session *Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		rawInput, err := editFileInput(session, call.Arguments)
		if err != nil {
			session.Throw(err)
		}
		input := rawInput.(*filesystem.EditFileInput)

		if len(input.Diffs) == 0 {
			session.Throw(NewCustomError("diffs array cannot be empty", []string{
				"Provide at least one diff object with 'old' and 'new' properties",
			}))
		}

		result, err := filesystem.EditFile(session.FS, input)
		if err != nil {
			session.Throw(err)
		}

		SetValue(session, "result", result)
		return session.VM.ToValue(result)
	}
}
