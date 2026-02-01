package conv

import (
	"fmt"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/event"
	tooltypes "github.com/furisto/construct/backend/tool/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ConvertStreamEventToProto converts a domain StreamEvent to a proto Event.
func ConvertStreamEventToProto(e *event.StreamEvent) (*v1.Event, error) {
	if e == nil {
		return nil, fmt.Errorf("event is nil")
	}

	protoEvent := &v1.Event{
		Type:      e.Type,
		Action:    convertActionToProto(e.Action),
		Timestamp: timestamppb.New(e.Timestamp),
	}

	// Convert payload based on event type
	switch e.Type {
	case event.EventTypeTaskCreated, event.EventTypeTaskUpdated, event.EventTypeTaskDeleted:
		payload, err := convertTaskEventPayload(e)
		if err != nil {
			return nil, err
		}
		protoEvent.Payload = payload

	case event.EventTypeMessageCreated, event.EventTypeMessageUpdated, event.EventTypeMessageDeleted:
		payload, err := convertMessageEventPayload(e)
		if err != nil {
			return nil, err
		}
		protoEvent.Payload = payload

	case event.EventTypeMessageChunk:
		payload, err := convertMessageChunkPayload(e)
		if err != nil {
			return nil, err
		}
		protoEvent.Payload = payload

	case event.EventTypeAgentCreated, event.EventTypeAgentUpdated, event.EventTypeAgentDeleted:
		payload, err := convertAgentEventPayload(e)
		if err != nil {
			return nil, err
		}
		protoEvent.Payload = payload

	case event.EventTypeModelCreated, event.EventTypeModelUpdated, event.EventTypeModelDeleted:
		payload, err := convertModelEventPayload(e)
		if err != nil {
			return nil, err
		}
		protoEvent.Payload = payload

	case event.EventTypeModelProviderCreated, event.EventTypeModelProviderUpdated, event.EventTypeModelProviderDeleted:
		payload, err := convertModelProviderEventPayload(e)
		if err != nil {
			return nil, err
		}
		protoEvent.Payload = payload

	case event.EventTypeToolCalled:
		payload, err := convertToolCalledPayload(e)
		if err != nil {
			return nil, err
		}
		protoEvent.Payload = payload

	case event.EventTypeToolResult:
		payload, err := convertToolResultPayload(e)
		if err != nil {
			return nil, err
		}
		protoEvent.Payload = payload

	default:
		return nil, fmt.Errorf("unknown event type: %s", e.Type)
	}

	return protoEvent, nil
}

func convertActionToProto(action string) v1.EventAction {
	switch action {
	case event.ActionCreated:
		return v1.EventAction_EVENT_ACTION_CREATED
	case event.ActionUpdated:
		return v1.EventAction_EVENT_ACTION_UPDATED
	case event.ActionDeleted:
		return v1.EventAction_EVENT_ACTION_DELETED
	default:
		return v1.EventAction_EVENT_ACTION_UNSPECIFIED
	}
}

func convertTaskEventPayload(e *event.StreamEvent) (*v1.Event_Task, error) {
	switch payload := e.Payload.(type) {
	case *event.TaskEventPayload:
		protoTask, err := ConvertTaskToProto(payload.Task)
		if err != nil {
			return nil, err
		}
		taskEvent := &v1.TaskEvent{
			Task: protoTask,
		}
		if payload.PreviousPhase != "" {
			taskEvent.PreviousPhase = &payload.PreviousPhase
		}
		return &v1.Event_Task{Task: taskEvent}, nil

	case *event.DeletedEntityPayload:
		// For deleted events, create a minimal task with just the ID
		protoTask := &v1.Task{
			Metadata: &v1.TaskMetadata{
				Id: payload.ID.String(),
			},
		}
		return &v1.Event_Task{Task: &v1.TaskEvent{Task: protoTask}}, nil

	default:
		return nil, fmt.Errorf("unexpected task event payload type: %T", e.Payload)
	}
}

func convertMessageEventPayload(e *event.StreamEvent) (*v1.Event_Message, error) {
	switch payload := e.Payload.(type) {
	case *event.MessageEventPayload:
		protoMessage, err := ConvertMemoryMessageToProto(payload.Message)
		if err != nil {
			return nil, err
		}
		return &v1.Event_Message{Message: &v1.MessageEvent{Message: protoMessage}}, nil

	case *event.DeletedEntityPayload:
		// For deleted events, create a minimal message with just the ID
		protoMessage := &v1.Message{
			Metadata: &v1.MessageMetadata{
				Id: payload.ID.String(),
			},
		}
		if payload.TaskID != nil {
			protoMessage.Metadata.TaskId = payload.TaskID.String()
		}
		return &v1.Event_Message{Message: &v1.MessageEvent{Message: protoMessage}}, nil

	default:
		return nil, fmt.Errorf("unexpected message event payload type: %T", e.Payload)
	}
}

func convertMessageChunkPayload(e *event.StreamEvent) (*v1.Event_MessageChunk, error) {
	payload, ok := e.Payload.(*event.MessageChunkPayload)
	if !ok {
		return nil, fmt.Errorf("unexpected message chunk payload type: %T", e.Payload)
	}

	return &v1.Event_MessageChunk{
		MessageChunk: &v1.MessageChunkEvent{
			TaskId:     payload.TaskID.String(),
			MessageId:  payload.MessageID.String(),
			Chunk:      payload.Chunk,
			ChunkIndex: int32(payload.ChunkIndex),
		},
	}, nil
}

func convertAgentEventPayload(e *event.StreamEvent) (*v1.Event_Agent, error) {
	switch payload := e.Payload.(type) {
	case *event.AgentEventPayload:
		protoAgent, err := ConvertAgentToProto(payload.Agent)
		if err != nil {
			return nil, err
		}
		return &v1.Event_Agent{Agent: &v1.AgentEvent{Agent: protoAgent}}, nil

	case *event.DeletedEntityPayload:
		// For deleted events, create a minimal agent with just the ID
		protoAgent := &v1.Agent{
			Metadata: &v1.AgentMetadata{
				Id: payload.ID.String(),
			},
		}
		return &v1.Event_Agent{Agent: &v1.AgentEvent{Agent: protoAgent}}, nil

	default:
		return nil, fmt.Errorf("unexpected agent event payload type: %T", e.Payload)
	}
}

func convertModelEventPayload(e *event.StreamEvent) (*v1.Event_Model, error) {
	switch payload := e.Payload.(type) {
	case *event.ModelEventPayload:
		protoModel, err := MemoryModelToProto(payload.Model)
		if err != nil {
			return nil, err
		}
		return &v1.Event_Model{Model: &v1.ModelEvent{Model: protoModel}}, nil

	case *event.DeletedEntityPayload:
		// For deleted events, create a minimal model with just the ID
		protoModel := &v1.Model{
			Metadata: &v1.ModelMetadata{
				Id: payload.ID.String(),
			},
		}
		return &v1.Event_Model{Model: &v1.ModelEvent{Model: protoModel}}, nil

	default:
		return nil, fmt.Errorf("unexpected model event payload type: %T", e.Payload)
	}
}

func convertModelProviderEventPayload(e *event.StreamEvent) (*v1.Event_ModelProvider, error) {
	switch payload := e.Payload.(type) {
	case *event.ModelProviderEventPayload:
		protoProvider, err := ConvertModelProviderIntoProto(payload.ModelProvider)
		if err != nil {
			return nil, err
		}
		return &v1.Event_ModelProvider{ModelProvider: &v1.ModelProviderEvent{ModelProvider: protoProvider}}, nil

	case *event.DeletedEntityPayload:
		// For deleted events, create a minimal provider with just the ID
		protoProvider := &v1.ModelProvider{
			Metadata: &v1.ModelProviderMetadata{
				Id: payload.ID.String(),
			},
		}
		return &v1.Event_ModelProvider{ModelProvider: &v1.ModelProviderEvent{ModelProvider: protoProvider}}, nil

	default:
		return nil, fmt.Errorf("unexpected model provider event payload type: %T", e.Payload)
	}
}

func convertToolCalledPayload(e *event.StreamEvent) (*v1.Event_ToolCalled, error) {
	payload, ok := e.Payload.(*tooltypes.ToolCallEvent)
	if !ok {
		return nil, fmt.Errorf("unexpected tool called payload type: %T", e.Payload)
	}

	taskID := ""
	if e.TaskID != nil {
		taskID = e.TaskID.String()
	}

	toolCall := convertToolInputToProto(payload.ID, payload.Tool, &payload.Input)

	return &v1.Event_ToolCalled{
		ToolCalled: &v1.ToolCalledEvent{
			TaskId:   taskID,
			ToolCall: toolCall,
		},
	}, nil
}

func convertToolResultPayload(e *event.StreamEvent) (*v1.Event_ToolResult, error) {
	payload, ok := e.Payload.(*tooltypes.ToolResultEvent)
	if !ok {
		return nil, fmt.Errorf("unexpected tool result payload type: %T", e.Payload)
	}

	taskID := ""
	if e.TaskID != nil {
		taskID = e.TaskID.String()
	}

	toolResult := convertToolOutputToProto(payload.ID, payload.Tool, &payload.Output)

	return &v1.Event_ToolResult{
		ToolResult: &v1.ToolResultEvent{
			TaskId:     taskID,
			ToolResult: toolResult,
		},
	}, nil
}

// convertToolInputToProto converts tool input to proto ToolCall.
func convertToolInputToProto(id, toolName string, input *tooltypes.ToolInput) *v1.ToolCall {
	tc := &v1.ToolCall{
		Id:       id,
		ToolName: toolName,
	}

	if input == nil {
		return tc
	}

	switch {
	case input.CreateFile != nil:
		tc.Input = &v1.ToolCall_CreateFile{
			CreateFile: &v1.ToolCall_CreateFileInput{
				Path:    input.CreateFile.Path,
				Content: input.CreateFile.Content,
			},
		}
	case input.EditFile != nil:
		diffs := make([]*v1.ToolCall_EditFileInput_DiffPair, 0, len(input.EditFile.Diffs))
		for _, d := range input.EditFile.Diffs {
			diffs = append(diffs, &v1.ToolCall_EditFileInput_DiffPair{
				Old: d.Old,
				New: d.New,
			})
		}
		tc.Input = &v1.ToolCall_EditFile{
			EditFile: &v1.ToolCall_EditFileInput{
				Path:  input.EditFile.Path,
				Diffs: diffs,
			},
		}
	case input.ExecuteCommand != nil:
		tc.Input = &v1.ToolCall_ExecuteCommand{
			ExecuteCommand: &v1.ToolCall_ExecuteCommandInput{
				Command: input.ExecuteCommand.Command,
			},
		}
	case input.FindFile != nil:
		tc.Input = &v1.ToolCall_FindFile{
			FindFile: &v1.ToolCall_FindFileInput{
				Pattern:        input.FindFile.Pattern,
				Path:           input.FindFile.Path,
				ExcludePattern: input.FindFile.ExcludePattern,
				MaxResults:     int32(input.FindFile.MaxResults),
			},
		}
	case input.Grep != nil:
		tc.Input = &v1.ToolCall_Grep{
			Grep: &v1.ToolCall_GrepInput{
				Query:          input.Grep.Query,
				Path:           input.Grep.Path,
				IncludePattern: input.Grep.IncludePattern,
				ExcludePattern: input.Grep.ExcludePattern,
				CaseSensitive:  input.Grep.CaseSensitive,
				MaxResults:     int32(input.Grep.MaxResults),
			},
		}
	case input.ListFiles != nil:
		tc.Input = &v1.ToolCall_ListFiles{
			ListFiles: &v1.ToolCall_ListFilesInput{
				Path:      input.ListFiles.Path,
				Recursive: input.ListFiles.Recursive,
			},
		}
	case input.ReadFile != nil:
		readFileInput := &v1.ToolCall_ReadFileInput{
			Path: input.ReadFile.Path,
		}
		if input.ReadFile.StartLine != nil {
			readFileInput.StartLine = int32(*input.ReadFile.StartLine)
		}
		if input.ReadFile.EndLine != nil {
			readFileInput.EndLine = int32(*input.ReadFile.EndLine)
		}
		tc.Input = &v1.ToolCall_ReadFile{
			ReadFile: readFileInput,
		}
	case input.SubmitReport != nil:
		tc.Input = &v1.ToolCall_SubmitReport{
			SubmitReport: &v1.ToolCall_SubmitReportInput{
				Summary:      input.SubmitReport.Summary,
				Completed:    input.SubmitReport.Completed,
				Deliverables: input.SubmitReport.Deliverables,
				NextSteps:    input.SubmitReport.NextSteps,
			},
		}
	case input.AskUser != nil:
		tc.Input = &v1.ToolCall_AskUser{
			AskUser: &v1.ToolCall_AskUserInput{
				Question: input.AskUser.Question,
				Options:  input.AskUser.Options,
			},
		}
	case input.Handoff != nil:
		tc.Input = &v1.ToolCall_Handoff{
			Handoff: &v1.ToolCall_HandoffInput{
				RequestedAgent:  input.Handoff.RequestedAgent,
				HandoverMessage: input.Handoff.HandoverMessage,
			},
		}
	case input.Fetch != nil:
		tc.Input = &v1.ToolCall_Fetch{
			Fetch: &v1.ToolCall_FetchInput{
				Url:     input.Fetch.URL,
				Headers: input.Fetch.Headers,
				Timeout: int32(input.Fetch.Timeout),
			},
		}
	}

	return tc
}

// convertToolOutputToProto converts tool output to proto ToolResult.
func convertToolOutputToProto(id, toolName string, output *tooltypes.ToolOutput) *v1.ToolResult {
	tr := &v1.ToolResult{
		Id:       id,
		ToolName: toolName,
	}

	if output == nil {
		return tr
	}

	switch {
	case output.CreateFile != nil:
		tr.Result = &v1.ToolResult_CreateFile{
			CreateFile: &v1.ToolResult_CreateFileResult{
				Overwritten: output.CreateFile.Overwritten,
			},
		}
	case output.EditFile != nil:
		patchInfo := &v1.ToolResult_EditFileResult_PatchInfo{
			Patch:        output.EditFile.PatchInfo.Patch,
			LinesAdded:   int32(output.EditFile.PatchInfo.LinesAdded),
			LinesRemoved: int32(output.EditFile.PatchInfo.LinesRemoved),
		}
		tr.Result = &v1.ToolResult_EditFile{
			EditFile: &v1.ToolResult_EditFileResult{
				Path:      output.EditFile.Path,
				PatchInfo: patchInfo,
			},
		}
	case output.ExecuteCommand != nil:
		tr.Result = &v1.ToolResult_ExecuteCommand{
			ExecuteCommand: &v1.ToolResult_ExecuteCommandResult{
				Stdout:   output.ExecuteCommand.Stdout,
				Stderr:   output.ExecuteCommand.Stderr,
				ExitCode: int32(output.ExecuteCommand.ExitCode),
				Command:  output.ExecuteCommand.Command,
			},
		}
	case output.FindFile != nil:
		tr.Result = &v1.ToolResult_FindFile{
			FindFile: &v1.ToolResult_FindFileResult{
				Files: output.FindFile.Files,
			},
		}
	case output.Grep != nil:
		matches := make([]*v1.ToolResult_GrepResult_GrepMatch, 0, len(output.Grep.Matches))
		for _, m := range output.Grep.Matches {
			matches = append(matches, &v1.ToolResult_GrepResult_GrepMatch{
				FilePath: m.FilePath,
				Value:    m.Value,
			})
		}
		tr.Result = &v1.ToolResult_Grep{
			Grep: &v1.ToolResult_GrepResult{
				Matches:        matches,
				TotalMatches:   int32(output.Grep.TotalMatches),
				TruncatedCount: int32(output.Grep.TruncatedMatches),
				SearchedFiles:  int32(output.Grep.SearchedFiles),
			},
		}
	case output.ListFiles != nil:
		entries := make([]*v1.ToolResult_ListFilesResult_DirectoryEntry, 0, len(output.ListFiles.Entries))
		for _, e := range output.ListFiles.Entries {
			entries = append(entries, &v1.ToolResult_ListFilesResult_DirectoryEntry{
				Name: e.Name,
				Type: e.Type,
				Size: e.Size,
			})
		}
		tr.Result = &v1.ToolResult_ListFiles{
			ListFiles: &v1.ToolResult_ListFilesResult{
				Path:    output.ListFiles.Path,
				Entries: entries,
			},
		}
	case output.ReadFile != nil:
		tr.Result = &v1.ToolResult_ReadFile{
			ReadFile: &v1.ToolResult_ReadFileResult{
				Content: output.ReadFile.Content,
			},
		}
	case output.SubmitReport != nil:
		tr.Result = &v1.ToolResult_SubmitReport{
			SubmitReport: &v1.ToolResult_SubmitReportResult{
				Summary:      output.SubmitReport.Summary,
				Completed:    output.SubmitReport.Completed,
				Deliverables: output.SubmitReport.Deliverables,
				NextSteps:    output.SubmitReport.NextSteps,
			},
		}
	case output.AskUser != nil:
		// AskUser result is not exposed via proto events - skip
	case output.Fetch != nil:
		tr.Result = &v1.ToolResult_Fetch{
			Fetch: &v1.ToolResult_FetchResult{
				Url:         output.Fetch.URL,
				Title:       output.Fetch.Title,
				Content:     output.Fetch.Content,
				ContentType: output.Fetch.ContentType,
				ByteSize:    int64(output.Fetch.ByteSize),
			},
		}
	}

	return tr
}

