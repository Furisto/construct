package codeact

import (
	"encoding/json"
	"fmt"

	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/subtask"
	"github.com/grafana/sobek"
)

const sendMessageDescription = `
## Description
Send a message to another task. Currently only supports sending to "parent" task.

This tool allows subtasks to communicate results back to their parent. Messages are stored as data and collected by the parent when it calls await_tasks().

## Parameters
- **to** (string, required): Recipient - "parent" (only supported value currently)
- **content** (object, required): Message content - can be any JSON-serializable object

## Expected Output
Returns an object indicating delivery status:
%[1]s
{
  "delivered": true,
  "error": ""  // Only present if delivery failed
}
%[1]s

## IMPORTANT USAGE NOTES
- **Only for subtasks**: This tool only works within a subtask (spawned via spawn_task)
- **Multiple messages**: You can send multiple messages during subtask execution
- **Parent receives all**: All messages are collected by parent when it calls await_tasks()
- **No response**: This is one-way communication; parent doesn't respond

## When to use
- **Return results**: Send computed results back to parent
- **Progress updates**: Report status during long-running operations
- **Incremental data**: Send data as it's discovered rather than all at once
- **Structured output**: Return different message types (summary, error, data)

## Usage Examples

### Send structured results
%[1]s
const issues = [];
const files = list_files({ path: "src", recursive: true });

for (const file of files.files) {
    const content = read_file({ path: file.path });
    // analyze...
    if (foundIssue) {
        issues.push({ file: file.path, line: 42, description: "..." });
    }
}

send_message({
    to: "parent",
    content: {
        issues: issues,
        files_analyzed: files.files.length
    }
});
%[1]s

### Send multiple progress updates
%[1]s
const files = list_files({ path: "src", recursive: true }).files;

for (const file of files) {
    const result = analyzeFile(file);
    
    if (result.issues.length > 0) {
        send_message({
            to: "parent",
            content: {
                file: file.path,
                issues: result.issues
            }
        });
    }
}

// Send final summary
send_message({
    to: "parent",
    content: {
        type: "summary",
        total_files: files.length,
        total_issues: totalIssues
    }
});
%[1]s
`

func NewSendMessageTool() Tool {
	return NewOnDemandTool(
		"send_message",
		fmt.Sprintf(sendMessageDescription, "```", "`"),
		sendMessageInput,
		sendMessageHandler,
	)
}

func sendMessageInput(session *Session, args []sobek.Value) (any, error) {
	if len(args) < 1 {
		return nil, base.NewCustomError(base.InvalidInput.String(), sendMessageSuggestions)
	}

	maybeObjectBasedInput := args[0].ToObject(session.VM)
	if maybeObjectBasedInput == nil || maybeObjectBasedInput == sobek.Undefined() {
		return nil, base.NewCustomError(base.InvalidInput.String(), sendMessageSuggestions)
	}

	toVal := maybeObjectBasedInput.Get("to")
	if toVal == nil || toVal == sobek.Undefined() {
		return nil, base.NewCustomError(base.InvalidInput.String(), sendMessageSuggestions)
	}

	contentVal := maybeObjectBasedInput.Get("content")
	if contentVal == nil || contentVal == sobek.Undefined() {
		return nil, base.NewCustomError(base.InvalidInput.String(), sendMessageSuggestions)
	}

	contentJSON, err := json.Marshal(contentVal.Export())
	if err != nil {
		return nil, base.NewCustomError("failed to serialize content", []string{
			"Ensure content is a valid JSON-serializable object",
		})
	}

	return &subtask.SendMessageInput{
		To: toVal.String(),
		Content: &types.MessageContent{
			Blocks: []types.MessageBlock{
				{
					Kind:    types.MessageBlockKindText,
					Payload: string(contentJSON),
				},
			},
		},
	}, nil
}

func sendMessageHandler(session *Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		input, err := sendMessageInput(session, call.Arguments)
		if err != nil {
			session.Throw(err)
		}

		result, err := subtask.SendMessage(
			session.Context,
			session.Memory,
			session.Task.ID,
			session.Task.ParentTaskID,
			input.(*subtask.SendMessageInput),
		)
		if err != nil {
			session.Throw(err)
		}

		SetValue(session, "result", result)
		return session.VM.ToValue(result)
	}
}

var sendMessageSuggestions = []string{
	"Ensure that you provide the correct input arguments as specified in the tool description",
	"- **to** (string, required): Recipient - 'parent' (only supported value currently)",
	"- **content** (object, required): Message content - can be any JSON-serializable object",
	"For example: send_message({ to: 'parent', content: { result: 'success', data: [...] } })",
}
