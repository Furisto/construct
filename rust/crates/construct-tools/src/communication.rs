//! Reports, Go's ask-user stub, and transactional agent handoff.
use crate::{custom, decode, description, schema};
use construct_core::{
    store::{AgentFilter, ListOptions, Store},
    tool::{Tool, ToolContext, ToolError},
};
use construct_types::{
    entity::Message,
    memory::{MessageBlock, MessageBlockKind, MessageContent, MessageSource},
    tools::*,
};
use serde_json::Value;
use std::sync::Arc;
use uuid::Uuid;

/// Go's currently unimplemented ask-user executor.
#[derive(Clone, Copy, Debug, Default)]
pub struct AskUser;
/// Validates and echoes a work report.
#[derive(Clone, Copy, Debug, Default)]
pub struct SubmitReport;
/// Transactionally reassigns a task and appends the handover message.
pub struct Handoff {
    store: Arc<dyn Store>,
}

impl Handoff {
    /// Inject the store used for all handoff transactions.
    pub fn new(store: Arc<dyn Store>) -> Self {
        Self { store }
    }
}

#[construct_core::async_trait]
impl Tool for AskUser {
    fn name(&self) -> &str {
        "ask_user"
    }
    fn description(&self) -> &str {
        description(self.name())
    }
    fn input_schema(&self) -> Value {
        schema(self.name())
    }
    async fn execute(&self, _ctx: &ToolContext, _value: Value) -> Result<ToolOutput, ToolError> {
        Ok(ToolOutput::default())
    }
}

#[construct_core::async_trait]
impl Tool for SubmitReport {
    fn name(&self) -> &str {
        "submit_report"
    }
    fn description(&self) -> &str {
        description(self.name())
    }
    fn input_schema(&self) -> Value {
        schema(self.name())
    }
    async fn execute(&self, _ctx: &ToolContext, value: Value) -> Result<ToolOutput, ToolError> {
        let input: SubmitReportInput = decode(value)?;
        if input.summary.is_empty() {
            return Err(custom("summary is required", &[
            "Please provide a clear summary of what was accomplished during the work session",
            "The summary should highlight key deliverables, changes made, or outcomes achieved"], &[]));
        }
        Ok(ToolOutput {
            submit_report: Some(Box::new(SubmitReportResult {
                summary: input.summary,
                completed: input.completed,
                deliverables: input.deliverables,
                next_steps: input.next_steps,
            })),
            ..Default::default()
        })
    }
}

#[construct_core::async_trait]
impl Tool for Handoff {
    fn name(&self) -> &str {
        "handoff"
    }
    fn description(&self) -> &str {
        description(self.name())
    }
    fn input_schema(&self) -> Value {
        schema(self.name())
    }
    async fn execute(&self, ctx: &ToolContext, value: Value) -> Result<ToolOutput, ToolError> {
        let mut input: HandoffInput = decode(value)?;
        if input.task_id.is_nil() {
            input.task_id = ctx.task_id.unwrap_or_default();
        }
        if input.current_agent_id.is_nil() {
            input.current_agent_id = ctx.agent_id.unwrap_or_default();
        }
        if input.task_id.is_nil() {
            return Err(custom(
                "task_id is required",
                &["Ensure the task ID is properly set in the session context"],
                &[],
            ));
        }
        if input.current_agent_id.is_nil() {
            return Err(custom(
                "current_agent_id is required",
                &["Ensure the current agent ID is properly set in the session context"],
                &[],
            ));
        }
        if input.requested_agent.is_empty() {
            return Err(custom(
                "requested_agent is required",
                &["Provide the name of the agent to handoff to"],
                &[],
            ));
        }
        let store_error = |e: construct_core::store::StoreError| custom(e.to_string(), &[], &[]);
        let tx = self.store.begin().await.map_err(store_error)?;
        let result = async {
            let current = tx.get_agent(input.current_agent_id).await.map_err(|e| custom(format!("failed to get current agent: {}", lookup_error(e,"agent")), &[
                "This is likely due to a bug in the system or the agent being deleted in the meantime. Retrying this operation probably won't help.", "Ask the user how to proceed"], &[]))?;
            let requested = tx.list_agents(AgentFilter { names: vec![input.requested_agent.clone()], ..Default::default() }, ListOptions::default()).await.map_err(store_error)?.into_iter().next()
                .ok_or_else(|| custom(format!("agent {} does not exist",input.requested_agent), &["Check the agent name and try again"], &[]))?;
            if current.id == requested.id { return Err(custom("agent cannot handoff to itself", &[&format!("You are the source {} agent and cannot handoff to yourself",current.name)], &[])); }
            let mut task = tx.get_task(input.task_id).await.map_err(|e| custom(format!("failed to get task: {}", lookup_error(e,"task")), &[
                "This is likely due to a bug in the system or the task being deleted in the meantime. Retrying this operation probably won't help.", "Ask the user how to proceed"], &[]))?;
            task.agent_id = Some(requested.id);
            let now = construct_types::Timestamp(std::time::SystemTime::now().into());
            task.update_time = now.clone();
            tx.update_task(task).await.map_err(store_error)?;
            let mut message = format!("The {} agent has performed a handoff to you, the {} agent.\n\n", current.name, input.requested_agent);
            if !input.handover_message.is_empty() { message.push_str(&format!("It has left the following instructions for you:\n{}",input.handover_message)); }
            tx.create_message(Message { id: Uuid::new_v4(), task_id: input.task_id, agent_id: Some(current.id), model_id: current.model_id,
                source: MessageSource::ASSISTANT.into(), content: Some(MessageContent { blocks: Some(vec![MessageBlock { kind: MessageBlockKind::TEXT.into(), payload: message }]) }), create_time: now.clone(), update_time: now, ..Default::default() }).await.map_err(store_error)?;
            if ctx.cancellation.is_cancelled() { return Err(custom("context canceled", &[], &[])); }
            Ok(())
        }.await;
        match result {
            Ok(()) => tx.commit().await.map_err(store_error)?,
            Err(error) => {
                let _ = tx.rollback().await;
                return Err(error);
            }
        }
        Ok(ToolOutput::default())
    }
}

fn lookup_error(error: construct_core::store::StoreError, entity: &str) -> String {
    match error {
        construct_core::store::StoreError::NotFound(_) => format!("{entity} not found"),
        other => other.to_string(),
    }
}
