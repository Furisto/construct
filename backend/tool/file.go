package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const FilesystemToolCategory = "filesystem"


type WriteFileInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

type EditFileInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

type FindFilesInput struct {
	Query string `json:"query"`
}

type GrepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func FilesystemTools() []NativeTool {
	return []NativeTool{
		// NewTool("read_file", "Read a file", FilesystemToolCategory, func(ctx context.Context, input ReadFileInput) (string, error) {
		// 	if input.FilePath == "" {
		// 		return "", fmt.Errorf("file path is required")
		// 	}

		// 	if !filepath.IsAbs(input.FilePath) {
		// 		return "", fmt.Errorf("file path must be absolute")
		// 	}

		// 	content, err := os.ReadFile(input.FilePath)
		// 	if err != nil {
		// 		return "", err
		// 	}
		// 	return string(content), nil
		// }),
		NewTool("write_file", "Write to a file", FilesystemToolCategory, func(ctx context.Context, input WriteFileInput) (string, error) {
			if input.FilePath == "" {
				return "", fmt.Errorf("file path is required")
			}

			if !filepath.IsAbs(input.FilePath) {
				return "", fmt.Errorf("file path must be absolute")
			}

			err := os.WriteFile(input.FilePath, []byte(input.Content), 0644)
			if err != nil {
				return "", err
			}
			return "File written successfully", nil
		}, WithReadonly(false)),
		NewTool("edit_file", "Edit a file", FilesystemToolCategory, func(ctx context.Context, input EditFileInput) (string, error) {
			// For editing a file, we'll read the file first, then write new content
			_, err := os.Stat(input.FilePath)
			if err != nil {
				return "", err
			}

			err = os.WriteFile(input.FilePath, []byte(input.Content), 0644)
			if err != nil {
				return "", err
			}
			return "File edited successfully", nil
		}, WithReadonly(false)),
		NewTool("find_files", "Search for files in the project", FilesystemToolCategory, func(ctx context.Context, input FindFilesInput) (string, error) {
			var results []string

			err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && strings.Contains(path, input.Query) {
					results = append(results, path)
				}
				return nil
			})

			if err != nil {
				return "", err
			}

			return strings.Join(results, "\n"), nil
		}),
		NewTool("grep", "Grep for a pattern in the project", FilesystemToolCategory, func(ctx context.Context, input GrepInput) (string, error) {
			if input.Pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}

			if input.Path == "" {
				return "", fmt.Errorf("path is required")
			}

			if !filepath.IsAbs(input.Path) {
				return "", fmt.Errorf("path must be absolute")
			}

			var results []string
			searchPath := "."
			if input.Path != "" {
				searchPath = input.Path
			}

			err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if !info.IsDir() {
					content, err := os.ReadFile(path)
					if err != nil {
						return nil // Skip files we can't read
					}

					if strings.Contains(string(content), input.Pattern) {
						results = append(results, path)
					}
				}
				return nil
			})

			if err != nil {
				return "", err
			}

			return strings.Join(results, "\n"), nil
		}),
		// NewTool("list_files", "List all files in the directory", FilesystemToolCategory, func(ctx context.Context, input ListFilesInput) (string, error) {
		// 	directory := input.Directory
		// 	if directory == "" {
		// 		directory = "."
		// 	}

		// 	entries, err := os.ReadDir(directory)
		// 	if err != nil {
		// 		return "", err
		// 	}

		// 	var results []string
		// 	for _, entry := range entries {
		// 		results = append(results, entry.Name())
		// 	}

		// 	return strings.Join(results, "\n"), nil
		// }),
	}
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
