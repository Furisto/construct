
## Description
Lists the contents (files and subdirectories) of a specified directory. This tool helps explore project file structures and navigate directories by providing a clear, structured view of their contents.

## Parameters
- **path** (string, required): Absolute path to the directory you want to list (e.g., "/workspace/project/src"). Forward slashes (/) work on all platforms.
- **recursive** (boolean, required): When set to true, lists all files and directories recursively through all subdirectories. When false, only lists the top-level contents of the specified directory.

## Expected Output
Returns an object containing an array of directory entries. A file is identified by the type code "f" and a directory by the type code "d":
```
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
```

**Details:**
-   The `path` field in the response object will be the same absolute path provided in the `path` parameter.
-   `entries`: An array of objects, each representing a file or directory.
    -   `n` (name): The name of the file or subdirectory. This will always be an absolute path.
    -   `t` (type): A character code indicating the entry type:
        -   `'f'`: Represents a regular file.
        -   `'d'`: Represents a directory.
    -   `s` (size): The size of the entry **in kilobytes**. For directories, the size is reported as 0.
-   If the target directory is empty, `entries` will be an empty array (`[]`).
-   If the specified `path` does not exist, is not a directory, or cannot be accessed due to permissions or other issues, the tool will throw an exception with a descriptive error message.

## IMPORTANT USAGE NOTES
- **Path format**: Always use absolute paths starting with "/"
```
  // Correct path format
  list_files("/workspace/project/src", false)
```
- **Performance considerations**: Be cautious with the recursive option on large directories
```
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
```
- **Exception handling**: Always wrap directory operations in try/catch blocks

## When to use
- **Project exploration**: When you need to understand the structure of a project
- **File location**: When looking for specific files or file types (e.g., all *.js files in /src)
- **Verification**: To confirm directories exist before performing operations
- **Path discovery**: To identify the correct paths for subsequent file operations
- **Pre-computation for operations**: Before batch operations like deleting, copying, or archiving, to gather the list of items to be processed

## Usage Examples

```
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
```
