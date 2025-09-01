package system

import (
	"context"
	"testing"

	"github.com/furisto/construct/backend/tool/base"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestExecuteCommand(t *testing.T) {
	t.Parallel()

	setup := &base.ToolTestSetup[*ExecuteCommandInput, *ExecuteCommandResult]{
		Call: func(ctx context.Context, services *base.ToolTestServices, input *ExecuteCommandInput) (*ExecuteCommandResult, error) {
			return ExecuteCommand(input)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreFields(base.ToolError{}, "Suggestions"),
		},
	}

	setup.RunToolTests(t, []base.ToolTestScenario[*ExecuteCommandInput, *ExecuteCommandResult]{
		{
			Name:      "successful command with output",
			TestInput: &ExecuteCommandInput{Command: "echo 'Hello World'"},
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Result: &ExecuteCommandResult{
					Command:  "echo 'Hello World'",
					Stdout:   "Hello World\n",
					Stderr:   "",
					ExitCode: 0,
				},
			},
		},
		{
			Name:      "successful command without output",
			TestInput: &ExecuteCommandInput{Command: "true"}, // Command that always succeeds and produces no output
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Result: &ExecuteCommandResult{
					Command:  "true",
					Stdout:   "",
					Stderr:   "",
					ExitCode: 0,
				},
			},
		},
		{
			Name:      "command that fails",
			TestInput: &ExecuteCommandInput{Command: "false"}, // Command that always fails
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Result: &ExecuteCommandResult{
					Command:  "false",
					Stdout:   "",
					Stderr:   "",
					ExitCode: 1,
				},
			},
		},
		{
			Name:      "command with stderr output",
			TestInput: &ExecuteCommandInput{Command: "echo 'error message' >&2"},
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Result: &ExecuteCommandResult{
					Command:  "echo 'error message' >&2",
					Stdout:   "error message\n", // CombinedOutput captures both stdout and stderr
					Stderr:   "",
					ExitCode: 0,
				},
			},
		},
		{
			Name:      "command with both stdout and stderr",
			TestInput: &ExecuteCommandInput{Command: "echo 'stdout'; echo 'stderr' >&2"},
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Result: &ExecuteCommandResult{
					Command:  "echo 'stdout'; echo 'stderr' >&2",
					Stdout:   "stdout\nstderr\n", // CombinedOutput combines both
					Stderr:   "",
					ExitCode: 0,
				},
			},
		},
		{
			Name:      "nonexistent command",
			TestInput: &ExecuteCommandInput{Command: "nonexistent_command_xyz_12345"},
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Error: base.NewCustomError("error executing command", []string{
					"Check if the command is valid and executable.",
					"Ensure the command is properly formatted for the target operating system.",
				}),
			},
		},
		{
			Name:      "command with exit code 2",
			TestInput: &ExecuteCommandInput{Command: "exit 2"},
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Result: &ExecuteCommandResult{
					Command:  "exit 2",
					Stdout:   "",
					Stderr:   "",
					ExitCode: 2,
				},
			},
		},
		{
			Name:      "command with special characters",
			TestInput: &ExecuteCommandInput{Command: "echo 'Hello \"World\" with $pecial chars!'"},
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Result: &ExecuteCommandResult{
					Command:  "echo 'Hello \"World\" with $pecial chars!'",
					Stdout:   "Hello \"World\" with $pecial chars!\n",
					Stderr:   "",
					ExitCode: 0,
				},
			},
		},
		{
			Name:      "multiline command",
			TestInput: &ExecuteCommandInput{Command: "echo 'line1'; echo 'line2'"},
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Result: &ExecuteCommandResult{
					Command:  "echo 'line1'; echo 'line2'",
					Stdout:   "line1\nline2\n",
					Stderr:   "",
					ExitCode: 0,
				},
			},
		},
		{
			Name:      "empty command",
			TestInput: &ExecuteCommandInput{Command: ""},
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Error: base.NewError(base.InvalidInput, "command", "command is required"),
			},
		},
		{
			Name:      "command with spaces",
			TestInput: &ExecuteCommandInput{Command: "echo hello world"},
			Expected: base.ToolTestExpectation[*ExecuteCommandResult]{
				Result: &ExecuteCommandResult{
					Command:  "echo hello world",
					Stdout:   "hello world\n",
					Stderr:   "",
					ExitCode: 0,
				},
			},
		},
	})
}
