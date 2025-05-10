package tool

import (
	"fmt"
	"os/exec"

	"github.com/grafana/sobek"
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

## Common Errors and Solutions
- **Command not found**: Ensure the command exists on the user's system and is in the PATH
  - Solution: Check if the command is installed, or provide installation instructions
- **Permission denied**: The command requires elevated privileges
  - Solution: Inform the user they may need to run with appropriate permissions
- **Path issues**: Specified files or directories don't exist
  - Solution: Verify paths exist before running commands that depend on them
%[1]s
  // Check if directory exists before using it
  const dirCheckResult = execute_command("test -d /path/to/dir && echo exists");
  if (!dirCheckResult.stdout.includes("exists")) {
    print("Directory doesn't exist, creating it...");
    execute_command("mkdir -p /path/to/dir");
  }
%[1]s

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

func NewExecuteCommandTool() CodeActTool {
	return NewOnDemandTool(
		"execute_command",
		fmt.Sprintf(executeCommandDescription, "```"),
		executeCommandHandler,
	)
}

func executeCommandHandler(session *CodeActSession) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		command := call.Argument(0).String()

		cmd := exec.Command(command)
		output, err := cmd.CombinedOutput()
		if err != nil {
			session.Throw(NewCustomError("error executing command", []string{
				"Check if the command is valid and executable.",
				"Ensure the command is properly formatted for the target operating system.",
			}, "command", command, "error", err))
		}

		return session.VM.ToValue(ExecuteCommandResult{
			Command:  command,
			Stdout:   string(output),
			Stderr:   "",
			ExitCode: cmd.ProcessState.ExitCode(),
		})
	}
}
