package tool

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/grafana/sobek"
)

const writeFileDescription = `
# Description
Creates a new file or replaces an existing file with your specified content. This tool writes the complete file in a single operation.

# Parameters
- **path**: (string, required) Absolute path to the file (e.g., "/workspace/construct/src/components/button.js"). Forward slashes (/) work on all platforms. All necessary parent directories will be created automatically.
- **content**: (string, required) ENTIRE content to write to the file. No placeholders, ellipses, or "rest of file unchanged". 

# Expected Output
This tool does return whether the file already existed. If the operation fails, it will return an error message describing the issue.
Example output:
{
	"existed": true
}

# CRITICAL REQUIREMENTS
- **Maintain proper syntax, indentation, and structure**
- **Include complete file content**: Always provide the entire content, including imports, exports, and all necessary code
- **Match file extension with content**: Ensure the file extension corresponds to the content type
%[1]s
  // Correct: .jsx extension for React JSX code
  create_file("/workspace/project/components/Header.jsx", "import React from 'react';...")
%[1]s
- **Preserve existing structure if overwriting**: If overwriting an existing file, consider reading it first to understand its structure
- **Verify file structure first**: Before creating a file, ensure you understand the project's file organization
%[1]s
  // First list the directory to understand structure
  list_dir("/workspace/project/src")
  // Then create the file in the appropriate location
  create_file("/workspace/project/src/utils/helpers.js", "...")
%[1]s
- **Use template literals**: For multi-line content, use backtick (%[2]s) template literals to preserve formatting
%[1]s
  create_file("/path/to/file.txt",%[2]sLine one
  Line two
  Line three%[2]s)
%[1]s

# When to use
- Creating new files: When you need to generate source code files, configuration files, or documentation from scratch
- Full file replacements: When you need to completely replace the contents of an existing file
- Generating code: When creating boilerplate code, templates, or scaffolding for a project
- Saving computation results: When you need to persist data, logs, or computation results to disk
- Building project structure: When setting up initial project organization or adding new components

# Common Errors and Solutions
- **"Invalid path"**: Ensure you're using an absolute path starting with "/"

# Usage examples

## Write a JSON file
%[1]s
write_file("config/settings.json",
"{\n\
  \"apiEndpoint\": \"https://api.example.com\",\n\
  \"debugMode\": false,\n\
  \"version\": \"1.0.0\"\n\
}")
%[1]s

## Write a JavaScript file
%[1]s
write_file("src/components/Button.jsx", 
"import React from \'react\';\\n\\\
\n\
function Button({ text, onClick }) {\n\
  return (\n\
	<button className=\"primary-button\" onClick={onClick}>\n\
	  {text}\n\
	</button>\n\
  );\n\
}\n\
\n\
export default Button;")
%[1]s
`

func NewCreateFileTool() CodeActTool {
	return NewOnDemandTool(
		"create_file",
		fmt.Sprintf(writeFileDescription, "```", "`"),
		createFileCallback,
	)
}

func createFileCallback(session CodeActSession) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) != 2 {
			panic("invalid arguments")
		}

		path := call.Arguments[0].String()
		content := call.Arguments[1].String()

		return session.VM().ToValue(createFile(path, content))
	}
}

func createFile(path string, content string) CreateFileResult {
	if !filepath.IsAbs(path) {
		panic(fmt.Sprintf("path must be absolute: %s", path))
	}

	var existed bool
	if stat, err := os.Stat(path); err == nil {
		if stat.IsDir() {
			panic(fmt.Sprintf("path is a directory: %s", path))
		}

		existed = true
	}

	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		panic(fmt.Errorf("error creating parent directories for %s: %w", path, err))
	}

	err = os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		panic(fmt.Errorf("error writing file %s: %w", path, err))
	}

	return CreateFileResult{Existed: existed}
}

type CreateFileResult struct {
	Existed bool `json:"existed"`
}
