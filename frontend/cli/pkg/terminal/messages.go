package terminal

import (
	"io"
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
)

type messageType int

const (
	MessageTypeUser messageType = iota
	MessageTypeAssistantText
	MessageTypeAssistantTool
	MessageTypeAssistantTyping
	MessageTypeSubmitReport
	MessageTypeError
)

type message interface {
	Type() messageType
	Timestamp() time.Time
}

// TEXT MESSAGES
type userTextMessage struct {
	content   string
	timestamp time.Time
}

func (m *userTextMessage) Type() messageType {
	return MessageTypeUser
}

func (m *userTextMessage) Timestamp() time.Time {
	return m.timestamp
}

func (m *userTextMessage) Render(writer io.Writer) string {
	return m.content
}

var _ message = (*userTextMessage)(nil)

type assistantTextMessage struct {
	content   string
	timestamp time.Time
}

func (m *assistantTextMessage) Type() messageType {
	return MessageTypeAssistantText
}

func (m *assistantTextMessage) Timestamp() time.Time {
	return m.timestamp
}

func (m *assistantTextMessage) Render(writer io.Writer) string {
	return m.content
}

var _ message = (*assistantTextMessage)(nil)

// TOOL CALL MESSAGES
type createFileToolCall struct {
	ID        string
	Input     *v1.ToolCall_CreateFileInput
	timestamp time.Time
}

func (m *createFileToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *createFileToolCall) Timestamp() time.Time {
	return m.timestamp
}

type editFileToolCall struct {
	ID        string
	Input     *v1.ToolCall_EditFileInput
	timestamp time.Time
}

func (m *editFileToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *editFileToolCall) Timestamp() time.Time {
	return m.timestamp
}

type executeCommandToolCall struct {
	ID        string
	Input     *v1.ToolCall_ExecuteCommandInput
	timestamp time.Time
}

func (m *executeCommandToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *executeCommandToolCall) Timestamp() time.Time {
	return m.timestamp
}

type findFileToolCall struct {
	ID        string
	Input     *v1.ToolCall_FindFileInput
	timestamp time.Time
}

func (m *findFileToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *findFileToolCall) Timestamp() time.Time {
	return m.timestamp
}

type grepToolCall struct {
	ID        string
	Input     *v1.ToolCall_GrepInput
	timestamp time.Time
}

func (m *grepToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *grepToolCall) Timestamp() time.Time {
	return m.timestamp
}

type handoffToolCall struct {
	ID        string
	Input     *v1.ToolCall_HandoffInput
	timestamp time.Time
}

func (m *handoffToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *handoffToolCall) Timestamp() time.Time {
	return m.timestamp
}

type askUserToolCall struct {
	ID        string
	Input     *v1.ToolCall_AskUserInput
	timestamp time.Time
}

func (m *askUserToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *askUserToolCall) Timestamp() time.Time {
	return m.timestamp
}

type listFilesToolCall struct {
	ID        string
	Input     *v1.ToolCall_ListFilesInput
	timestamp time.Time
}

func (m *listFilesToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *listFilesToolCall) Timestamp() time.Time {
	return m.timestamp
}

type readFileToolCall struct {
	ID        string
	Input     *v1.ToolCall_ReadFileInput
	timestamp time.Time
}

func (m *readFileToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *readFileToolCall) Timestamp() time.Time {
	return m.timestamp
}

type submitReportToolCall struct {
	ID        string
	Input     *v1.ToolCall_SubmitReportInput
	timestamp time.Time
}

func (m *submitReportToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *submitReportToolCall) Timestamp() time.Time {
	return m.timestamp
}

type codeInterpreterToolCall struct {
	ID        string
	Input     *v1.ToolCall_CodeInterpreterInput
	timestamp time.Time
}

func (m *codeInterpreterToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *codeInterpreterToolCall) Timestamp() time.Time {
	return m.timestamp
}

// SPAWN TASK TOOL CALL
type spawnTaskToolCall struct {
	ID        string
	Input     *v1.ToolCall_SpawnTaskInput
	timestamp time.Time
}

func (m *spawnTaskToolCall) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *spawnTaskToolCall) Timestamp() time.Time {
	return m.timestamp
}

var _ message = (*spawnTaskToolCall)(nil)

// TOOL RESULT MESSAGES
type createFileResult struct {
	ID        string
	Result    *v1.ToolResult_CreateFileResult
	timestamp time.Time
}

func (m *createFileResult) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *createFileResult) Timestamp() time.Time {
	return m.timestamp
}

type editFileResult struct {
	ID        string
	Result    *v1.ToolResult_EditFileResult
	timestamp time.Time
}

func (m *editFileResult) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *editFileResult) Timestamp() time.Time {
	return m.timestamp
}

type executeCommandResult struct {
	ID        string
	Result    *v1.ToolResult_ExecuteCommandResult
	timestamp time.Time
}

func (m *executeCommandResult) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *executeCommandResult) Timestamp() time.Time {
	return m.timestamp
}

type findFileResult struct {
	ID        string
	Result    *v1.ToolResult_FindFileResult
	timestamp time.Time
}

func (m *findFileResult) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *findFileResult) Timestamp() time.Time {
	return m.timestamp
}

type grepResult struct {
	ID        string
	Result    *v1.ToolResult_GrepResult
	timestamp time.Time
}

func (m *grepResult) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *grepResult) Timestamp() time.Time {
	return m.timestamp
}

type listFilesResult struct {
	ID        string
	Result    *v1.ToolResult_ListFilesResult
	timestamp time.Time
}

func (m *listFilesResult) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *listFilesResult) Timestamp() time.Time {
	return m.timestamp
}

type readFileResult struct {
	ID        string
	Result    *v1.ToolResult_ReadFileResult
	timestamp time.Time
}

func (m *readFileResult) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *readFileResult) Timestamp() time.Time {
	return m.timestamp
}

type submitReportResult struct {
	ID        string
	Result    *v1.ToolResult_SubmitReportResult
	timestamp time.Time
}

func (m *submitReportResult) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *submitReportResult) Timestamp() time.Time {
	return m.timestamp
}

type codeInterpreterResult struct {
	ID        string
	Result    *v1.ToolResult_CodeInterpreterResult
	timestamp time.Time
}

func (m *codeInterpreterResult) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *codeInterpreterResult) Timestamp() time.Time {
	return m.timestamp
}

// SPAWN TASK RESULT
type spawnTaskResult struct {
	ID        string
	Result    *v1.ToolResult_SpawnTaskResult
	timestamp time.Time
}

func (m *spawnTaskResult) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *spawnTaskResult) Timestamp() time.Time {
	return m.timestamp
}

var _ message = (*spawnTaskResult)(nil)
