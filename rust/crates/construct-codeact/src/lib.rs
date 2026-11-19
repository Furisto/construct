//! Synchronous JavaScript CodeAct execution over Construct's asynchronous host tools.
#![deny(missing_docs)]

mod bindings;
mod interpreter;
mod middleware;
mod result;

pub use bindings::*;
pub use interpreter::*;
pub use middleware::*;

use construct_core::tool::ToolContext;
use construct_types::tools::{FunctionCall, ToolOutput};
use std::{collections::BTreeMap, path::PathBuf};
use uuid::Uuid;

/// Task identity and working directory supplied to an interpretation.
#[derive(Clone, Debug)]
pub struct Task {
    /// Task identifier.
    pub id: Uuid,
    /// Directory injected into execute_command.
    pub project_directory: PathBuf,
}

/// Durable call sequence; failed calls never advance the index.
#[derive(Default)]
pub struct FunctionCallState {
    /// Successful calls, in execution order.
    pub calls: Vec<FunctionCall>,
    /// Index assigned to the next successful call.
    pub index: i64,
}

/// Mutable state private to one script, never shared between interpretations.
pub struct Session {
    /// Task being interpreted.
    pub task: Task,
    /// Calling agent; the nil UUID matches Go's uninitialized session.
    pub agent_id: Uuid,
    /// Host execution scope.
    pub context: ToolContext,
    /// Captured print and report output.
    pub console: String,
    /// Temporary tool result, cleared by ResetTemporarySessionValues.
    pub result: Option<ToolOutput>,
    /// Successful durable calls, allocated lazily like Go's session value.
    pub function_calls: Option<FunctionCallState>,
    /// Attempt counters (including calls that throw).
    pub tool_stats: BTreeMap<String, i64>,
}

impl Session {
    /// Create isolated state for a script.
    pub fn new(context: ToolContext, task: Task) -> Self {
        Self {
            agent_id: Uuid::nil(),
            context,
            task,
            console: String::new(),
            result: None,
            function_calls: None,
            tool_stats: BTreeMap::new(),
        }
    }
}
