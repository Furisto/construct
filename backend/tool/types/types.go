package types

import (
	"fmt"

	"github.com/furisto/construct/backend/tool/communication"
	"github.com/furisto/construct/backend/tool/filesystem"
	"github.com/furisto/construct/backend/tool/system"
	"github.com/furisto/construct/backend/tool/web"
)

// InterpreterInput represents input for the code interpreter tool.
type InterpreterInput struct {
	Script string `json:"script"`
}

// InterpreterOutput represents output from the code interpreter tool.
type InterpreterOutput struct {
	ConsoleOutput string           `json:"console_output"`
	FunctionCalls []FunctionCall   `json:"function_calls"`
	ToolStats     map[string]int64 `json:"tool_stats,omitempty"`
}

// FunctionCall represents a function call made within the interpreter.
type FunctionCall struct {
	ToolName string     `json:"tool_name"`
	Input    ToolInput  `json:"input"`
	Output   ToolOutput `json:"output"`
	Index    int        `json:"index"`
}

// ToolInput contains the typed input for a tool call.
// Only one field will be set at a time based on the tool being called.
type ToolInput struct {
	CreateFile     *filesystem.CreateFileInput      `json:"create_file,omitempty"`
	EditFile       *filesystem.EditFileInput        `json:"edit_file,omitempty"`
	ExecuteCommand *system.ExecuteCommandInput      `json:"execute_command,omitempty"`
	FindFile       *filesystem.FindFileInput        `json:"find_file,omitempty"`
	Grep           *filesystem.GrepInput            `json:"grep,omitempty"`
	ListFiles      *filesystem.ListFilesInput       `json:"list_files,omitempty"`
	ReadFile       *filesystem.ReadFileInput        `json:"read_file,omitempty"`
	SubmitReport   *communication.SubmitReportInput `json:"submit_report,omitempty"`
	AskUser        *communication.AskUserInput      `json:"ask_user,omitempty"`
	Handoff        *communication.HandoffInput      `json:"handoff,omitempty"`
	Fetch          *web.FetchInput                  `json:"fetch,omitempty"`
	Interpreter    *InterpreterInput                `json:"interpreter,omitempty"`
}

// ToolInputFrom converts a raw tool input to the typed ToolInput struct.
// Returns an error if the input type is not recognized.
func ToolInputFrom(input any) (ToolInput, error) {
	var result ToolInput
	switch v := input.(type) {
	case *filesystem.CreateFileInput:
		result.CreateFile = v
	case *filesystem.EditFileInput:
		result.EditFile = v
	case *system.ExecuteCommandInput:
		result.ExecuteCommand = v
	case *filesystem.FindFileInput:
		result.FindFile = v
	case *filesystem.GrepInput:
		result.Grep = v
	case *filesystem.ListFilesInput:
		result.ListFiles = v
	case *filesystem.ReadFileInput:
		result.ReadFile = v
	case *communication.SubmitReportInput:
		result.SubmitReport = v
	case *communication.AskUserInput:
		result.AskUser = v
	case *communication.HandoffInput:
		result.Handoff = v
	case *web.FetchInput:
		result.Fetch = v
	case *InterpreterInput:
		result.Interpreter = v
	default:
		return result, fmt.Errorf("unknown tool input type: %T", input)
	}
	return result, nil
}

// ToolOutput contains the typed output for a tool result.
// Only one field will be set at a time based on the tool that was executed.
type ToolOutput struct {
	CreateFile     *filesystem.CreateFileResult      `json:"create_file,omitempty"`
	EditFile       *filesystem.EditFileResult        `json:"edit_file,omitempty"`
	ExecuteCommand *system.ExecuteCommandResult      `json:"execute_command,omitempty"`
	FindFile       *filesystem.FindFileResult        `json:"find_file,omitempty"`
	Grep           *filesystem.GrepResult            `json:"grep,omitempty"`
	ListFiles      *filesystem.ListFilesResult       `json:"list_files,omitempty"`
	ReadFile       *filesystem.ReadFileResult        `json:"read_file,omitempty"`
	SubmitReport   *communication.SubmitReportResult `json:"submit_report,omitempty"`
	AskUser        *communication.AskUserResult      `json:"ask_user,omitempty"`
	Fetch          *web.FetchResult                  `json:"fetch,omitempty"`
	Interpreter    *InterpreterOutput                `json:"interpreter,omitempty"`
}

// ToolOutputFrom converts a raw tool output to the typed ToolOutput struct.
// Returns an error if the output type is not recognized.
func ToolOutputFrom(output any) (ToolOutput, error) {
	var result ToolOutput
	switch v := output.(type) {
	case *filesystem.CreateFileResult:
		result.CreateFile = v
	case *filesystem.EditFileResult:
		result.EditFile = v
	case *system.ExecuteCommandResult:
		result.ExecuteCommand = v
	case *filesystem.FindFileResult:
		result.FindFile = v
	case *filesystem.GrepResult:
		result.Grep = v
	case *filesystem.ListFilesResult:
		result.ListFiles = v
	case *filesystem.ReadFileResult:
		result.ReadFile = v
	case *communication.SubmitReportResult:
		result.SubmitReport = v
	case *communication.AskUserResult:
		result.AskUser = v
	case *web.FetchResult:
		result.Fetch = v
	case *InterpreterOutput:
		result.Interpreter = v
	default:
		return result, fmt.Errorf("unknown tool output type: %T", output)
	}
	return result, nil
}

// ToolCallEvent represents a tool call for event streaming.
type ToolCallEvent struct {
	ID    string    `json:"id"`
	Tool  string    `json:"tool"`
	Input ToolInput `json:"input"`
}

// ToolResultEvent represents a tool result for event streaming.
type ToolResultEvent struct {
	ID     string     `json:"id"`
	Tool   string     `json:"tool"`
	Output ToolOutput `json:"output"`
}
