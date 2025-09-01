package codeact

import (
	"fmt"
	"reflect"

	"github.com/grafana/sobek"

	"github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/filesystem"
)

const listFilesDescription = `
## Description
Lists the contents (files and subdirectories) of a specified directory. This tool helps explore project file structures and navigate directories by providing a clear, structured view of their contents.

## Parameters
- **path** (string, required): Absolute path to the directory you want to list (e.g., "/workspace/project/src"). Forward slashes (/) work on all platforms.
- **recursive** (boolean, required): When set to true, lists all files and directories recursively through all subdirectories. When false, only lists the top-level contents of the specified directory.

## Expected Output
Returns an object containing an array of directory entries. A file is identified by the type code "f" and a directory by the type code "d":
%[1]s
{
  "path": "/absolute/path/to/listed/directory", 
  "entries": [
    {
      "n": "/absolute/path/to/listed/directory/file.js", 
      "t": "f", 
      "s": 8 
    },
    {
      "n": "/absolute/path/to/listed/directory/images", 
      "t": "d",
      "s": 0 
    },
    {
      "n": "/absolute/path/to/listed/directory/images/logo.png", 
      "t": "f", 
      "s": 5 
    }
  ]
}
%[1]s

**Details:**
-   The %[2]spath%[2]s field in the response object will be the same absolute path provided in the %[2]spath%[2]s parameter.
-   %[2]sentries%[2]s: An array of objects, each representing a file or directory.
    -   %[2]sn%[2]s (name): The name of the file or subdirectory. This will always be an absolute path.
    -   %[2]st%[2]s (type): A character code indicating the entry type:
        -   %[2]s'f'%[2]s: Represents a regular file.
        -   %[2]s'd'%[2]s: Represents a directory.
    -   %[2]ss%[2]s (size): The size of the entry **in kilobytes**. For directories, the size is reported as 0.
-   If the target directory is empty, %[2]sentries%[2]s will be an empty array (%[2]s[]%[2]s).
-   If the specified %[2]spath%[2]s does not exist, is not a directory, or cannot be accessed due to permissions or other issues, the tool will throw an exception with a descriptive error message.

## IMPORTANT USAGE NOTES
- **Path format**: Always use absolute paths starting with "/"
%[1]s
  // Correct path format
  list_files("/workspace/project/src", false)
%[1]s
- **Performance considerations**: Be cautious with the recursive option on large directories
%[1]s
  // First list non-recursively to understand structure
  try {
    const topLevelContents = list_files("/workspace/project", false);
    print("Top-level directories:", topLevelContents.entries
      .filter(entry => entry.t === "d")
      .map(dir => dir.n));

    // Then list specific subdirectories recursively if needed
    const componentsContents = list_files("/workspace/project/src/components", true);
  } catch (error) {
    print("Error exploring project structure:", error);
  }
%[1]s
- **Exception handling**: Always wrap directory operations in try/catch blocks

## When to use
- **Project exploration**: When you need to understand the structure of a project
- **File location**: When looking for specific files or file types (e.g., all *.js files in /src)
- **Verification**: To confirm directories exist before performing operations
- **Path discovery**: To identify the correct paths for subsequent file operations
- **Pre-computation for operations**: Before batch operations like deleting, copying, or archiving, to gather the list of items to be processed

## Usage Examples

%[1]s
try {
  // List top-level contents non-recursively
  const srcFiles = list_files("/workspace/project/src", false);
  print("Top-level JS files:", srcFiles.entries
    .filter(e => e.t === "f" && e.n.endsWith(".js"))
    .map(f => f.n));

  // Find subdirectories and explore one recursively
  const components = srcFiles.entries.find(e => e.t === "d" && e.n === "components");
  if (components) {
    const allComponents = list_files("/workspace/project/src/components", true);

    // Group files by extension
    const byExt = allComponents.entries
      .filter(e => e.t === "f")
      .reduce((acc, f) => {
        const ext = f.n.split('.').pop() || "unknown";
        acc[ext] = (acc[ext] || 0) + 1;
        return acc;
      }, {});
    print("Files by extension:", byExt);
  }
} catch (error) {
  print("Error listing directory:", error);
}
%[1]s
`

func NewListFilesTool() Tool {
	return NewOnDemandTool(
		"list_files",
		fmt.Sprintf(listFilesDescription, "```", "`"),
		listFilesInput,
		listFilesHandler,
	)
}

func listFilesInput(session *Session, args []sobek.Value) (any, error) {
	switch len(args) {
	case 1:
		maybeObjectBasedInput := args[0].ToObject(session.VM)
		if maybeObjectBasedInput == nil || maybeObjectBasedInput == sobek.Undefined() {
			return nil, base.NewCustomError(base.InvalidInput.String(), listFilesSuggestions)
		}

		path := maybeObjectBasedInput.Get("path")
		if path == nil || path == sobek.Undefined() || path.ExportType() != reflect.TypeOf("") {
			return nil, base.NewCustomError(base.InvalidInput.String(), listFilesSuggestions)
		}

		recursive := maybeObjectBasedInput.Get("recursive")
		if recursive == nil || recursive == sobek.Undefined() || recursive.ExportType() != reflect.TypeOf(bool(false)) {
			return nil, base.NewCustomError(base.InvalidInput.String(), listFilesSuggestions)
		}

		return &filesystem.ListFilesInput{
			Path:      path.String(),
			Recursive: recursive.ToBoolean(),
		}, nil
	case 2:
		path := args[0]

		if path == nil || path == sobek.Undefined() || path.ExportType() != reflect.TypeOf("") {
			return nil, base.NewCustomError(base.InvalidInput.String(), listFilesSuggestions)
		}

		recursive := args[1]
		if recursive == nil || recursive == sobek.Undefined() || recursive.ExportType() != reflect.TypeOf(bool(false)) {
			return nil, base.NewCustomError(base.InvalidInput.String(), listFilesSuggestions)
		}

		return &filesystem.ListFilesInput{
			Path:      path.String(),
			Recursive: recursive.ToBoolean(),
		}, nil
	}

	return nil, base.NewCustomError(base.InvalidInput.String(), listFilesSuggestions)
}

func listFilesHandler(session *Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		rawInput, err := listFilesInput(session, call.Arguments)
		if err != nil {
			session.Throw(err)
		}
		input := rawInput.(*filesystem.ListFilesInput)

		result, err := filesystem.ListFiles(session.FS, input)
		if err != nil {
			session.Throw(err)
		}

		SetValue(session, "result", result)
		return session.VM.ToValue(result)
	}
}

var listFilesSuggestions = []string{
	"Ensure that you provide the correct input arguments as specified in the tool description",
	"- **path** (string, required): Absolute path to the directory you want to list (e.g., \"/workspace/project/src\"). Forward slashes (/) work on all platforms.",
	"- **recursive** (boolean, required): When set to true, lists all files and directories recursively through all subdirectories. When false, only lists the top-level contents of the specified directory.",
	"For example: list_files('/project/src', false)",
}
