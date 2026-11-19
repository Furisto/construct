//! Stored-message reconciliation, independent of RPC serving.
use crate::{
    conv, environment, prompt, provider::ProviderFactory, queue::WorkQueue, title, Result,
};
use construct_codeact::{CodeActTool, Interpreter};
use construct_core::{
    model::{InvokeOptions, ToolDefinition},
    store::{ListOptions, MessageFilter, SortOrder, Store},
    tool::ToolContext,
    CancellationToken,
};
use construct_event::{
    new_message_chunk_event, new_message_created_event, new_task_updated_event, EventPublisher,
    EventRouter,
};
use construct_types::{
    entity::{self, Message, Task},
    memory::{MessageBlock, MessageContent, TaskPhase},
    provider::ContentBlock,
    to_go_json,
    tools::{ToolCall, ToolOutput, ToolResult},
    Timestamp,
};
use std::{
    collections::{BTreeMap, HashMap, HashSet},
    path::Path,
    sync::{
        atomic::{AtomicIsize, Ordering},
        Arc, Mutex,
    },
    time::{Duration, SystemTime},
};
use tokio::task::JoinHandle;
use uuid::Uuid;

pub fn now() -> Timestamp {
    Timestamp(SystemTime::now().into())
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Phase {
    AwaitInput,
    ExecuteTools,
    InvokeModel,
    Suspended,
}

impl Phase {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::AwaitInput => "await_input",
            Self::ExecuteTools => "execute_tools",
            Self::InvokeModel => "invoke_model",
            Self::Suspended => "suspended",
        }
    }
    pub fn stored(self) -> TaskPhase {
        match self {
            Self::AwaitInput => "awaiting",
            Self::ExecuteTools | Self::InvokeModel => "running",
            Self::Suspended => "suspended",
        }
        .into()
    }
}

pub struct TaskStatus<'a> {
    pub phase: Phase,
    pub next_message: Option<&'a Message>,
    pub processed_messages: Vec<&'a Message>,
}

pub fn is_processed(message: &Message) -> bool {
    message
        .processed_time
        .as_ref()
        .is_some_and(|time| *time != Timestamp::default())
}

/// Input order must be ascending create_time, as in Go's message query.
pub fn compute_status<'a>(task: &Task, messages: &'a [Message]) -> TaskStatus<'a> {
    let mut status = TaskStatus {
        phase: Phase::AwaitInput,
        next_message: None,
        processed_messages: Vec::new(),
    };
    if task.desired_phase.as_ref() == "suspended" {
        status.phase = Phase::Suspended;
        return status;
    }
    status.processed_messages = messages.iter().filter(|m| is_processed(m)).collect();
    for (source, phase) in [
        ("system", Phase::InvokeModel),
        ("assistant", Phase::ExecuteTools),
        ("user", Phase::InvokeModel),
    ] {
        if let Some(message) = messages
            .iter()
            .find(|m| !is_processed(m) && m.source.as_ref() == source)
        {
            status.phase = phase;
            status.next_message = Some(message);
            break;
        }
    }
    status
}

#[derive(Default)]
struct Outcome {
    retry: bool,
    retry_after: Option<Duration>,
    error: Option<String>,
}

/// Owns task cancellation, title singleflight and all durable turn transitions.
pub struct TaskReconciler {
    pub(crate) store: Arc<dyn Store>,
    pub(crate) router: Arc<EventRouter>,
    pub(crate) interpreter: Interpreter,
    pub(crate) tools: Vec<Arc<dyn CodeActTool>>,
    pub(crate) skills: Option<construct_skill::Discoverer>,
    pub(crate) providers: Arc<dyn ProviderFactory>,
    pub(crate) queue: Arc<WorkQueue>,
    pub(crate) cancellation: CancellationToken,
    running: Mutex<HashMap<Uuid, CancellationToken>>,
    titles: Mutex<HashSet<Uuid>>,
    title_handles: Mutex<Vec<JoinHandle<()>>>,
    errors: Mutex<HashMap<Uuid, String>>,
    mutations: tokio::sync::Mutex<()>,
}

impl TaskReconciler {
    #[allow(clippy::too_many_arguments)]
    pub(crate) fn new(
        store: Arc<dyn Store>,
        router: Arc<EventRouter>,
        tools: Vec<Arc<dyn CodeActTool>>,
        skills: Option<construct_skill::Discoverer>,
        providers: Arc<dyn ProviderFactory>,
        queue: Arc<WorkQueue>,
        cancellation: CancellationToken,
    ) -> Self {
        let interpreter = Interpreter::new(
            tools.clone(),
            construct_codeact::production_interceptors(router.clone()),
        );
        Self {
            store,
            router,
            tools,
            skills,
            providers,
            queue,
            cancellation,
            interpreter,
            running: Mutex::new(HashMap::new()),
            titles: Mutex::new(HashSet::new()),
            title_handles: Mutex::new(Vec::new()),
            errors: Mutex::new(HashMap::new()),
            mutations: tokio::sync::Mutex::new(()),
        }
    }

    pub fn suspend(&self, id: Uuid) {
        if let Some(token) = self.running.lock().unwrap().get(&id) {
            token.cancel();
        }
    }

    pub fn last_error(&self, id: Uuid) -> Option<String> {
        self.errors.lock().unwrap().get(&id).cloned()
    }

    pub(crate) async fn worker(self: Arc<Self>) {
        while let Some(id) = self.queue.get().await {
            let token = self.cancellation.child_token();
            self.running.lock().unwrap().insert(id, token.clone());
            let outcome = self.reconcile(id, token).await;
            self.running.lock().unwrap().remove(&id);
            match outcome {
                Ok(outcome) => {
                    if let Some(error) = outcome.error {
                        self.errors.lock().unwrap().insert(id, error);
                    }
                    if let Some(delay) = outcome.retry_after {
                        self.queue.retry(id, delay);
                    } else if outcome.retry {
                        self.queue.add(id);
                    }
                }
                Err(error) => {
                    self.errors.lock().unwrap().insert(id, error.to_string());
                }
            }
            self.queue.done(id);
        }
    }

    async fn reconcile(self: &Arc<Self>, id: Uuid, token: CancellationToken) -> Result<Outcome> {
        if token.is_cancelled() {
            return Ok(Outcome::default());
        }
        let task = self.store.get_task(id).await?;
        let agent = self
            .store
            .get_agent(task.agent_id.ok_or("no agent associated with task")?)
            .await?;
        let messages = self
            .store
            .list_messages(
                MessageFilter {
                    task_id: Some(id),
                    ..Default::default()
                },
                ListOptions {
                    sort_order: SortOrder::Ascending,
                    ..Default::default()
                },
            )
            .await?;
        self.schedule_title(&task, &agent, &messages);
        let status = compute_status(&task, &messages);
        self.set_phase(id, status.phase).await?;
        let outcome = match status.phase {
            Phase::AwaitInput | Phase::Suspended => Ok(Outcome::default()),
            Phase::InvokeModel => self.invoke_model(&task, &agent, &status, token).await,
            Phase::ExecuteTools => {
                self.execute_tools(&task, status.next_message.unwrap(), token)
                    .await
            }
        };
        self.set_phase(id, Phase::AwaitInput).await?;
        outcome
    }

    async fn set_phase(&self, id: Uuid, phase: Phase) -> Result<()> {
        let _write = self.mutations.lock().await;
        let tx = self.store.begin().await?;
        let mut task = tx.get_task(id).await?;
        let previous = task.phase.to_string();
        task.phase = phase.stored();
        task.update_time = now();
        let updated = tx.update_task(task).await?;
        tx.commit().await?;
        self.router
            .publish(new_task_updated_event(updated, previous));
        Ok(())
    }

    fn schedule_title(self: &Arc<Self>, task: &Task, agent: &entity::Agent, messages: &[Message]) {
        if !task.description.is_empty() || !messages.iter().any(|m| m.source.as_ref() == "user") {
            return;
        }
        if !self.titles.lock().unwrap().insert(task.id) {
            return;
        }
        let this = self.clone();
        let id = task.id;
        let model_id = agent.model_id;
        let messages = messages.to_vec();
        let handle = tokio::spawn(async move {
            let result = tokio::select! {
                _ = this.cancellation.cancelled() => return,
                result = this.generate_title(id, model_id, &messages) => result,
            };
            if let Err(error) = result {
                this.errors
                    .lock()
                    .unwrap()
                    .insert(id, format!("title: {error}"));
            }
        });
        let mut handles = self.title_handles.lock().unwrap();
        handles.retain(|handle| !handle.is_finished());
        handles.push(handle);
    }

    async fn generate_title(
        &self,
        id: Uuid,
        model_id: Option<Uuid>,
        messages: &[Message],
    ) -> Result<()> {
        let model = self
            .store
            .get_model(model_id.ok_or("agent has no model")?)
            .await?;
        let provider_record = self
            .store
            .get_model_provider(model.model_provider_id)
            .await?;
        let provider = self.providers.create(model.model_provider_id).await?;
        if let Some(title) = title::generate_title(
            provider.as_ref(),
            provider_record.provider_type.as_ref(),
            messages,
        )
        .await?
        {
            let _write = self.mutations.lock().await;
            let tx = self.store.begin().await?;
            let mut task = tx.get_task(id).await?;
            task.description = title;
            task.update_time = now();
            tx.update_task(task).await?;
            tx.commit().await?;
        }
        Ok(())
    }

    pub(crate) async fn join_titles(&self) {
        let handles = std::mem::take(&mut *self.title_handles.lock().unwrap());
        for handle in handles {
            let _ = handle.await;
        }
    }

    fn system_prompt(&self, task: &Task, agent: &entity::Agent) -> Result<String> {
        prompt::assemble_system_prompt(
            &agent.instructions,
            &prompt::PromptContext {
                working_directory: task.project_directory.clone(),
                operating_system: match std::env::consts::OS {
                    "macos" => "darwin",
                    other => other,
                }
                .into(),
                default_shell: environment::default_shell()
                    .map(|s| s.name)
                    .unwrap_or_default(),
                project_structure: environment::project_structure(&task.project_directory)
                    .unwrap_or_default(),
                dev_tools: environment::available_dev_tools(),
                tools: self
                    .tools
                    .iter()
                    .map(|tool| (tool.name().into(), tool.description().into()))
                    .collect(),
                skills: self
                    .skills
                    .as_ref()
                    .map(|skills| skills.discover(Path::new(&task.project_directory)))
                    .unwrap_or_default(),
            },
        )
    }

    async fn invoke_model(
        &self,
        task: &Task,
        agent: &entity::Agent,
        status: &TaskStatus<'_>,
        token: CancellationToken,
    ) -> Result<Outcome> {
        let next = status.next_message.unwrap();
        if next.source.as_ref() == "user" {
            self.router.publish(new_message_created_event(next.clone()));
        }
        let history = status
            .processed_messages
            .iter()
            .copied()
            .chain(std::iter::once(next))
            .map(conv::memory_message_to_model)
            .collect::<Result<Vec<_>>>()?;
        let model = self
            .store
            .get_model(agent.model_id.ok_or("agent has no model")?)
            .await?;
        let provider = self.providers.create(model.model_provider_id).await?;
        let prompt = self.system_prompt(task, agent)?;
        let message_id = Uuid::new_v4();
        let router = self.router.clone();
        let task_id = task.id;
        let index = AtomicIsize::new(0);
        let options = InvokeOptions {
            cancellation: token.clone(),
            tools: vec![ToolDefinition {
                name: self.interpreter.name().into(),
                description: self.interpreter.description().into(),
                input_schema: self.interpreter.input_schema(),
            }],
            text_sink: Some(Arc::new(move |chunk| {
                router.publish(new_message_chunk_event(
                    task_id,
                    message_id,
                    chunk,
                    index.fetch_add(1, Ordering::SeqCst),
                ))
            })),
            ..Default::default()
        };
        let response = tokio::select! {
            biased;
            _ = token.cancelled() => None,
            result = provider.invoke_model(&model.name, &prompt, &history, options) => Some(result),
        };
        let response = match response {
            None => {
                self.mark_processed(next.id).await?;
                return Ok(Outcome::default());
            }
            Some(Err(error)) if token.is_cancelled() || error.kind.as_ref() == "canceled" => {
                self.mark_processed(next.id).await?;
                return Ok(Outcome::default());
            }
            Some(Err(error)) => {
                let (retryable, nanos) = error.retryable();
                return Ok(Outcome {
                    retry_after: (retryable && nanos > 0)
                        .then(|| Duration::from_nanos(nanos as u64)),
                    error: Some(error.to_string()),
                    ..Default::default()
                });
            }
            Some(Ok(response)) => response,
        };
        let cost = response.usage.input_tokens as f64 * model.input_cost / 1_000_000.0
            + response.usage.output_tokens as f64 * model.output_cost / 1_000_000.0
            + response.usage.cache_write_tokens as f64 * model.cache_write_cost / 1_000_000.0
            + response.usage.cache_read_tokens as f64 * model.cache_read_cost / 1_000_000.0;
        let has_tools = response
            .content
            .as_deref()
            .unwrap_or_default()
            .iter()
            .any(|block| matches!(block, ContentBlock::ToolCall(_)));
        let mut message = Message {
            id: message_id,
            task_id: task.id,
            source: "assistant".into(),
            content: Some(conv::model_blocks_to_memory(
                response.content.as_deref().unwrap_or_default(),
                "",
            )?),
            usage: Some(construct_types::memory::MessageUsage {
                input_tokens: response.usage.input_tokens,
                output_tokens: response.usage.output_tokens,
                cache_write_tokens: response.usage.cache_write_tokens,
                cache_read_tokens: response.usage.cache_read_tokens,
                cost,
            }),
            processed_time: (!has_tools).then(now),
            create_time: now(),
            update_time: now(),
            ..Default::default()
        };
        let _write = self.mutations.lock().await;
        let tx = self.store.begin().await?;
        let mut input = tx.get_message(next.id).await?;
        input.processed_time = Some(now());
        input.update_time = now();
        tx.update_message(input).await?;
        message = tx.create_message(message).await?;
        let mut task = tx.get_task(task.id).await?;
        task.input_tokens += response.usage.input_tokens;
        task.output_tokens += response.usage.output_tokens;
        task.cache_write_tokens += response.usage.cache_write_tokens;
        task.cache_read_tokens += response.usage.cache_read_tokens;
        task.cost += cost;
        task.update_time = now();
        tx.update_task(task).await?;
        tx.commit().await?;
        self.router.publish(new_message_created_event(message));
        Ok(Outcome {
            retry: true,
            ..Default::default()
        })
    }

    async fn mark_processed(&self, id: Uuid) -> Result<()> {
        let _write = self.mutations.lock().await;
        let tx = self.store.begin().await?;
        let mut message = tx.get_message(id).await?;
        message.processed_time = Some(now());
        message.update_time = now();
        tx.update_message(message).await?;
        tx.commit().await?;
        Ok(())
    }

    async fn execute_tools(
        &self,
        task: &Task,
        message: &Message,
        token: CancellationToken,
    ) -> Result<Outcome> {
        let mut results = Vec::new();
        let mut stats: BTreeMap<String, i64> = BTreeMap::new();
        let mut error = None;
        for block in message
            .content
            .as_ref()
            .and_then(|content| content.blocks.as_deref())
            .unwrap_or_default()
        {
            if block.kind.as_ref() != "tool_call" {
                continue;
            }
            let call: ToolCall = match serde_json::from_str(&block.payload) {
                Ok(call) => call,
                Err(err) => {
                    error = Some(format!("failed to unmarshal tool call: {err}"));
                    results.clear();
                    stats.clear();
                    break;
                }
            };
            if call.tool != "code_interpreter" {
                continue;
            }
            let Some(input) = call
                .input
                .as_ref()
                .and_then(|input| input.interpreter.as_deref())
            else {
                continue;
            };
            let context = ToolContext {
                workspace_root: (&task.project_directory).into(),
                task_id: Some(task.id),
                agent_id: task.agent_id,
                cancellation: token.clone(),
                events: Some(self.router.clone()),
            };
            let (output, failure) = self
                .interpreter
                .interpret(
                    context,
                    input.clone(),
                    construct_codeact::Task {
                        id: task.id,
                        project_directory: (&task.project_directory).into(),
                    },
                )
                .await;
            if let Some(counts) = &output.tool_stats {
                for (name, count) in counts {
                    *stats.entry(name.clone()).or_default() += count;
                }
            }
            let result = ToolResult {
                provider: call.provider,
                tool: call.tool,
                output: Some(Box::new(ToolOutput {
                    interpreter: Some(Box::new(output)),
                    ..Default::default()
                })),
                succeeded: failure.is_none() && !token.is_cancelled(),
            };
            results.push(MessageBlock {
                kind: "tool_result".into(),
                payload: String::from_utf8(to_go_json(&result)?)?,
            });
        }
        let _write = self.mutations.lock().await;
        let tx = self.store.begin().await?;
        let mut input = tx.get_message(message.id).await?;
        input.processed_time = Some(now());
        input.update_time = now();
        tx.update_message(input).await?;
        if !results.is_empty() {
            tx.create_message(Message {
                id: Uuid::new_v4(),
                task_id: task.id,
                source: "system".into(),
                content: Some(MessageContent {
                    blocks: Some(results),
                }),
                create_time: now(),
                update_time: now(),
                ..Default::default()
            })
            .await?;
            let mut task = tx.get_task(task.id).await?;
            task.tool_uses = Some(stats);
            task.update_time = now();
            tx.update_task(task).await?;
        }
        tx.commit().await?;
        Ok(Outcome {
            retry: true,
            error,
            ..Default::default()
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn status_priority_zero_time_unknown_and_suspended() {
        let mut task = Task::default();
        assert_eq!(compute_status(&task, &[]).phase, Phase::AwaitInput);
        let mut messages: Vec<_> = ["user", "assistant", "system"]
            .iter()
            .map(|source| Message {
                id: Uuid::new_v4(),
                source: (*source).into(),
                processed_time: Some(Timestamp::default()),
                ..Default::default()
            })
            .collect();
        let status = compute_status(&task, &messages);
        assert_eq!(status.next_message.unwrap().id, messages[2].id);
        assert_eq!(status.phase, Phase::InvokeModel);
        messages[2].processed_time = Some(now());
        assert_eq!(compute_status(&task, &messages).phase, Phase::ExecuteTools);
        messages[1].processed_time = Some(now());
        assert_eq!(
            compute_status(&task, &messages).next_message.unwrap().id,
            messages[0].id
        );
        messages[0].processed_time = Some(now());
        assert_eq!(compute_status(&task, &messages).phase, Phase::AwaitInput);
        assert_eq!(compute_status(&task, &messages).processed_messages.len(), 3);
        task.desired_phase = "suspended".into();
        assert_eq!(compute_status(&task, &messages).phase, Phase::Suspended);
        assert!(compute_status(&task, &messages)
            .processed_messages
            .is_empty());
        assert_eq!(Phase::InvokeModel.stored().as_ref(), "running");
        assert_eq!(Phase::ExecuteTools.stored().as_ref(), "running");
    }
}
