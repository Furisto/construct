package tool

import (
	"os"
)

type Toolbox struct {
	tools map[string]Tool
}

func NewToolbox() *Toolbox {
	return &Toolbox{
		tools: map[string]Tool{},
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
