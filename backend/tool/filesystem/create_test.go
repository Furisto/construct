package filesystem

import (
	"context"
	"testing"

	"github.com/furisto/construct/backend/tool/base"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/spf13/afero"
)

func TestCreateFile(t *testing.T) {
	setup := &base.ToolTestSetup[*CreateFileInput, *CreateFileResult]{
		Call: func(ctx context.Context, services *base.ToolTestServices, input *CreateFileInput) (*CreateFileResult, error) {
			return CreateFile(services.FS, input)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreFields(base.ToolError{}, "Suggestions"),
		},
	}

	setup.RunToolTests(t, []base.ToolTestScenario[*CreateFileInput, *CreateFileResult]{
		{
			Name:      "successful creation of new text file",
			TestInput: &CreateFileInput{Path: "/workspace/new_file.txt", Content: "Hello, World!\nThis is a new file."},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/new_file.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/new_file.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/new_file.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/new_file.txt": "Hello, World!\nThis is a new file.",
				},
			},
		},
		{
			Name:           "successful overwriting of existing file",
			TestInput:      &CreateFileInput{Path: "/workspace/test.txt", Content: "New content for existing file"},
			SeedFilesystem: seedCreateFileTestFilesystem,
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/test.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/test.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/test.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: true,
				},
				Filesystem: map[string]any{
					"/workspace/test.txt": "New content for existing file",
				},
			},
		},
		{
			Name:      "successful creation of empty file",
			TestInput: &CreateFileInput{Path: "/workspace/empty_new.txt", Content: ""},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/empty_new.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/empty_new.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/empty_new.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/empty_new.txt": "",
				},
			},
		},
		{
			Name:      "successful creation with nested directory structure",
			TestInput: &CreateFileInput{Path: "/workspace/deep/nested/path/new_file.txt", Content: "File in nested directory"},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/deep/nested/path/new_file.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/deep/nested/path/new_file.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/deep/nested/path/new_file.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/deep/nested/path/new_file.txt": "File in nested directory",
				},
			},
		},
		{
			Name:      "successful creation of JSON file",
			TestInput: &CreateFileInput{Path: "/workspace/new_config.json", Content: `{"name": "new_project", "version": "2.0.0", "description": "A new project"}`},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/new_config.json"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/new_config.json")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/new_config.json": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/new_config.json": `{"name": "new_project", "version": "2.0.0", "description": "A new project"}`,
				},
			},
		},
		{
			Name:      "successful creation with unicode content",
			TestInput: &CreateFileInput{Path: "/workspace/unicode_new.txt", Content: "¡Hola mundo! 🌍 Здравствуй мир! こんにちは世界！"},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/unicode_new.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/unicode_new.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/unicode_new.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/unicode_new.txt": "¡Hola mundo! 🌍 Здравствуй мир! こんにちは世界！",
				},
			},
		},
		{
			Name:      "successful creation with special characters in filename",
			TestInput: &CreateFileInput{Path: "/workspace/special-file_with@symbols#new.txt", Content: "File with special characters in name"},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/special-file_with@symbols#new.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/special-file_with@symbols#new.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/special-file_with@symbols#new.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/special-file_with@symbols#new.txt": "File with special characters in name",
				},
			},
		},
		{
			Name:      "successful creation with large content",
			TestInput: &CreateFileInput{Path: "/workspace/large_new.txt", Content: generateLargeCreateFileContent()},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/large_new.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/large_new.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/large_new.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/large_new.txt": generateLargeCreateFileContent(),
				},
			},
		},
		{
			Name:      "successful creation with binary-like content",
			TestInput: &CreateFileInput{Path: "/workspace/binary_new.bin", Content: string([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD})},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/binary_new.bin"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/binary_new.bin")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/binary_new.bin": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/binary_new.bin": string([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD}),
				},
			},
		},
		{
			Name:      "successful creation with multiline content and special formatting",
			TestInput: &CreateFileInput{Path: "/workspace/formatted.txt", Content: "Line 1\n\tIndented line 2\n    Spaced line 3\n\nEmpty line above\r\nWindows line ending"},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/formatted.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/formatted.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/formatted.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/formatted.txt": "Line 1\n\tIndented line 2\n    Spaced line 3\n\nEmpty line above\r\nWindows line ending",
				},
			},
		},
		{
			Name:           "successful overwriting of JSON file with new structure",
			TestInput:      &CreateFileInput{Path: "/workspace/config.json", Content: `{"name": "updated_project", "version": "3.0.0", "author": "John Doe", "license": "MIT"}`},
			SeedFilesystem: seedCreateFileTestFilesystem,
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/config.json"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/config.json")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/config.json": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: true,
				},
				Filesystem: map[string]any{
					"/workspace/config.json": `{"name": "updated_project", "version": "3.0.0", "author": "John Doe", "license": "MIT"}`,
				},
			},
		},
		{
			Name:      "relative path error",
			TestInput: &CreateFileInput{Path: "relative/path/file.txt", Content: "Some content"},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Error: base.NewError(base.PathIsNotAbsolute, "path", "relative/path/file.txt"),
			},
		},
		{
			Name:      "current directory relative path error",
			TestInput: &CreateFileInput{Path: "./file.txt", Content: "Some content"},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Error: base.NewError(base.PathIsNotAbsolute, "path", "./file.txt"),
			},
		},
		{
			Name:      "parent directory relative path error",
			TestInput: &CreateFileInput{Path: "../file.txt", Content: "Some content"},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Error: base.NewError(base.PathIsNotAbsolute, "path", "../file.txt"),
			},
		},
		{
			Name:           "path is directory error",
			TestInput:      &CreateFileInput{Path: "/workspace/src", Content: "Cannot write to directory"},
			SeedFilesystem: seedCreateFileTestFilesystem,
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Error: base.NewError(base.PathIsDirectory, "path", "/workspace/src"),
			},
		},
		{
			Name:      "successful creation at root level",
			TestInput: &CreateFileInput{Path: "/root_file.txt", Content: "File at root level"},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/root_file.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/root_file.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/root_file.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/root_file.txt": "File at root level",
				},
			},
		},
		{
			Name:      "successful creation with very long path",
			TestInput: &CreateFileInput{Path: "/workspace/very/deep/nested/directory/structure/with/many/levels/that/goes/quite/far/down/the/tree/final_file.txt", Content: "Deep nested file"},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/very/deep/nested/directory/structure/with/many/levels/that/goes/quite/far/down/the/tree/final_file.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/very/deep/nested/directory/structure/with/many/levels/that/goes/quite/far/down/the/tree/final_file.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/very/deep/nested/directory/structure/with/many/levels/that/goes/quite/far/down/the/tree/final_file.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/very/deep/nested/directory/structure/with/many/levels/that/goes/quite/far/down/the/tree/final_file.txt": "Deep nested file",
				},
			},
		},
		{
			Name:      "successful creation with path containing dots",
			TestInput: &CreateFileInput{Path: "/workspace/file.with.dots.in.name.txt", Content: "File with dots in name"},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				if _, err := fs.Stat("/workspace/file.with.dots.in.name.txt"); err != nil {
					return nil, err
				}

				content, err := afero.ReadFile(fs, "/workspace/file.with.dots.in.name.txt")
				if err != nil {
					return nil, err
				}

				return map[string]any{
					"/workspace/file.with.dots.in.name.txt": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*CreateFileResult]{
				Result: &CreateFileResult{
					Overwritten: false,
				},
				Filesystem: map[string]any{
					"/workspace/file.with.dots.in.name.txt": "File with dots in name",
				},
			},
		},
	})
}

func seedCreateFileTestFilesystem(ctx context.Context, fs afero.Fs) {
	fs.MkdirAll("/workspace/src", 0755)
	fs.MkdirAll("/workspace/deep/nested", 0755)

	afero.WriteFile(fs, "/workspace/test.txt", []byte("Original content in test file"), 0644)
	afero.WriteFile(fs, "/workspace/empty.txt", []byte(""), 0644)
	afero.WriteFile(fs, "/workspace/config.json", []byte(`{"name": "original", "version": "1.0.0"}`), 0644)
	afero.WriteFile(fs, "/workspace/src/main.go", []byte(`package main

import "fmt"

func main() {
	fmt.Println("Original Hello, World!")
}`), 0644)
	afero.WriteFile(fs, "/workspace/binary.bin", []byte{0x00, 0x01, 0x02, 0x03, 0xFF}, 0644)
	afero.WriteFile(fs, "/workspace/special-file_with@symbols.txt", []byte("Original special file content"), 0644)
	afero.WriteFile(fs, "/workspace/unicode.txt", []byte("Original unicode: Hello 世界! 🌍"), 0644)
	afero.WriteFile(fs, "/workspace/large.txt", []byte("Original large file content"), 0644)
}

func generateLargeCreateFileContent() string {
	content := "Large file content for testing:\n"
	for i := 0; i < 100; i++ {
		content += "This is line " + string(rune(i%10+'0')) + " of a large file for create_file testing purposes. It contains some repeated content to make it larger.\n"
	}
	return content
}
