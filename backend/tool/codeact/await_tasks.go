package codeact

import (
	"fmt"
	"reflect"

	"github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/subtask"
	"github.com/grafana/sobek"
)

const awaitTasksDescription = `
## Description
Wait for subtasks to complete and collect their messages. Blocks until all specified subtasks reach completion or timeout.

A subtask is considered complete when it reaches the AwaitInput phase (the agent has finished its work and is waiting for new input).

## Parameters
- **task_ids** (array of strings, required): Array of task IDs to wait for (from spawn_task results)
- **timeout** (number, optional): Timeout in seconds (default: 300)

## Expected Output
Returns an object with results for each task:
%[1]s
{
  "results": [
    {
      "task_id": "uuid-string",
      "messages": [
        { /* message content from send_message */ }
      ]
    }
  ]
}
%[1]s

## IMPORTANT USAGE NOTES
- **Blocks execution**: This tool waits for subtasks to complete before returning
- **Ownership validation**: Can only await tasks spawned by the current task
- **Message order**: Messages are returned in the order they were sent
- **Multiple messages**: Each subtask can send multiple messages; all are collected
- **Timeout handling**: If timeout occurs, an error is thrown with incomplete task IDs

## When to use
- **After spawning subtasks**: Always call after spawn_task to get results
- **Parallel processing**: Wait for multiple concurrent subtasks
- **Sequential workflows**: Wait for one phase before starting the next

## Usage Examples

### Single subtask
%[1]s
const task = spawn_task({
    agent: "reviewer",
    prompt: "Review src/auth.go for security issues"
});

const result = await_tasks({ task_ids: [task.task_id] });
const messages = result.results[0].messages;

for (const msg of messages) {
    print("Review finding: " + JSON.stringify(msg));
}
%[1]s

### Multiple parallel subtasks
%[1]s
const dirs = ["pkg/api", "pkg/reconciler", "pkg/tools"];
const tasks = dirs.map(dir => spawn_task({
    agent: "summarizer",
    prompt: "Summarize the code in " + dir
}));

const taskIds = tasks.map(t => t.task_id);
const results = await_tasks({ task_ids: taskIds });

// Process results in order
for (let i = 0; i < results.results.length; i++) {
    print("Summary for " + dirs[i] + ":");
    for (const msg of results.results[i].messages) {
        print(JSON.stringify(msg));
    }
}
%[1]s

### With custom timeout
%[1]s
const task = spawn_task({
    agent: "analyzer",
    prompt: "Perform deep analysis of the entire codebase"
});

// Give it 10 minutes instead of default 5 minutes
const result = await_tasks({ 
    task_ids: [task.task_id],
    timeout: 600
});
%[1]s

### Sequential workflow
%[1]s
// Phase 1: Planning
const planTask = spawn_task({ 
    agent: "planner", 
    prompt: "Create a refactoring plan" 
});
const planResult = await_tasks({ task_ids: [planTask.task_id] });
const plan = planResult.results[0].messages[0];

// Phase 2: Execution based on plan
const steps = plan.steps;
const execTasks = steps.map(step => spawn_task({ 
    agent: "coder", 
    prompt: "Implement: " + step 
}));
const execResults = await_tasks({ 
    task_ids: execTasks.map(t => t.task_id) 
});

// Phase 3: Verification
const verifyTask = spawn_task({
    agent: "tester",
    prompt: "Verify all changes work correctly"
});
await_tasks({ task_ids: [verifyTask.task_id] });
%[1]s
`

func NewAwaitTasksTool() Tool {
	return NewOnDemandTool(
		"await_tasks",
		fmt.Sprintf(awaitTasksDescription, "```", "`"),
		awaitTasksInput,
		awaitTasksHandler,
	)
}

func awaitTasksInput(session *Session, args []sobek.Value) (any, error) {
	if len(args) < 1 {
		return nil, base.NewCustomError(base.InvalidInput.String(), awaitTasksSuggestions)
	}

	maybeObjectBasedInput := args[0].ToObject(session.VM)
	if maybeObjectBasedInput == nil || maybeObjectBasedInput == sobek.Undefined() {
		return nil, base.NewCustomError(base.InvalidInput.String(), awaitTasksSuggestions)
	}

	taskIDsVal := maybeObjectBasedInput.Get("task_ids")
	if taskIDsVal == nil || taskIDsVal == sobek.Undefined() {
		return nil, base.NewCustomError(base.InvalidInput.String(), awaitTasksSuggestions)
	}

	taskIDsExported := taskIDsVal.Export()
	taskIDsSlice, ok := taskIDsExported.([]interface{})
	if !ok {
		return nil, base.NewCustomError("task_ids must be an array", []string{
			"Provide an array of task ID strings",
			"For example: { task_ids: [task1.task_id, task2.task_id] }",
		})
	}

	taskIDs := make([]string, 0, len(taskIDsSlice))
	for _, idVal := range taskIDsSlice {
		idStr, ok := idVal.(string)
		if !ok {
			return nil, base.NewCustomError("all task_ids must be strings", []string{
				"Ensure each task ID is a string",
			})
		}
		taskIDs = append(taskIDs, idStr)
	}

	input := &subtask.AwaitTasksInput{
		TaskIDs: taskIDs,
		Timeout: 300,
	}

	timeoutVal := maybeObjectBasedInput.Get("timeout")
	if timeoutVal != nil && timeoutVal != sobek.Undefined() && timeoutVal.ExportType() == reflect.TypeOf(0) || timeoutVal.ExportType() == reflect.TypeOf(int64(0)) {
		if timeout, ok := timeoutVal.Export().(int64); ok {
			input.Timeout = int(timeout)
		} else if timeout, ok := timeoutVal.Export().(int); ok {
			input.Timeout = timeout
		}
	}

	return input, nil
}

func awaitTasksHandler(session *Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		input, err := awaitTasksInput(session, call.Arguments)
		if err != nil {
			session.Throw(err)
		}

		result, err := subtask.AwaitTasks(
			session.Context,
			session.Memory,
			session.Bus,
			session.Task.ID,
			input.(*subtask.AwaitTasksInput),
		)
		if err != nil {
			session.Throw(err)
		}

		SetValue(session, "result", result)
		return session.VM.ToValue(result)
	}
}

var awaitTasksSuggestions = []string{
	"Ensure that you provide the correct input arguments as specified in the tool description",
	"- **task_ids** (array of strings, required): Array of task IDs to wait for",
	"- **timeout** (number, optional): Timeout in seconds (default: 300)",
	"For example: await_tasks({ task_ids: [task1.task_id, task2.task_id], timeout: 600 })",
}
