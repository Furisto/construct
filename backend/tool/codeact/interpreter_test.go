package codeact

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/afero"
)

func TestInterpreter(t *testing.T) {
	tests := []struct {
		Name   string
		Script string
		Tools  []Tool
		FS     afero.Fs
	}{
		// {
		// 	Name: "read_file",
		// 	Script: `try {
		// 		read_file("test.txt");
		// 	} catch (err) {
		// 		print(err);
		// 	}`,
		// 	Tools: []Tool{NewReadFileTool(), NewPrintTool()},
		// 	FS:    afero.NewMemMapFs(),
		// },
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			interpreter := NewInterpreter(test.Tools...)
			args := InterpreterArgs{
				Script: test.Script,
			}

			jsonArgs, err := json.Marshal(args)
			if err != nil {
				t.Fatalf("error marshalling args: %v", err)
			}

			result, err := interpreter.Run(context.Background(), test.FS, jsonArgs)
			if err != nil {
				t.Fatalf("error running interpreter: %v", err)
			}
			if result != "test" {
				t.Fatalf("expected result to be 'test', got '%s'", result)
			}
		})
	}
}
