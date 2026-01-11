package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		options  *RenderOptions
		expected []string
	}{
		{
			name: "single struct",
			input: &AgentDisplay{
				ID:          "123",
				Name:        "test-agent",
				Description: "A test agent",
				Model:       "gpt-4",
			},
			options: &RenderOptions{Format: OutputFormatMarkdown},
			expected: []string{
				"**ID:** 123",
				"**Name:** test-agent",
				"**Description:** A test agent",
				"**Model:** gpt-4",
			},
		},
		{
			name: "slice of structs",
			input: []*AgentDisplay{
				{ID: "1", Name: "agent-one", Model: "gpt-4"},
				{ID: "2", Name: "agent-two", Model: "claude"},
			},
			options: &RenderOptions{Format: OutputFormatMarkdown},
			expected: []string{
				"**ID:** 1",
				"**Name:** agent-one",
				"---",
				"**ID:** 2",
				"**Name:** agent-two",
			},
		},
		{
			name:     "nil input",
			input:    nil,
			options:  &RenderOptions{Format: OutputFormatMarkdown},
			expected: []string{},
		},
		{
			name:     "empty slice",
			input:    []*AgentDisplay{},
			options:  &RenderOptions{Format: OutputFormatMarkdown},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := renderMarkdown(tt.input, tt.options)
			if err != nil {
				t.Fatalf("renderMarkdown() error = %v", err)
			}

			w.Close()
			var buf bytes.Buffer
			io.Copy(&buf, r)
			os.Stdout = old

			output := buf.String()

			for _, exp := range tt.expected {
				if !strings.Contains(output, exp) {
					t.Errorf("expected output to contain %q, got:\n%s", exp, output)
				}
			}
		})
	}
}

func TestOutputFormatSet(t *testing.T) {
	tests := []struct {
		input    string
		expected OutputFormat
		wantErr  bool
	}{
		{"json", OutputFormatJSON, false},
		{"yaml", OutputFormatYAML, false},
		{"table", OutputFormatTable, false},
		{"card", OutputFormatCard, false},
		{"markdown", OutputFormatMarkdown, false},
		{"md", OutputFormatMarkdown, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var f OutputFormat
			err := f.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("OutputFormat.Set(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && f != tt.expected {
				t.Errorf("OutputFormat.Set(%q) = %v, want %v", tt.input, f, tt.expected)
			}
		})
	}
}
