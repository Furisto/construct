package tool

import (
	"fmt"
	"os/exec"

	"github.com/grafana/sobek"

	"github.com/furisto/construct/backend/tool/codeact"
)

const executeCommandDescription = `
## Description
The execute_command tool allows you to run system commands directly from your CodeAct JavaScript program. Use this tool when you need to interact with the system environment, file operations, execute CLI tools, or perform operations that require shell access. This tool provides a bridge between your code and the underlying operating system's command line interface.

## Parameters
- **command** (string, required): The CLI command to execute. This should be valid for the current operating system. Ensure the command is properly formatted and does not contain any harmful instructions.

## Expected Output
Returns an object containing the command's output:
%[1]s
{
  "stdout": "Standard output from the command (if any)",
  "stderr": "Standard error output (if any)",
  "exitCode": 0, // The exit code of the command (0 typically indicates success)
  "command": "The command that was executed"
}
%[1]s

## CRITICAL REQUIREMENTS
- **Command safety**: Always ensure commands are safe and appropriate for the user's environment
- **Error handling**: Always check the exit code and stderr to determine if the command was successful
- **Prefer specialized tools**: You should only use this tool if it would be impractical to use a more specialized tool.
%[1]s
  const result = execute_command("git status");
  if (result.exitCode !== 0) {
    print("Command failed: ${result.stderr}");
    return;
  }
%[1]s
- **Command formatting**: Ensure commands are properly formatted for the target operating system
%[1]s
  // For Windows and Unix-compatible systems
  execute_command("echo Hello, World!");
  
  // For command chaining
  execute_command("cd /path/to/dir && npm install"); // Unix-like
  execute_command("cd /path/to/dir & npm install");  // Windows
%[1]s
- IMPORTANT: You are not allowed to run any destructive commands. You should always use special tools for destructive commands.

## When to use
- **System interactions**: When you need to access system functionality not available through JavaScript APIs
- **File and directory operations**: For complex file operations beyond basic read/write
- **Development tools**: To run build processes, dev servers, or package managers
- **Git operations**: For source control management
- **Network utilities**: For ping, curl, wget, and other network tools
- **Process management**: To start, stop, or monitor system processes

## Usage Examples
%[1]s
// Simple command with error checking
const result = execute_command("ls -la");
if (result.exitCode !== 0) {
print(Error: ${result.stderr});
return;
}
// Git operations
const gitStatus = execute_command("git status --porcelain");
if (gitStatus.stdout.trim() === "") {
// Repository is clean, create and checkout new branch
execute_command("git checkout -b feature/new-feature", true);
}
// Development commands
const npmInstall = execute_command("npm install", true);
if (npmInstall.exitCode === 0) {
execute_command("npm run dev", false);
}
%[1]s
`

type ExecuteCommandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
	Command  string `json:"command"`
}

type ExecuteCommandInput struct {
	Command string
}

func (e *ExecuteCommandInput) Validate() error {
	
	return nil
}

func NewExecuteCommandTool() codeact.Tool {
	return codeact.NewOnDemandTool(
		"execute_command",
		fmt.Sprintf(executeCommandDescription, "```"),
		executeCommandHandler,
	)
}

func executeCommandHandler(session *codeact.Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		command := call.Argument(0).String()

		input := &ExecuteCommandInput{Command: command}
		result, err := executeCommand(input)
		if err != nil {
			session.Throw(err)
		}

		return session.VM.ToValue(result)
	}
}

func executeCommand(input *ExecuteCommandInput) (*ExecuteCommandResult, error) {
	if input.Command == "" {
		return nil, codeact.NewError(codeact.InvalidArgument, "command", "command is required")
	}

	script := fmt.Sprintf(`#!/bin/sh
		set -euo pipefail
		%s
		`,
		input.Command,
	)

	cmd := exec.Command("/bin/sh", "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, codeact.NewCustomError("error executing command", []string{
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
