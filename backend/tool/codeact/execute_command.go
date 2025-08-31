package codeact

import (
	"fmt"

	"github.com/grafana/sobek"

	"github.com/furisto/construct/backend/tool/system"
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

## Commiting with Git
When the user asks you to create a new git commit, follow these steps carefully:

### Safety Requirements
- Never force push without explicit user instruction and warning
- Never %[2]sgit reset --hard%[2]s or %[2]sgit clean%[2]s without confirmation
- Warn if staging files >50MB, suggest Git LFS or .gitignore
- Check for sensitive patterns: %[2]s.env%[2]s, secrets, keys, passwords in filenames/content etc.
- Mention when committing binary files

### Analysis Workflow
1. **Assess repository state**: Run %[2]sgit status%[2]s, %[2]sgit diff --stat%[2]s, %[2]sgit diff --cached --stat%[2]s
2. **Analyze changes**: List modified files, categorize change type (feature/fix/refactor/docs/test)
3. **Determine motivation**: Why were these changes made?
4. **Security scan**: Check staged files for sensitive information
5. **Check repository style**: Review recent commits (%[2]sgit log --format='%h - %s%n%b%n---' -6%[2]s) for message patterns

### Commit Message Rules
- Focus on "why" not "what" - explain purpose, not just actions
- Use specific verbs: %[2]sgit add%[2]s (new), %[2]sgit fix%[2]s (bugs), %[2]sgit update%[2]s (enhance existing), %[2]sgit refactor%[2]s, %[2]sgit remove%[2]s
- Avoid generic terms like "Update" without context
- Match repository's existing style (capitalization, tense, format)

### Error Handling
- **No changes**: Show status, suggest staging files
- **Merge conflicts**: Guide through resolution before committing
- **Hook failures**: Show output, ask how to proceed
- **Large/sensitive files**: Require explicit confirmation

### Execution Process
1. Perform safety checks and analysis
2. If pre-commit hooks modify fail, retry commit ONCE
3. If commit succeeds but hooks modified files, amend: %[2]sgit add . && git commit --amend --no-edit%[2]s
4. Verify commit was created successfully

### Key Success Factors
- Craft meaningful messages that reflect actual changes and their purpose
- Adapt to each repository's established commit conventions. The commit message should close match the style of the repository
- After you have created the commit, do not explain to the user why the commit message follows the rules that have been explained to you.

## Usage Examples
%[1]s
// Simple command with error checking
const result = execute_command("ls -la");
if (result.exitCode !== 0) {
print(Error: ${result.stderr});
return;
}
// Git operations
# TURN 1
// BATCH REPOSITORY STATE ANALYSIS: Get complete picture first
const gitStatus = execute_command("git status --porcelain");
print("=== STATUS SUMMARY ===");
print(gitStatus.stdout);

const gitDiffStat = execute_command("git diff --stat");
print("=== DIFF SUMMARY ===");
print(gitDiffStat.stdout);

const gitDiffCachedStat = execute_command("git diff --cached --stat");
print("=== CACHED DIFF SUMMARY ===");
print(gitDiffCachedStat.stdout);

const gitDiffCached = execute_command("git diff --cached");
print("=== CACHED DIFF ===");
print(gitDiffCached.stdout);

const gitLog = execute_command("git log --format='%h - %s%n%b%n---' -6");
print("=== LAST 6 COMMITS ===");
print(gitLog.stdout);

# TURN 2
const commitMessage = %[2]sEnhance read_file tool with line number prefixing
... Rest of message depends on style found in the last commits.

Co-authored-by: construct-agent <agent@construct.sh>%[2]s

const commitResult = execute_command(%[2]sgit commit -m "$commitMessage"%[2]s);
// Verify the commit was created successfully
if (commitResult.exitCode === 0) {
  print("✅ Commit successful");
  
  const lastCommit = execute_command("git log --oneline -1");
  print("=== CREATED COMMIT ===");
  print(lastCommit.stdout);
  
} else {
  print("❌ Commit failed");
  print("Exit code:", commitResult.exitCode);
  print("Errors:", commitResult.stderr);
}

// Development commands
const npmInstall = execute_command("npm install", true);
if (npmInstall.exitCode === 0) {
execute_command("npm run dev", false);
}
%[1]s
`

func NewExecuteCommandTool() Tool {
	return NewOnDemandTool(
		"execute_command",
		fmt.Sprintf(executeCommandDescription, "```", "`"),
		executeCommandInput,
		executeCommandHandler,
	)
}

func executeCommandInput(session *Session, args []sobek.Value) (any, error) {
	if len(args) < 1 {
		return nil, nil
	}

	return &system.ExecuteCommandInput{
		Command:          args[0].String(),
		WorkingDirectory: session.Task.ProjectDirectory,
	}, nil
}

func executeCommandHandler(session *Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		rawInput, err := executeCommandInput(session, call.Arguments)
		if err != nil {
			session.Throw(err)
		}
		input := rawInput.(*system.ExecuteCommandInput)

		result, err := system.ExecuteCommand(input)
		if err != nil {
			session.Throw(err)
		}

		SetValue(session, "result", result)
		return session.VM.ToValue(result)
	}
}
