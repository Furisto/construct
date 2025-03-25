package toolbox

import (
	"context"
	"os"
)

type Tool struct {
	Name        string
	Description string
	Parameters  []Parameter
	Readonly    bool
	Execute     func(ctx context.Context, parameters map[string]string) (string, error)
}

type Parameter struct {
	Name        string
	Description string
	Required    bool
}

type Toolbox struct {
	tools map[string]Tool
}

func NewToolbox() *Toolbox {
	return &Toolbox{
		tools: map[string]Tool{
			// File operations
			"read_file": {
				Name:        "read_file",
				Description: "Read a file",
				Parameters: []Parameter{
					{Name: "file_path", Description: "The path to the file to read", Required: true},
				},
				Execute: func(ctx context.Context, parameters map[string]string) (string, error) {
					filePath := parameters["file_path"]
					content, err := os.ReadFile(filePath)
					if err != nil {
						return "", err
					}
					return string(content), nil
				},
			},
			"write_file": {
				Name:        "write_file",
				Description: "Write to a file",
				Parameters: []Parameter{
					{Name: "file_path", Description: "The path to the file to write to", Required: true},
				},
				Execute: func(ctx context.Context, parameters map[string]string) (string, error) {
					filePath := parameters["file_path"]
					content := parameters["content"]
					err := os.WriteFile(filePath, []byte(content), 0644)
					if err != nil {
						return "", err
					}
					return "File written successfully", nil
				},
			},
			"edit_file": {
				Name:        "edit_file",
				Description: "Edit a file",
				Parameters: []Parameter{
					{Name: "file_path", Description: "The path to the file to edit", Required: true},
				},
			},
			"find_files": {
				Name:        "find_files",
				Description: "Search for files in the project",
				Parameters: []Parameter{
					{Name: "query", Description: "The query to search for", Required: true},
				},
			},
			"grep": {
				Name:        "grep",
				Description: "Grep for a pattern in the project",
				Parameters: []Parameter{
					{Name: "pattern", Description: "The pattern to grep for", Required: true},
				},
			},
			"list_files": {
				Name:        "list_files",
				Description: "List all files in the directory",
				Parameters: []Parameter{
					{Name: "directory", Description: "The directory to list files from", Required: true},
				},
				Execute: func(ctx context.Context, parameters map[string]string) (string, error) {
					directory := parameters["directory"]
					_, err := os.ReadDir(directory)
					if err != nil {
						return "", err
					}

					return "", nil
				},
			},
		},
	}
}

func (t *Toolbox) ListTools() []Tool {
	tools := make([]Tool, 0, len(t.tools))
	for _, tool := range t.tools {
		tools = append(tools, tool)
	}
	return tools
}

func WalkDirectoryTree(rootPath string) (string, error) {
	result := rootPath + "\n"
	err := walkDirRecursive(rootPath, &result, 1, 3, "  ")
	if err != nil {
		return "", err
	}
	return result, nil
}

func walkDirRecursive(path string, result *string, currentLevel, maxLevel int, indent string) error {
	if currentLevel > maxLevel {
		return nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			*result += indent + "|_ " + entry.Name() + "\n"
			err := walkDirRecursive(path+"/"+entry.Name(), result, currentLevel+1, maxLevel, indent+"        ")
			if err != nil {
				return err
			}
		}
	}

	return nil
}
