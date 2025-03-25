package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type ReadFileInput struct {
	FilePath string `json:"file_path"`
}

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

type ListFilesInput struct {
	Directory string `json:"directory"`
}

func FilesystemTools() []Tool {
	return []Tool{
		NewTool("read_file", "Read a file", func(ctx context.Context, input ReadFileInput) (string, error) {
			content, err := os.ReadFile(input.FilePath)
			if err != nil {
				return "", err
			}
			return string(content), nil
		}),
		NewTool("write_file", "Write to a file", func(ctx context.Context, input WriteFileInput) (string, error) {
			err := os.WriteFile(input.FilePath, []byte(input.Content), 0644)
			if err != nil {
				return "", err
			}
			return "File written successfully", nil
		}),
		NewTool("edit_file", "Edit a file", func(ctx context.Context, input EditFileInput) (string, error) {
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
		}),
		NewTool("find_files", "Search for files in the project", func(ctx context.Context, input FindFilesInput) (string, error) {
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
		NewTool("grep", "Grep for a pattern in the project", func(ctx context.Context, input GrepInput) (string, error) {
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
		NewTool("list_files", "List all files in the directory", func(ctx context.Context, input ListFilesInput) (string, error) {
			directory := input.Directory
			if directory == "" {
				directory = "."
			}

			entries, err := os.ReadDir(directory)
			if err != nil {
				return "", err
			}

			var results []string
			for _, entry := range entries {
				results = append(results, entry.Name())
			}

			return strings.Join(results, "\n"), nil
		}),
	}
}
