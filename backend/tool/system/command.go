package system

import (
	"fmt"
	"os/exec"

	"github.com/furisto/construct/backend/tool/base"
)

type ExecuteCommandInput struct {
	Command          string
	WorkingDirectory string
}

type ExecuteCommandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
	Command  string `json:"command"`
}

func ExecuteCommand(input *ExecuteCommandInput) (*ExecuteCommandResult, error) {
	if input.Command == "" {
		return nil, base.NewError(base.InvalidInput, "command", "command is required")
	}

	script := fmt.Sprintf(`#!/bin/sh
		set -euo pipefail
		%s
		`,
		input.Command,
	)

	cmd := exec.Command("/bin/sh", "-c", script)
	if input.WorkingDirectory != "" {
		cmd.Dir = input.WorkingDirectory
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, base.NewCustomError("error executing command", []string{
			"Check if the command is valid and executable.",
			"Ensure the command is properly formatted for the target operating system.",
		}, "command", input.Command, "error", err)
	}

	return &ExecuteCommandResult{
		Command:  input.Command,
		Stdout:   string(output),
		Stderr:   "",
		ExitCode: cmd.ProcessState.ExitCode(),
	}, nil
}
