package tool

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/grafana/sobek"
)

const listFilesDescription = `
## Description
Lists the contents of a directory, showing files and subdirectories. This tool provides a quick way to explore the file structure of your project and navigate through directories. It's optimized for performance and provides a clear, structured view of directory contents.

## Parameters
- **path** (string, required): Absolute path to the directory you want to list (e.g., "/workspace/project/src"). Forward slashes (/) work on all platforms.
- **recursive** (boolean, required): When set to true, lists all files and directories recursively through all subdirectories. When false or omitted, only lists the top-level contents of the specified directory.

## Expected Output
Returns an object containing an array of directory entries. A file is identified by the type code "f" and a directory by the type code "d":
%[1]s
{
  "path": "/absolute/path/to/listed/directory",
  "entries": [
    ["file.js", "f", 1024],  // [name, type code, size in kilobytes]
    ["images", "d"]          // directories don't need size
  ]
}
%[1]s

If the directory doesn't exist or cannot be accessed, this tool will throw an exception with a descriptive error message.

## CRITICAL REQUIREMENTS
- **Verify directory existence**: Try/catch the operation to handle potential exceptions
%[1]s
  try {
    const dirContents = list_dir("/workspace/project/src");
    print("Found ${dirContents.entries.length} items");
  } catch (error) {
    print("Error listing directory:", error.message);
  }
%[1]s
- **Path format**: Always use absolute paths starting with "/"
%[1]s
  // Correct path format
  list_dir("/workspace/project/src")
%[1]s
- **Performance considerations**: Be cautious with the recursive option on large directories
%[1]s
  // First list non-recursively to understand structure
  try {
    const topLevelContents = list_dir("/workspace/project");
    print("Top-level directories:", topLevelContents.entries
      .filter(entry => entry.type === "directory")
      .map(dir => dir.name));

    // Then list specific subdirectories recursively if needed
    const componentsContents = list_dir("/workspace/project/src/components", true);
  } catch (error) {
    print("Error exploring project structure:", error.message);
  }
%[1]s
- **Exception handling**: Always wrap directory operations in try/catch blocks

## When to use
- **Project exploration**: When you need to understand the structure of a project
- **File location**: When looking for specific files or file types
- **Verification**: To confirm directories exist before performing operations
- **Path discovery**: To identify the correct paths for file operations
- **Structure analysis**: To analyze the organization of a project directory
- **Before file operations**: Before reading from or writing to files to ensure correct paths

## Common Errors and Solutions
- **"Directory not found"**: Exception will be thrown if the directory doesn't exist - verify the path is correct
- **"Permission denied"**: Exception will be thrown if you lack read permissions - check file system permissions
- **"Not a directory"**: Exception will be thrown if the path points to a file - ensure you're using a directory path
- **"Path is not absolute"**: Exception will be thrown if path doesn't start with "/" - always use absolute paths

## Usage Examples

%[1]s
try {
  // List top-level contents non-recursively
  const srcFiles = list_dir("/workspace/project/src");
  print("Top-level JS files:", srcFiles.entries
    .filter(e => e.type === "file" && e.name.endsWith(".js"))
    .map(f => f.name));

  // Find subdirectories and explore one recursively
  const components = srcFiles.entries.find(e => e.type === "directory" && e.name === "components");
  if (components) {
    const allComponents = list_dir("/workspace/project/src/components", true);

    // Group files by extension
    const byExt = allComponents.entries
      .filter(e => e.type === "file")
      .reduce((acc, f) => {
        const ext = f.name.split('.').pop() || "unknown";
        acc[ext] = (acc[ext] || 0) + 1;
        return acc;
      }, {});
    print("Files by extension:", byExt);
  }
} catch (error) {
  print("Error listing directory:", error.message);
}
%[1]s
`

func NewListFilesTool() CodeActTool {
	return NewOnDemandTool(
		"list_files",
		fmt.Sprintf(listFilesDescription, "```"),
		listFilesHandler,
	)
}

type DirectoryEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

func listFilesHandler(session CodeActSession) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) != 2 {
			session.Throw(NewCustomError("list_files requires exactly 2 arguments: path and recursive", []string{
				"- **path** (string, required): Absolute path to the directory you want to list (e.g., \"/workspace/project/src\"). Forward slashes (/) work on all platforms.\n" +
					"- **recursive** (boolean, required): When set to true, lists all files and directories recursively through all subdirectories. When false only lists the top-level contents of the specified directory.",
			}))
		}

		path := call.Argument(0).String()
		recursive := call.Argument(1).ToBoolean()

		dirEntries, err := listFiles(path, recursive)
		if err != nil {
			session.Throw(err)
		}

		return session.VM.ToValue(dirEntries)
	}
}

func listFiles(path string, recursive bool) ([]DirectoryEntry, error) {
	if !filepath.IsAbs(path) {
		return nil, NewError(PathIsNotAbsolute, "path", path)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewError(DirectoryNotFound, "path", path)
		}
		if os.IsPermission(err) {
			return nil, NewError(PermissionDenied, "path", path)
		}
		return nil, NewError(CannotStatFile, "path", path)
	}

	if !fileInfo.IsDir() {
		return nil, NewError(PathIsNotDirectory, "path", path)
	}

	var entries []DirectoryEntry
	if recursive {
		err = filepath.WalkDir(path, func(filePath string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if filePath == path {
				return nil
			}

			dirEntry, err := toDirectoryEntry(entry)
			if err != nil {
				return err
			}
			entries = append(entries, *dirEntry)
			return nil
		})

		if err != nil {
			if os.IsPermission(err) {
				return nil, NewError(PermissionDenied, "path", path)
			}
			return nil, NewError(GenericFileError, "path", path, "error", err)
		}
	} else {
		dirEntries, err := os.ReadDir(path)
		if err != nil {
			if os.IsPermission(err) {
				return nil, NewError(PermissionDenied, "path", path)
			}
			return nil, NewError(GenericFileError, "path", path, "error", err)
		}

		for _, entry := range dirEntries {
			dirEntry, err := toDirectoryEntry(entry)
			if err != nil {
				return nil, err
			}
			entries = append(entries, *dirEntry)
		}
	}

	return entries, nil
}

func toDirectoryEntry(entry fs.DirEntry) (*DirectoryEntry, error) {
	info, err := entry.Info()
	if err != nil {
		return nil, NewError(GenericFileError, "path", entry.Name(), "error", err)
	}

	if entry.IsDir() {
		return &DirectoryEntry{
			Name: entry.Name(),
			Type: "d",
			Size: 0,
		}, nil
	} else {
		return &DirectoryEntry{
			Name: entry.Name(),
			Type: "f",
			Size: (info.Size() + 1023) / 1024, // Size in KB
		}, nil
	}
}
