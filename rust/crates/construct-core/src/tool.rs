//! Tool execution contracts. I/O adapters belong to tool constructors, not VM state.
use crate::{event::EventPublisher, CancellationToken};
use async_trait::async_trait;
use construct_types::tools::ToolOutput;
use serde_json::Value;
use std::{collections::BTreeMap, path::PathBuf, sync::Arc};
use uuid::Uuid;

/// Per-call scope. Implementations inject filesystem/command/network/user-I/O
/// adapters at construction (Go passes afero.Fs, CommandRunner, and writers).
/// Handoff tools similarly capture Arc<dyn Store>. There is no live Go ask-user
/// channel to mirror: the base AskUser Execute is currently a stub.
#[derive(Clone)]
pub struct ToolContext {
    /// Task workspace. Not a security sandbox: implementations must enforce
    /// their own path/permission policy and honor explicit absolute tool paths.
    pub workspace_root: PathBuf,
    /// Optional task scope for standalone tools.
    pub task_id: Option<Uuid>,
    /// Calling agent, needed for handoff/audit.
    pub agent_id: Option<Uuid>,
    /// Cooperative cancellation, including child processes.
    pub cancellation: CancellationToken,
    /// Optional hooks for tool/interpreter lifecycle events.
    pub events: Option<Arc<dyn EventPublisher>>,
}

/// Base/codeact error kinds. CodeAct aliases base errors; it adds no distinct
/// kinds. InvalidArgument corresponds to base.InvalidInput (numeric code 11).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(i32)]
pub enum ToolErrorCode {
    /// Custom diagnostic with no standard code.
    Custom = 0,
    /// Expected an absolute path.
    PathIsNotAbsolute = 1,
    /// Expected a file, found directory.
    PathIsDirectory = 2,
    /// Expected directory, found file.
    PathIsNotDirectory = 3,
    /// Operating-system permission failure.
    PermissionDenied = 4,
    /// File missing.
    FileNotFound = 5,
    /// Directory missing.
    DirectoryNotFound = 6,
    /// Metadata lookup failed.
    CannotStatFile = 7,
    /// Other filesystem failure.
    GenericFileError = 8,
    /// Internal implementation failure.
    Internal = 9,
    /// Go's None sentinel.
    None = 10,
    /// InvalidInput in base, InvalidArgument in codeact.
    InvalidArgument = 11,
}

/// Structured tool failure, preserving Go's suggestions and diagnostic details.
#[derive(Clone, Debug)]
pub struct ToolError {
    /// Classification; Go's ToolError itself stores only the formatted message.
    pub code: ToolErrorCode,
    /// Human-readable error.
    pub message: String,
    /// Ordered recovery hints.
    pub suggestions: Vec<String>,
    /// Diagnostic values, never secrets.
    pub details: BTreeMap<String, Value>,
}

/// Go's generic recovery suggestion.
pub const GENERIC_SUGGESTION: &str = "Check the provided system error for more details";

impl ToolError {
    /// Construct Go's standard base/codeact error, including generic then specific suggestions.
    pub fn new(code: ToolErrorCode) -> Self {
        let (message,specific):(&str,&[&str])=match code{
ToolErrorCode::PathIsNotAbsolute=>("Path is not absolute",&["Ensure you're using an absolute path starting with '/'.","Use the list_files tool to get the absolute path of a file."]),
ToolErrorCode::PathIsDirectory=>("Path is a directory",&["You must specify a file path, not a directory."]),
ToolErrorCode::PathIsNotDirectory=>("Path is not a directory",&["You must specify a directory path, not a file."]),
ToolErrorCode::PermissionDenied=>("Permission denied",&["Ensure you have write permissions for the file."]),
ToolErrorCode::FileNotFound=>("File not found",&["Ensure the file exists using the list_files tool."]),
ToolErrorCode::DirectoryNotFound=>("Directory not found",&["Ensure the directory exists using the list_files tool."]),
ToolErrorCode::CannotStatFile=>("Cannot stat file",&["Verify that you have the permission to read the file."]),
ToolErrorCode::GenericFileError=>("File error",&["Check if the provided details can give you more information about the cause of the error.","Verify that you have the permission to read the file."]),
ToolErrorCode::Internal=>("Internal error",&["An internal error occurred. This is a bug with the tool itself. Try to work around it."]),
ToolErrorCode::InvalidArgument=>("Invalid argument",&[]),
_=>("",&[]),
};
        Self {
            code,
            message: message.into(),
            suggestions: std::iter::once(GENERIC_SUGGESTION)
                .chain(specific.iter().copied())
                .map(str::to_owned)
                .collect(),
            details: BTreeMap::new(),
        }
    }
}

impl ToolError {
    /// Custom error with Go's generic suggestion appended.
    pub fn custom(message: impl Into<String>, mut suggestions: Vec<String>) -> Self {
        suggestions.push(GENERIC_SUGGESTION.into());
        Self {
            code: ToolErrorCode::Custom,
            message: message.into(),
            suggestions,
            details: BTreeMap::new(),
        }
    }
}

impl std::fmt::Display for ToolError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.message)?;
        if !self.suggestions.is_empty() {
            write!(f, "\n\nSuggestions:")?;
            for s in &self.suggestions {
                write!(f, "\n- {s}")?;
            }
        }
        if !self.details.is_empty() {
            write!(f, "\n\nDetails:")?;
            for (k, v) in &self.details {
                write!(
                    f,
                    "\n- {k}: {}",
                    v.as_str()
                        .map(str::to_owned)
                        .unwrap_or_else(|| v.to_string())
                )?;
            }
        }
        Ok(())
    }
}
impl std::error::Error for ToolError {}

/// Uniform adapter over native/base/CodeAct tools, with typed persisted output.
#[async_trait]
pub trait Tool: Send + Sync {
    /// Stable tool name.
    fn name(&self) -> &str;
    /// Provider/user-visible description.
    fn description(&self) -> &str;
    /// JSON-schema object matching native.Tool.Schema().
    fn input_schema(&self) -> Value;
    /// Validate input, honor cancellation, and return the matching ToolOutput arm.
    /// VM bindings, output writers, and tool statistics stay in adapters.
    async fn execute(&self, ctx: &ToolContext, input: Value) -> Result<ToolOutput, ToolError>;
}
