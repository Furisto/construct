package types

import (
	"errors"

	"github.com/furisto/construct/backend/tool/communication"
	"github.com/furisto/construct/backend/tool/filesystem"
	"github.com/furisto/construct/backend/tool/subtask"
	"github.com/furisto/construct/backend/tool/system"
	"github.com/google/uuid"
)

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
	SpawnTask      *subtask.SpawnTaskInput          `json:"spawn_task,omitempty"`
	SendMessage    *subtask.SendMessageInput        `json:"send_message,omitempty"`
	AwaitTasks     *subtask.AwaitTasksInput         `json:"await_tasks,omitempty"`
}

func ToolInputFromAny(any any) (ToolInput, error) {
	switch input := any.(type) {
	case *filesystem.CreateFileInput:
		return ToolInput{CreateFile: input}, nil
	case *filesystem.EditFileInput:
		return ToolInput{EditFile: input}, nil
	case *system.ExecuteCommandInput:
		return ToolInput{ExecuteCommand: input}, nil
	case *filesystem.FindFileInput:
		return ToolInput{FindFile: input}, nil
	case *filesystem.GrepInput:
		return ToolInput{Grep: input}, nil
	case *filesystem.ListFilesInput:
		return ToolInput{ListFiles: input}, nil
	case *filesystem.ReadFileInput:
		return ToolInput{ReadFile: input}, nil
	case *communication.SubmitReportInput:
		return ToolInput{SubmitReport: input}, nil
	case *communication.AskUserInput:
		return ToolInput{AskUser: input}, nil
	case *communication.HandoffInput:
		return ToolInput{Handoff: input}, nil
	case *subtask.SpawnTaskInput:
		return ToolInput{SpawnTask: input}, nil
	case *subtask.SendMessageInput:
		return ToolInput{SendMessage: input}, nil
	case *subtask.AwaitTasksInput:
		return ToolInput{AwaitTasks: input}, nil
	}
	return ToolInput{}, errors.New("invalid input")
}

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
	SpawnTask      *subtask.SpawnTaskResult          `json:"spawn_task,omitempty"`
	SendMessage    *subtask.SendMessageResult        `json:"send_message,omitempty"`
	AwaitTasks     *subtask.AwaitTasksResult         `json:"await_tasks,omitempty"`
}

func ToolOutputFromAny(any any) (ToolOutput, error) {
	switch output := any.(type) {
	case *filesystem.CreateFileResult:
		return ToolOutput{CreateFile: output}, nil
	case *filesystem.EditFileResult:
		return ToolOutput{EditFile: output}, nil
	case *system.ExecuteCommandResult:
		return ToolOutput{ExecuteCommand: output}, nil
	case *filesystem.FindFileResult:
		return ToolOutput{FindFile: output}, nil
	case *filesystem.GrepResult:
		return ToolOutput{Grep: output}, nil
	case *filesystem.ListFilesResult:
		return ToolOutput{ListFiles: output}, nil
	case *filesystem.ReadFileResult:
		return ToolOutput{ReadFile: output}, nil
	case *communication.SubmitReportResult:
		return ToolOutput{SubmitReport: output}, nil
	case *communication.AskUserResult:
		return ToolOutput{AskUser: output}, nil
	case *subtask.SpawnTaskResult:
		return ToolOutput{SpawnTask: output}, nil
	case *subtask.SendMessageResult:
		return ToolOutput{SendMessage: output}, nil
	case *subtask.AwaitTasksResult:
		return ToolOutput{AwaitTasks: output}, nil
	}
	return ToolOutput{}, errors.New("invalid output")
}

type ToolCallEvent struct {
	TaskID   uuid.UUID
	ToolName string
	Input    ToolInput
}

func (e ToolCallEvent) Event() {}

type ToolResultEvent struct {
	TaskID   uuid.UUID
	ToolName string
	Input    ToolInput
	Output   ToolOutput
}

func (e ToolResultEvent) Event() {}