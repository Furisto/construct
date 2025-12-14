package codeact

import (
	"fmt"
	"reflect"

	"github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/subtask"
	"github.com/grafana/sobek"
)

const spawnTaskDescription = `
## Description
Create a subtask and assign it to another agent. The subtask runs concurrently and independently. Returns immediately with the task ID.

Subtasks are useful for delegating specialized work to other agents, running multiple tasks in parallel, or implementing multi-phase workflows.

## Available Agents
- **scout**: Specialized agent for discovering and analyzing files relevant to a coding task. Use when you need to:
  - Identify which files in the workspace are relevant to a specific feature or functionality
  - Find files by name patterns, content, or project structure
  - Get a structured analysis of relevant files grouped by type (implementation, tests, config, docs, utilities)
  - Understand the codebase layout before making changes
  The scout agent works independently and cannot ask questions, so provide clear, complete task descriptions.

## Parameters
- **agent** (string, required): Agent name or ID to assign to the subtask
- **prompt** (string, required): Initial message/instructions for the subtask

## Expected Output
Returns an object with the task ID:
%[1]s
{
  "task_id": "uuid-string"
}
%[1]s

## IMPORTANT USAGE NOTES
- **Use await_tasks**: Call await_tasks() to wait for subtask completion and retrieve results

## When to use
- **Task specialization**: Delegate work to agents with specific expertise
- **Parallel processing**: Process multiple items concurrently
- **Multi-phase workflows**: Break complex tasks into sequential stages
- **Load distribution**: Spread work across multiple agents

## Usage Examples

### Single subtask delegation
%[1]s
const task = spawn_task({
    agent: "reviewer",
    prompt: "Review src/auth.go for security issues"
});
print("Spawned task: " + task.task_id);

const result = await_tasks({ task_ids: [task.task_id] });
%[1]s

### Parallel processing
%[1]s
const dirs = ["pkg/api", "pkg/reconciler", "pkg/tools"];
const tasks = dirs.map(dir => spawn_task({
    agent: "summarizer",
    prompt: "Summarize the code in " + dir
}));

const taskIds = tasks.map(t => t.task_id);
const results = await_tasks({ task_ids: taskIds });
%[1]s

### Staged workflow
%[1]s
// Phase 1: Planning
const planTask = spawn_task({ 
    agent: "planner", 
    prompt: "Create a refactoring plan for the auth module" 
});
const planResult = await_tasks({ task_ids: [planTask.task_id] });

// Phase 2: Execute each step
const steps = planResult.results[0].messages[0].content.steps;
const execTasks = steps.map(step => spawn_task({ 
    agent: "coder", 
    prompt: step 
}));
await_tasks({ task_ids: execTasks.map(t => t.task_id) });
%[1]s
`

func NewSpawnTaskTool() Tool {
	return NewOnDemandTool(
		"spawn_task",
		fmt.Sprintf(spawnTaskDescription, "```", "`"),
		spawnTaskInput,
		spawnTaskHandler,
	)
}

func spawnTaskInput(session *Session, args []sobek.Value) (any, error) {
	if len(args) < 1 {
		return nil, base.NewCustomError(base.InvalidInput.String(), spawnTaskSuggestions)
	}

	maybeObjectBasedInput := args[0].ToObject(session.VM)
	if maybeObjectBasedInput == nil || maybeObjectBasedInput == sobek.Undefined() {
		return nil, base.NewCustomError(base.InvalidInput.String(), spawnTaskSuggestions)
	}

	agentVal := maybeObjectBasedInput.Get("agent")
	if agentVal == nil || agentVal == sobek.Undefined() || agentVal.ExportType() != reflect.TypeOf("") {
		return nil, base.NewCustomError(base.InvalidInput.String(), spawnTaskSuggestions)
	}

	promptVal := maybeObjectBasedInput.Get("prompt")
	if promptVal == nil || promptVal == sobek.Undefined() || promptVal.ExportType() != reflect.TypeOf("") {
		return nil, base.NewCustomError(base.InvalidInput.String(), spawnTaskSuggestions)
	}

	return &subtask.SpawnTaskInput{
		Agent:  agentVal.String(),
		Prompt: promptVal.String(),
	}, nil
}

func spawnTaskHandler(session *Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		input, err := spawnTaskInput(session, call.Arguments)
		if err != nil {
			session.Throw(err)
		}

		result, err := subtask.SpawnTask(
			session.Context,
			session.Memory,
			session.Bus,
			session.Task.ID,
			input.(*subtask.SpawnTaskInput),
		)
		if err != nil {
			session.Throw(err)
		}

		SetValue(session, "result", result)
		return session.VM.ToValue(result)
	}
}

var spawnTaskSuggestions = []string{
	"Ensure that you provide the correct input arguments as specified in the tool description",
	"- **agent** (string, required): Agent name or ID to assign to the subtask",
	"- **prompt** (string, required): Initial message/instructions for the subtask",
}
