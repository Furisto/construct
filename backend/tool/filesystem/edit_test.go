package filesystem

import (
	"context"
	"testing"

	"github.com/furisto/construct/backend/tool/base"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/spf13/afero"
)

func TestEditFile(t *testing.T) {
	t.Parallel()

	setup := &base.ToolTestSetup[*EditFileInput, *EditFileResult]{
		Call: func(ctx context.Context, services *base.ToolTestServices, input *EditFileInput) (*EditFileResult, error) {
			return EditFile(services.FS, input)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreFields(EditFileResult{}, "PatchInfo"),
			cmpopts.IgnoreFields(base.ToolError{}, "Suggestions"),
		},
	}

	setup.RunToolTests(t, []base.ToolTestScenario[*EditFileInput, *EditFileResult]{
		{
			Name: "escape sequence in string literal - actual newline to literal escape",
			TestInput: &EditFileInput{
				Path: "/workspace/daemon_run.go",
				Diffs: []DiffPair{
					{
						// Model generates actual newline in string
						Old: "fmt.Fprintf(cmd.OutOrStdout(), \"🤖 Starting Agent Runtime...\n\")",
						// Model also generates actual newline in diff.New
						New: "fmt.Fprintf(cmd.OutOrStdout(), \"🤖 Starting Agent Runtime...\n\") // updated",
					},
				},
			},
			SeedFilesystem: func(ctx context.Context, fs afero.Fs) {
				fs.MkdirAll("/workspace", 0755)
				// File contains literal escape sequence
				afero.WriteFile(fs, "/workspace/daemon_run.go", []byte(
					"fmt.Fprintf(cmd.OutOrStdout(), \"🤖 Starting Agent Runtime...\\n\")\n",
				), 0644)
			},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				content, err := afero.ReadFile(fs, "/workspace/daemon_run.go")
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"/workspace/daemon_run.go": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*EditFileResult]{
				Result: &EditFileResult{
					Success:              true,
					Path:                 "/workspace/daemon_run.go",
					ReplacementsMade:     1,
					ExpectedReplacements: 1,
					ValidationErrors:     nil,
					ConflictWarnings:     nil,
				},
				Filesystem: map[string]any{
					"/workspace/daemon_run.go": "fmt.Fprintf(cmd.OutOrStdout(), \"🤖 Starting Agent Runtime...\n\") // updated\n",
				},
			},
		},
		{
			Name: "multiple escape sequences in string",
			TestInput: &EditFileInput{
				Path: "/workspace/test.go",
				Diffs: []DiffPair{
					{
						Old: "s := \"line1\nline2\ttabbed\"",
						New: "s := \"line1\nline2\ttabbed\" // fixed",
					},
				},
			},
			SeedFilesystem: func(ctx context.Context, fs afero.Fs) {
				fs.MkdirAll("/workspace", 0755)
				afero.WriteFile(fs, "/workspace/test.go", []byte(
					"s := \"line1\\nline2\\ttabbed\"\n",
				), 0644)
			},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				content, err := afero.ReadFile(fs, "/workspace/test.go")
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"/workspace/test.go": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*EditFileResult]{
				Result: &EditFileResult{
					Success:              true,
					Path:                 "/workspace/test.go",
					ReplacementsMade:     1,
					ExpectedReplacements: 1,
					ValidationErrors:     nil,
					ConflictWarnings:     nil,
				},
				Filesystem: map[string]any{
					"/workspace/test.go": "s := \"line1\nline2\ttabbed\" // fixed\n",
				},
			},
		},
		{
			Name: "preserve actual newlines between code lines",
			TestInput: &EditFileInput{
				Path: "/workspace/multiline.go",
				Diffs: []DiffPair{
					{
						Old: "func hello() {\n    return 42\n}",
						New: "func hello() {\n    return 100\n}",
					},
				},
			},
			SeedFilesystem: func(ctx context.Context, fs afero.Fs) {
				fs.MkdirAll("/workspace", 0755)
				afero.WriteFile(fs, "/workspace/multiline.go", []byte(
					"func hello() {\n    return 42\n}\n",
				), 0644)
			},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				content, err := afero.ReadFile(fs, "/workspace/multiline.go")
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"/workspace/multiline.go": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*EditFileResult]{
				Result: &EditFileResult{
					Success:              true,
					Path:                 "/workspace/multiline.go",
					ReplacementsMade:     1,
					ExpectedReplacements: 1,
					ValidationErrors:     nil,
					ConflictWarnings:     nil,
				},
				Filesystem: map[string]any{
					"/workspace/multiline.go": "func hello() {\n    return 100\n}\n",
				},
			},
		},
		{
			Name: "exact match without escape sequences",
			TestInput: &EditFileInput{
				Path: "/workspace/simple.go",
				Diffs: []DiffPair{
					{
						Old: "fmt.Println(\"Hello\")",
						New: "fmt.Println(\"Hello, World\")",
					},
				},
			},
			SeedFilesystem: func(ctx context.Context, fs afero.Fs) {
				fs.MkdirAll("/workspace", 0755)
				afero.WriteFile(fs, "/workspace/simple.go", []byte(
					"fmt.Println(\"Hello\")\n",
				), 0644)
			},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				content, err := afero.ReadFile(fs, "/workspace/simple.go")
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"/workspace/simple.go": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*EditFileResult]{
				Result: &EditFileResult{
					Success:              true,
					Path:                 "/workspace/simple.go",
					ReplacementsMade:     1,
					ExpectedReplacements: 1,
					ValidationErrors:     nil,
					ConflictWarnings:     nil,
				},
				Filesystem: map[string]any{
					"/workspace/simple.go": "fmt.Println(\"Hello, World\")\n",
				},
			},
		},
		{
			Name: "whitespace differences with escape sequences",
			TestInput: &EditFileInput{
				Path: "/workspace/indented.go",
				Diffs: []DiffPair{
					{
						Old: "msg := \"test\nvalue\"",
						New: "msg := \"test\nvalue\" // fixed indent",
					},
				},
			},
			SeedFilesystem: func(ctx context.Context, fs afero.Fs) {
				fs.MkdirAll("/workspace", 0755)
				afero.WriteFile(fs, "/workspace/indented.go", []byte(
					"   msg := \"test\\nvalue\"\n",
				), 0644)
			},
			QueryFilesystem: func(fs afero.Fs) (any, error) {
				content, err := afero.ReadFile(fs, "/workspace/indented.go")
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"/workspace/indented.go": string(content),
				}, nil
			},
			Expected: base.ToolTestExpectation[*EditFileResult]{
				Result: &EditFileResult{
					Success:              true,
					Path:                 "/workspace/indented.go",
					ReplacementsMade:     1,
					ExpectedReplacements: 1,
					ValidationErrors:     nil,
					ConflictWarnings:     nil,
				},
				Filesystem: map[string]any{
					"/workspace/indented.go": "   msg := \"test\nvalue\" // fixed indent\n",
				},
			},
		},
	})
}

func TestNormalizeEscapesInString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "actual newline in double quotes",
			input:    "\"hello\nworld\"",
			expected: "\"hello\\nworld\"",
		},
		{
			name:     "actual tab in double quotes",
			input:    "\"hello\tworld\"",
			expected: "\"hello\\tworld\"",
		},
		{
			name:     "actual carriage return in double quotes",
			input:    "\"hello\rworld\"",
			expected: "\"hello\\rworld\"",
		},
		{
			name:     "multiple escape sequences",
			input:    "\"line1\nline2\ttabbed\"",
			expected: "\"line1\\nline2\\ttabbed\"",
		},
		{
			name:     "preserves literal escape sequences",
			input:    "\"hello\\nworld\"",
			expected: "\"hello\\nworld\"",
		},
		{
			name:     "escapes actual newlines but not in code",
			input:    "func test() {\n\treturn \"hello\nworld\"\n}",
			expected: "func test() {\n\treturn \"hello\\nworld\"\n}",
		},
		{
			name:     "single quotes string",
			input:    "'hello\nworld'",
			expected: "'hello\\nworld'",
		},
		{
			name:     "backtick string",
			input:    "`hello\nworld`",
			expected: "`hello\\nworld`",
		},
		{
			name:     "no escape sequences needed",
			input:    "fmt.Println(\"hello\")",
			expected: "fmt.Println(\"hello\")",
		},
		{
			name:     "mixed quoted strings",
			input:    "\"first\nstring\" and 'second\tstring'",
			expected: "\"first\\nstring\" and 'second\\tstring'",
		},
		{
			name:     "empty string",
			input:    "\"\"",
			expected: "\"\"",
		},
		{
			name:     "string with already escaped sequences",
			input:    "\"already\\nescaped\"",
			expected: "\"already\\nescaped\"",
		},
		{
			name:     "emoji in string with escape",
			input:    "\"🤖 Agent\nRuntime\"",
			expected: "\"🤖 Agent\\nRuntime\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeEscapesInString(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeEscapesInString(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
