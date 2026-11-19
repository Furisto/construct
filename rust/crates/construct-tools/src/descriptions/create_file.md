
## Description
Creates a new file with the specified content or completely overwrites an existing file if it already exists. This tool writes the complete file in a single operation. 
It provides detailed error messages via exceptions on failure.

## Parameters
- **path**: (string, required) Absolute path to the file beginning with a forward slash (e.g., "/workspace/construct/src/components/button.js"). Forward slashes (/) work on all platforms. All necessary parent directories will be created automatically.
- **content**: (string, required) ENTIRE content to write to the file. Do not use placeholders, ellipses, or "rest of file unchanged". 

## Expected Output
This tool returns whether the file already existed and was overwritten -- true if an existing file was replaced, false if a new file was created. If the operation fails, it will throw an exception describing the issue.
Example output if the file was overwritten:
{
"overwritten": true
}

## IMPORTANT USAGE NOTES
- **Maintain proper syntax, indentation, and structure**
- **Include complete file content**: Always provide the entire content, including imports, exports, and all necessary code.
- **Match file extension with content**: Ensure the file extension corresponds to the content type
```
// Correct: .jsx extension for React JSX code
create_file("/workspace/project/components/Header.jsx", "import React from 'react';...")
```
- **Preserve existing structure if overwriting**: If you intend to modify an existing file, it's best practice to first use the 'read_file' tool to understand its current structure and content. Then, when calling create_file, provide the complete new content for the file, incorporating your changes while ensuring the overall desired structure is maintained in the content you provide
- **Verify file structure first**: Before creating a file, ensure you understand the project's file organization
```
// First list the directory to understand structure
list_dir("/workspace/project/src")
// Then create the file in the appropriate location
create_file("/workspace/project/src/utils/helpers.js", "...")
```
- **Use template literals**: For multi-line content, use backtick (`) template literals to preserve formatting
```
create_file("/path/to/file.txt",`Line one
Line two
Line three`)
```
- **Always use absolute paths**: Always use absolute paths starting with "/".
- **For text-based files**: This tool is primarily designed for creating/overwriting text-based files (e.g., source code, configuration files, JSON, XML, Markdown). It does not support creating binary files.

## When to use
- Creating new files: When you need to generate source code files, configuration files, or documentation from scratch
- Full file replacements: When you need to completely replace the contents of an existing file
- Generating code: When creating boilerplate code, templates, or scaffolding for a project
- Saving computation results: When you need to persist data, logs, or computation results to disk
- Building project structure: When setting up initial project organization or adding new components
- An existing file is corrupted or fundamentally broken

## AVOID create_file when:
- **The file already exists** - use edit_file instead
- **Making modifications to existing functionality** - use edit_file instead
- **The change affects only part of a file** - use edit_file instead

## Usage examples

### Write a JSON file
```
create_file("/project/config/settings.json",
"{\n\
\"apiEndpoint\": \"https://api.example.com\",\n\
\"debugMode\": false,\n\
\"version\": \"1.0.0\"\n\
}")
```

### Write a JavaScript file
```
create_file("/todo-app/src/components/Button.jsx", `import React from 'react';

function Button({ text, onClick }) {
return (
<button className="primary-button" onClick={onClick}>
{text}
</button>
);
}

export default Button;`)
```
