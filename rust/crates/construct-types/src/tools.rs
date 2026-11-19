//! Go tools domain values; declaration order is JSON field order.
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
use uuid::Uuid;

/// Go `backend/tool/filesystem/create.go` `CreateFileInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct CreateFileInput {
    /// Go `Path` field.
    #[serde(rename = "Path")]
    pub path: String,
    /// Go `Content` field.
    #[serde(rename = "Content")]
    pub content: String,
}

/// Go `backend/tool/filesystem/create.go` `CreateFileResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct CreateFileResult {
    /// Go `Overwritten` field.
    #[serde(rename = "overwritten")]
    pub overwritten: bool,
}

/// Go `backend/tool/filesystem/edit.go` `EditFileInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct EditFileInput {
    /// Go `Path` field.
    #[serde(rename = "path")]
    pub path: String,
    /// Go `Diffs` field (`None` is a nil slice).
    #[serde(rename = "diffs")]
    pub diffs: Option<Vec<DiffPair>>,
}

/// Go `backend/tool/filesystem/edit.go` `DiffPair`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct DiffPair {
    /// Go `Old` field.
    #[serde(rename = "old")]
    pub old: String,
    /// Go `New` field.
    #[serde(rename = "new")]
    pub new: String,
}

/// Go `backend/tool/filesystem/edit.go` `DiffValidationError`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct DiffValidationError {
    /// Go `DiffIndex` field.
    #[serde(rename = "diff_index")]
    pub diff_index: i64,
    /// Go `ErrorType` field.
    #[serde(rename = "error_type")]
    pub error_type: String,
    /// Go `ErrorMessage` field.
    #[serde(rename = "error_message")]
    pub error_message: String,
    /// Go `SuggestedFix` field.
    #[serde(rename = "suggested_fix", skip_serializing_if = "String::is_empty")]
    pub suggested_fix: String,
}

/// Go `backend/tool/filesystem/edit.go` `ConflictWarning`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ConflictWarning {
    /// Go `Edit1Index` field.
    #[serde(rename = "edit1_index")]
    pub edit1_index: i64,
    /// Go `Edit2Index` field.
    #[serde(rename = "edit2_index")]
    pub edit2_index: i64,
    /// Go `ConflictType` field.
    #[serde(rename = "conflict_type")]
    pub conflict_type: String,
    /// Go `Message` field.
    #[serde(rename = "message")]
    pub message: String,
}

/// Go `backend/tool/filesystem/edit.go` `PatchInfo`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct PatchInfo {
    /// Go `Patch` field.
    #[serde(rename = "patch")]
    pub patch: String,
    /// Go `LinesAdded` field.
    #[serde(rename = "lines_added")]
    pub lines_added: i64,
    /// Go `LinesRemoved` field.
    #[serde(rename = "lines_removed")]
    pub lines_removed: i64,
}

/// Go `backend/tool/filesystem/edit.go` `EditFileResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct EditFileResult {
    /// Go `Success` field.
    #[serde(rename = "success")]
    pub success: bool,
    /// Go `Path` field.
    #[serde(rename = "path")]
    pub path: String,
    /// Go `ReplacementsMade` field.
    #[serde(rename = "replacements_made")]
    pub replacements_made: i64,
    /// Go `ExpectedReplacements` field.
    #[serde(rename = "expected_replacements")]
    pub expected_replacements: i64,
    /// Go `FailureReason` field.
    #[serde(rename = "failure_reason", skip_serializing_if = "String::is_empty")]
    pub failure_reason: String,
    /// Go `ValidationErrors` field (`None` is a nil slice).
    #[serde(
        rename = "validation_errors",
        skip_serializing_if = "crate::json::is_empty"
    )]
    pub validation_errors: Option<Vec<DiffValidationError>>,
    /// Go `ConflictWarnings` field (`None` is a nil slice).
    #[serde(
        rename = "conflict_warnings",
        skip_serializing_if = "crate::json::is_empty"
    )]
    pub conflict_warnings: Option<Vec<ConflictWarning>>,
    /// Go `PatchInfo` field.
    #[serde(rename = "patch_info")]
    pub patch_info: PatchInfo,
}

/// Go `backend/tool/filesystem/find.go` `FindFileInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct FindFileInput {
    /// Go `Pattern` field.
    #[serde(rename = "pattern")]
    pub pattern: String,
    /// Go `Path` field.
    #[serde(rename = "path")]
    pub path: String,
    /// Go `ExcludePattern` field.
    #[serde(rename = "exclude_pattern")]
    pub exclude_pattern: String,
    /// Go `MaxResults` field.
    #[serde(rename = "max_results")]
    pub max_results: i64,
}

/// Go `backend/tool/filesystem/find.go` `FindFileResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct FindFileResult {
    /// Go `Files` field (`None` is a nil slice).
    #[serde(rename = "files")]
    pub files: Option<Vec<String>>,
    /// Go `TotalFiles` field.
    #[serde(rename = "total_files")]
    pub total_files: i64,
    /// Go `TruncatedCount` field.
    #[serde(rename = "truncated_count")]
    pub truncated_count: i64,
}

/// Go `backend/tool/filesystem/grep.go` `GrepInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct GrepInput {
    /// Go `Query` field.
    #[serde(rename = "query")]
    pub query: String,
    /// Go `Path` field.
    #[serde(rename = "path")]
    pub path: String,
    /// Go `IncludePattern` field.
    #[serde(rename = "include_pattern")]
    pub include_pattern: String,
    /// Go `ExcludePattern` field.
    #[serde(rename = "exclude_pattern")]
    pub exclude_pattern: String,
    /// Go `CaseSensitive` field.
    #[serde(rename = "case_sensitive")]
    pub case_sensitive: bool,
    /// Go `MaxResults` field.
    #[serde(rename = "max_results")]
    pub max_results: i64,
    /// Go `Context` field.
    #[serde(rename = "context")]
    pub context: i64,
}

/// Go `backend/tool/filesystem/grep.go` `GrepMatch`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct GrepMatch {
    /// Go `FilePath` field.
    #[serde(rename = "file_path")]
    pub file_path: String,
    /// Go `Value` field.
    #[serde(rename = "value")]
    pub value: String,
}

/// Go `backend/tool/filesystem/grep.go` `GrepResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct GrepResult {
    /// Go `Matches` field (`None` is a nil slice).
    #[serde(rename = "matches")]
    pub matches: Option<Vec<GrepMatch>>,
    /// Go `TotalMatches` field.
    #[serde(rename = "total_matches")]
    pub total_matches: i64,
    /// Go `TruncatedMatches` field.
    #[serde(rename = "truncated_matches")]
    pub truncated_matches: i64,
    /// Go `SearchedFiles` field.
    #[serde(rename = "searched_files")]
    pub searched_files: i64,
}

/// Go `backend/tool/filesystem/list.go` `ListFilesInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ListFilesInput {
    /// Go `Path` field.
    #[serde(rename = "Path")]
    pub path: String,
    /// Go `Recursive` field.
    #[serde(rename = "Recursive")]
    pub recursive: bool,
}

/// Go `backend/tool/filesystem/list.go` `ListFilesResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ListFilesResult {
    /// Go `Path` field.
    #[serde(rename = "path")]
    pub path: String,
    /// Go `Entries` field (`None` is a nil slice).
    #[serde(rename = "entries")]
    pub entries: Option<Vec<DirectoryEntry>>,
}

/// Go `backend/tool/filesystem/list.go` `DirectoryEntry`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct DirectoryEntry {
    /// Go `Name` field.
    #[serde(rename = "n")]
    pub name: String,
    /// Go `Type` field.
    #[serde(rename = "t")]
    pub r#type: String,
    /// Go `Size` field.
    #[serde(rename = "s")]
    pub size: i64,
}

/// Go `backend/tool/filesystem/read.go` `ReadFileInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ReadFileInput {
    /// Go `Path` field.
    #[serde(rename = "path")]
    pub path: String,
    /// Go `StartLine` field.
    #[serde(rename = "start_line", skip_serializing_if = "Option::is_none")]
    pub start_line: Option<Box<i64>>,
    /// Go `EndLine` field.
    #[serde(rename = "end_line", skip_serializing_if = "Option::is_none")]
    pub end_line: Option<Box<i64>>,
}

/// Go `backend/tool/filesystem/read.go` `ReadFileResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ReadFileResult {
    /// Go `Path` field.
    #[serde(rename = "path")]
    pub path: String,
    /// Go `Content` field.
    #[serde(rename = "content")]
    pub content: String,
}

/// Go `backend/tool/system/command.go` `ExecuteCommandInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ExecuteCommandInput {
    /// Go `Command` field.
    #[serde(rename = "Command")]
    pub command: String,
    /// Go `WorkingDirectory` field.
    #[serde(rename = "WorkingDirectory")]
    pub working_directory: String,
}

/// Go `backend/tool/system/command.go` `ExecuteCommandResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ExecuteCommandResult {
    /// Go `Stdout` field.
    #[serde(rename = "stdout")]
    pub stdout: String,
    /// Go `Stderr` field.
    #[serde(rename = "stderr")]
    pub stderr: String,
    /// Go `ExitCode` field.
    #[serde(rename = "exitCode")]
    pub exit_code: i64,
    /// Go `Command` field.
    #[serde(rename = "command")]
    pub command: String,
}

/// Go `backend/tool/communication/ask.go` `AskUserInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct AskUserInput {
    /// Go `Question` field.
    #[serde(rename = "question")]
    pub question: String,
    /// Go `Options` field (`None` is a nil slice).
    #[serde(rename = "options", skip_serializing_if = "crate::json::is_empty")]
    pub options: Option<Vec<String>>,
}

/// Go `backend/tool/communication/ask.go` `AskUserResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct AskUserResult {
    /// Go `UserResponse` field.
    #[serde(rename = "user_response")]
    pub user_response: String,
    /// Go `SelectedOption` field.
    #[serde(rename = "selected_option", skip_serializing_if = "String::is_empty")]
    pub selected_option: String,
}

/// Go `backend/tool/communication/report.go` `SubmitReportInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct SubmitReportInput {
    /// Go `Summary` field.
    #[serde(rename = "Summary")]
    pub summary: String,
    /// Go `Completed` field.
    #[serde(rename = "Completed")]
    pub completed: bool,
    /// Go `Deliverables` field (`None` is a nil slice).
    #[serde(rename = "Deliverables")]
    pub deliverables: Option<Vec<String>>,
    /// Go `NextSteps` field.
    #[serde(rename = "NextSteps")]
    pub next_steps: String,
}

/// Go `backend/tool/communication/report.go` `SubmitReportResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct SubmitReportResult {
    /// Go `Summary` field.
    #[serde(rename = "summary")]
    pub summary: String,
    /// Go `Completed` field.
    #[serde(rename = "completed")]
    pub completed: bool,
    /// Go `Deliverables` field (`None` is a nil slice).
    #[serde(rename = "deliverables")]
    pub deliverables: Option<Vec<String>>,
    /// Go `NextSteps` field.
    #[serde(rename = "next_steps")]
    pub next_steps: String,
}

/// Go `backend/tool/communication/handoff.go` `HandoffInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct HandoffInput {
    /// Go `TaskID` field.
    #[serde(rename = "TaskID")]
    pub task_id: Uuid,
    /// Go `CurrentAgentID` field.
    #[serde(rename = "CurrentAgentID")]
    pub current_agent_id: Uuid,
    /// Go `RequestedAgent` field.
    #[serde(rename = "RequestedAgent")]
    pub requested_agent: String,
    /// Go `HandoverMessage` field.
    #[serde(rename = "HandoverMessage")]
    pub handover_message: String,
}

/// Go `backend/tool/web/fetch.go` `FetchInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct FetchInput {
    /// Go `URL` field.
    #[serde(rename = "url")]
    pub url: String,
    /// Go `Headers` field.
    #[serde(rename = "headers", skip_serializing_if = "crate::json::is_empty")]
    pub headers: Option<BTreeMap<String, String>>,
    /// Go `Timeout` field.
    #[serde(rename = "timeout", skip_serializing_if = "crate::json::is_zero")]
    pub timeout: i64,
}

/// Go `backend/tool/web/fetch.go` `FetchResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct FetchResult {
    /// Go `URL` field.
    #[serde(rename = "url")]
    pub url: String,
    /// Go `Title` field.
    #[serde(rename = "title")]
    pub title: String,
    /// Go `Content` field.
    #[serde(rename = "content")]
    pub content: String,
    /// Go `ContentType` field.
    #[serde(rename = "content_type")]
    pub content_type: String,
    /// Go `ByteSize` field.
    #[serde(rename = "byte_size")]
    pub byte_size: i64,
    /// Go `Truncated` field.
    #[serde(rename = "truncated")]
    pub truncated: bool,
}

/// Go `backend/tool/types/types.go` `InterpreterInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct InterpreterInput {
    /// Go `Script` field.
    #[serde(rename = "script")]
    pub script: String,
}

/// Go `backend/tool/types/types.go` `InterpreterOutput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct InterpreterOutput {
    /// Go `ConsoleOutput` field.
    #[serde(rename = "console_output")]
    pub console_output: String,
    /// Go `FunctionCalls` field (`None` is a nil slice).
    #[serde(rename = "function_calls")]
    pub function_calls: Option<Vec<FunctionCall>>,
    /// Go `ToolStats` field.
    #[serde(rename = "tool_stats", skip_serializing_if = "crate::json::is_empty")]
    pub tool_stats: Option<BTreeMap<String, i64>>,
}

/// Go `backend/tool/types/types.go` `FunctionCall`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct FunctionCall {
    /// Go `ToolName` field.
    #[serde(rename = "tool_name")]
    pub tool_name: String,
    /// Go `Input` field.
    #[serde(rename = "input")]
    pub input: ToolInput,
    /// Go `Output` field.
    #[serde(rename = "output")]
    pub output: ToolOutput,
    /// Go `Index` field.
    #[serde(rename = "index")]
    pub index: i64,
}

/// Go `backend/tool/types/types.go` `ToolInput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ToolInput {
    /// Go `CreateFile` field.
    #[serde(rename = "create_file", skip_serializing_if = "Option::is_none")]
    pub create_file: Option<Box<CreateFileInput>>,
    /// Go `EditFile` field.
    #[serde(rename = "edit_file", skip_serializing_if = "Option::is_none")]
    pub edit_file: Option<Box<EditFileInput>>,
    /// Go `ExecuteCommand` field.
    #[serde(rename = "execute_command", skip_serializing_if = "Option::is_none")]
    pub execute_command: Option<Box<ExecuteCommandInput>>,
    /// Go `FindFile` field.
    #[serde(rename = "find_file", skip_serializing_if = "Option::is_none")]
    pub find_file: Option<Box<FindFileInput>>,
    /// Go `Grep` field.
    #[serde(rename = "grep", skip_serializing_if = "Option::is_none")]
    pub grep: Option<Box<GrepInput>>,
    /// Go `ListFiles` field.
    #[serde(rename = "list_files", skip_serializing_if = "Option::is_none")]
    pub list_files: Option<Box<ListFilesInput>>,
    /// Go `ReadFile` field.
    #[serde(rename = "read_file", skip_serializing_if = "Option::is_none")]
    pub read_file: Option<Box<ReadFileInput>>,
    /// Go `SubmitReport` field.
    #[serde(rename = "submit_report", skip_serializing_if = "Option::is_none")]
    pub submit_report: Option<Box<SubmitReportInput>>,
    /// Go `AskUser` field.
    #[serde(rename = "ask_user", skip_serializing_if = "Option::is_none")]
    pub ask_user: Option<Box<AskUserInput>>,
    /// Go `Handoff` field.
    #[serde(rename = "handoff", skip_serializing_if = "Option::is_none")]
    pub handoff: Option<Box<HandoffInput>>,
    /// Go `Fetch` field.
    #[serde(rename = "fetch", skip_serializing_if = "Option::is_none")]
    pub fetch: Option<Box<FetchInput>>,
    /// Go `Interpreter` field.
    #[serde(rename = "interpreter", skip_serializing_if = "Option::is_none")]
    pub interpreter: Option<Box<InterpreterInput>>,
}

/// Go `backend/tool/types/types.go` `ToolOutput`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ToolOutput {
    /// Go `CreateFile` field.
    #[serde(rename = "create_file", skip_serializing_if = "Option::is_none")]
    pub create_file: Option<Box<CreateFileResult>>,
    /// Go `EditFile` field.
    #[serde(rename = "edit_file", skip_serializing_if = "Option::is_none")]
    pub edit_file: Option<Box<EditFileResult>>,
    /// Go `ExecuteCommand` field.
    #[serde(rename = "execute_command", skip_serializing_if = "Option::is_none")]
    pub execute_command: Option<Box<ExecuteCommandResult>>,
    /// Go `FindFile` field.
    #[serde(rename = "find_file", skip_serializing_if = "Option::is_none")]
    pub find_file: Option<Box<FindFileResult>>,
    /// Go `Grep` field.
    #[serde(rename = "grep", skip_serializing_if = "Option::is_none")]
    pub grep: Option<Box<GrepResult>>,
    /// Go `ListFiles` field.
    #[serde(rename = "list_files", skip_serializing_if = "Option::is_none")]
    pub list_files: Option<Box<ListFilesResult>>,
    /// Go `ReadFile` field.
    #[serde(rename = "read_file", skip_serializing_if = "Option::is_none")]
    pub read_file: Option<Box<ReadFileResult>>,
    /// Go `SubmitReport` field.
    #[serde(rename = "submit_report", skip_serializing_if = "Option::is_none")]
    pub submit_report: Option<Box<SubmitReportResult>>,
    /// Go `AskUser` field.
    #[serde(rename = "ask_user", skip_serializing_if = "Option::is_none")]
    pub ask_user: Option<Box<AskUserResult>>,
    /// Go `Fetch` field.
    #[serde(rename = "fetch", skip_serializing_if = "Option::is_none")]
    pub fetch: Option<Box<FetchResult>>,
    /// Go `Interpreter` field.
    #[serde(rename = "interpreter", skip_serializing_if = "Option::is_none")]
    pub interpreter: Option<Box<InterpreterOutput>>,
}

/// Go `backend/tool/types/types.go` `ToolCallEvent`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ToolCallEvent {
    /// Go `ID` field.
    #[serde(rename = "id")]
    pub id: String,
    /// Go `Tool` field.
    #[serde(rename = "tool")]
    pub tool: String,
    /// Go `Input` field.
    #[serde(rename = "input")]
    pub input: ToolInput,
}

/// Go `backend/tool/types/types.go` `ToolResultEvent`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ToolResultEvent {
    /// Go `ID` field.
    #[serde(rename = "id")]
    pub id: String,
    /// Go `Tool` field.
    #[serde(rename = "tool")]
    pub tool: String,
    /// Go `Output` field.
    #[serde(rename = "output")]
    pub output: ToolOutput,
}

/// Go `backend/tool/types/persistence.go` `ProviderData`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ProviderData {
    /// Go `Kind` field.
    #[serde(rename = "kind")]
    pub kind: String,
    /// Go `ID` field.
    #[serde(rename = "id")]
    pub id: String,
}

/// Go `backend/tool/types/persistence.go` `ToolCall`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ToolCall {
    /// Go `Provider` field.
    #[serde(rename = "provider")]
    pub provider: Option<Box<ProviderData>>,
    /// Go `Tool` field.
    #[serde(rename = "tool")]
    pub tool: String,
    /// Go `Input` field.
    #[serde(rename = "input", skip_serializing_if = "Option::is_none")]
    pub input: Option<Box<ToolInput>>,
}

/// Go `backend/tool/types/persistence.go` `ToolResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ToolResult {
    /// Go `Provider` field.
    #[serde(rename = "provider")]
    pub provider: Option<Box<ProviderData>>,
    /// Go `Tool` field.
    #[serde(rename = "tool")]
    pub tool: String,
    /// Go `Output` field.
    #[serde(rename = "output", skip_serializing_if = "Option::is_none")]
    pub output: Option<Box<ToolOutput>>,
    /// Go `Succeeded` field.
    #[serde(rename = "succeeded")]
    pub succeeded: bool,
}

/// Go `backend/tool/codeact/result.go` `InterpreterToolResult`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct InterpreterToolResult {
    /// Go `ID` field.
    #[serde(rename = "id")]
    pub id: String,
    /// Go `ProviderKind` field.
    #[serde(rename = "provider_kind")]
    pub provider_kind: String,
    /// Go `Output` field.
    #[serde(rename = "output")]
    pub output: String,
    /// Go `FunctionCalls` field (`None` is a nil slice).
    #[serde(rename = "function_calls")]
    pub function_calls: Option<Vec<FunctionCall>>,
    /// Go `ToolStats` field.
    #[serde(rename = "tool_stats", skip_serializing_if = "crate::json::is_empty")]
    pub tool_stats: Option<BTreeMap<String, i64>>,
    /// Go `Error` field.
    #[serde(rename = "error")]
    pub error: String,
}
