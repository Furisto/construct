use construct_agent::{
    provider::ProviderFactory,
    reconciler::{compute_status, now, Phase},
    title,
    wire::message_status,
    Runtime, RuntimeOptions,
};
use construct_core::{
    async_trait,
    model::{InvokeOptions, ModelProvider},
    store::{ListOptions, MessageFilter, SortOrder},
    testing::{ModelStep, ScriptedModelProvider},
    CancellationToken,
};
use construct_event::{
    new_internal_task_suspend_event, new_internal_task_trigger_event, Event, EventPublisher,
    EventSubscriber, MessageChunkPayload, MessageEventPayload, SubscribeOptions, Subscription,
    TaskEventPayload,
};
use construct_secret::{Encryption, Keyset};
use construct_types::{
    entity::{Agent, Message, Model, ModelProvider as ProviderRecord, Task},
    memory::{MessageBlock, MessageContent},
    provider::{self, ContentBlock, ProviderError, TextBlock, ToolCallBlock, Usage},
    to_go_json,
    tools::ToolResult,
};
use futures_util::StreamExt;
use std::{
    sync::{
        atomic::{AtomicBool, AtomicUsize, Ordering},
        Arc,
    },
    time::Duration,
};
use tokio::{sync::Semaphore, task::JoinHandle};
use uuid::Uuid;

fn text(value: &str) -> provider::Message {
    provider::Message {
        source: "model".into(),
        content: Some(vec![ContentBlock::Text(TextBlock { text: value.into() })]),
        ..Default::default()
    }
}
fn step(result: Result<provider::Message, ProviderError>) -> ModelStep {
    ModelStep {
        expected_model: None,
        chunks: Vec::new(),
        retries: Vec::new(),
        result,
    }
}
fn error(kind: &str, delay: i64) -> ProviderError {
    ProviderError {
        provider: "anthropic".into(),
        kind: kind.into(),
        retry_after: delay,
        err: Some("test error".into()),
    }
}
fn tool(script: &str) -> provider::Message {
    provider::Message {
        source: "model".into(),
        content: Some(vec![ContentBlock::ToolCall(ToolCallBlock {
            id: "call-1".into(),
            tool: "code_interpreter".into(),
            args: Some(
                serde_json::value::RawValue::from_string(
                    serde_json::json!({"script":script}).to_string(),
                )
                .unwrap(),
            ),
        })]),
        ..Default::default()
    }
}

struct TestProvider {
    script: ScriptedModelProvider,
    titles: ScriptedModelProvider,
    block: AtomicBool,
    started: Semaphore,
    gate: Semaphore,
    cancelled: AtomicBool,
    active: AtomicUsize,
    max_active: AtomicUsize,
}
impl TestProvider {
    fn new(steps: Vec<ModelStep>, block: bool) -> Arc<Self> {
        Arc::new(Self {
            script: ScriptedModelProvider::new(steps),
            titles: ScriptedModelProvider::new([step(Ok(text("\"A generated title\"")))]),
            block: AtomicBool::new(block),
            started: Semaphore::new(0),
            gate: Semaphore::new(0),
            cancelled: AtomicBool::new(false),
            active: AtomicUsize::new(0),
            max_active: AtomicUsize::new(0),
        })
    }
    async fn entered(&self) {
        tokio::time::timeout(Duration::from_secs(5), self.started.acquire())
            .await
            .unwrap()
            .unwrap()
            .forget();
    }
}
struct InvocationGuard<'a> {
    provider: &'a TestProvider,
    cancellation: CancellationToken,
}
impl Drop for InvocationGuard<'_> {
    fn drop(&mut self) {
        self.provider.active.fetch_sub(1, Ordering::SeqCst);
        if self.cancellation.is_cancelled() {
            self.provider.cancelled.store(true, Ordering::SeqCst);
        }
    }
}
#[async_trait]
impl ModelProvider for TestProvider {
    async fn invoke_model(
        &self,
        model: &str,
        prompt: &str,
        messages: &[provider::Message],
        options: InvokeOptions,
    ) -> Result<provider::Message, ProviderError> {
        if prompt == title::SYSTEM_PROMPT {
            return self
                .titles
                .invoke_model(model, prompt, messages, options)
                .await;
        }
        let active = self.active.fetch_add(1, Ordering::SeqCst) + 1;
        self.max_active.fetch_max(active, Ordering::SeqCst);
        let _guard = InvocationGuard {
            provider: self,
            cancellation: options.cancellation.clone(),
        };
        self.started.add_permits(1);
        if self.block.load(Ordering::SeqCst) {
            tokio::select! {
                permit = self.gate.acquire() => permit.unwrap().forget(),
                _ = options.cancellation.cancelled() => return Err(error("canceled", 0)),
            }
        }
        self.script
            .invoke_model(model, prompt, messages, options)
            .await
    }
}
struct Factory(Arc<TestProvider>);
#[async_trait]
impl ProviderFactory for Factory {
    async fn create(&self, _id: Uuid) -> construct_agent::Result<Arc<dyn ModelProvider>> {
        Ok(self.0.clone())
    }
}

struct Harness {
    _directory: tempfile::TempDir,
    runtime: Arc<Runtime>,
    provider: Arc<TestProvider>,
    task: Uuid,
    user: Uuid,
}
impl Harness {
    async fn new(steps: Vec<ModelStep>, block: bool, title: &str) -> Self {
        let directory = tempfile::tempdir().unwrap();
        let provider = TestProvider::new(steps, block);
        let encryption = Arc::new(Encryption::new(Keyset::generate()).unwrap());
        let options = RuntimeOptions {
            tools: vec![construct_codeact::print(), construct_codeact::read_file()],
            concurrency: 4,
            skills: None,
            provider_factory: Some(Arc::new(Factory(provider.clone()))),
            ..Default::default()
        };
        let runtime = Arc::new(
            Runtime::open_sqlite(directory.path().join("memory.db"), encryption, options)
                .await
                .unwrap(),
        );
        let store = runtime.store();
        let provider_id = Uuid::new_v4();
        let model_id = Uuid::new_v4();
        let agent_id = Uuid::new_v4();
        let task_id = Uuid::new_v4();
        let user_id = Uuid::new_v4();
        store
            .create_model_provider(ProviderRecord {
                id: provider_id,
                name: "test-provider".into(),
                provider_type: "anthropic".into(),
                secret: vec![1],
                enabled: true,
                create_time: now(),
                update_time: now(),
                ..Default::default()
            })
            .await
            .unwrap();
        store
            .create_model(Model {
                id: model_id,
                name: "conversation".into(),
                model_provider_id: provider_id,
                input_cost: 2.0,
                output_cost: 4.0,
                cache_read_cost: 1.0,
                cache_write_cost: 3.0,
                enabled: true,
                create_time: now(),
                update_time: now(),
                ..Default::default()
            })
            .await
            .unwrap();
        store
            .create_agent(Agent {
                id: agent_id,
                name: "test-agent".into(),
                instructions: "Work in {{.WorkingDirectory}}\n{{.Tools}}".into(),
                model_id: Some(model_id),
                create_time: now(),
                update_time: now(),
                ..Default::default()
            })
            .await
            .unwrap();
        store
            .create_task(Task {
                id: task_id,
                agent_id: Some(agent_id),
                description: title.into(),
                project_directory: directory.path().to_string_lossy().into(),
                phase: "awaiting".into(),
                desired_phase: "running".into(),
                create_time: now(),
                update_time: now(),
                ..Default::default()
            })
            .await
            .unwrap();
        store
            .create_message(Message {
                id: user_id,
                task_id,
                source: "user".into(),
                content: Some(MessageContent {
                    blocks: Some(vec![MessageBlock {
                        kind: "text".into(),
                        payload: "do the work".into(),
                    }]),
                }),
                create_time: now(),
                update_time: now(),
                ..Default::default()
            })
            .await
            .unwrap();
        Self {
            _directory: directory,
            runtime,
            provider,
            task: task_id,
            user: user_id,
        }
    }
    async fn start(&self) -> JoinHandle<construct_agent::Result<()>> {
        let runtime = self.runtime.clone();
        let handle = tokio::spawn(runtime.run(CancellationToken::new()));
        self.runtime.wait_until_running().await;
        handle
    }
    fn trigger(&self) {
        self.runtime
            .event_router()
            .publish(new_internal_task_trigger_event(self.task));
    }
    fn subscribe(&self) -> Subscription {
        self.runtime.event_router().subscribe(SubscribeOptions {
            task_id: self.task.to_string(),
            ..Default::default()
        })
    }
    async fn messages(&self) -> Vec<Message> {
        self.runtime
            .store()
            .list_messages(
                MessageFilter {
                    task_id: Some(self.task),
                    ..Default::default()
                },
                ListOptions {
                    sort_order: SortOrder::Ascending,
                    ..Default::default()
                },
            )
            .await
            .unwrap()
    }
    async fn stop(&self, handle: JoinHandle<construct_agent::Result<()>>) {
        tokio::time::timeout(Duration::from_secs(5), self.runtime.shutdown())
            .await
            .unwrap();
        handle.await.unwrap().unwrap();
        assert_eq!(self.provider.active.load(Ordering::SeqCst), 0);
        assert_eq!(
            self.runtime
                .metrics()
                .depth
                .with_label_values(&["construct"])
                .get(),
            0.0
        );
    }
}
async fn next(events: &mut Subscription) -> Arc<Event> {
    tokio::time::timeout(Duration::from_secs(5), events.next())
        .await
        .unwrap()
        .unwrap()
}
async fn final_message(events: &mut Subscription) -> (Message, Vec<Arc<Event>>) {
    let mut all = Vec::new();
    loop {
        let event = next(events).await;
        let final_message = event
            .payload
            .downcast_ref::<MessageEventPayload>()
            .filter(|payload| message_status(&payload.message).is_final_response)
            .map(|payload| payload.message.clone());
        all.push(event);
        if let Some(message) = final_message {
            return (message, all);
        }
    }
}
async fn phase_event(events: &mut Subscription, phase: &str) {
    loop {
        let event = next(events).await;
        if event
            .payload
            .downcast_ref::<TaskEventPayload>()
            .is_some_and(|payload| payload.task.phase.as_ref() == phase)
        {
            return;
        }
    }
}

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn sqlite_tool_round_trip_streaming_usage_events_and_single_title() {
    let mut first = step(Ok(tool("print('hello');")));
    first.chunks = vec!["thinking".into()];
    let mut response = text("All done");
    response.usage = Usage {
        input_tokens: 10,
        output_tokens: 20,
        cache_write_tokens: 30,
        cache_read_tokens: 40,
    };
    let mut last = step(Ok(response));
    last.chunks = vec!["All ".into(), "done".into()];
    let h = Harness::new(vec![first, last], false, "").await;
    let mut events = h.subscribe();
    let handle = h.start().await;
    h.trigger();
    let (message, all) = final_message(&mut events).await;
    phase_event(&mut events, "awaiting").await;
    assert!(message_status(&message).is_final_response);
    assert_eq!(message.usage.as_ref().unwrap().cache_read_tokens, 40);
    assert_eq!(to_go_json(message.usage.as_ref().unwrap()).unwrap(), br#"{"input_tokens":10,"output_tokens":20,"cache_write_tokens":30,"cache_read_tokens":40,"cost":0.00023}"#);
    let messages = h.messages().await;
    assert_eq!(messages.len(), 4);
    assert!(messages.iter().all(|m| m.processed_time.is_some()));
    assert!(!message_status(&messages[1]).is_final_response);
    assert_eq!(messages[2].source.as_ref(), "system");
    let result_payload = &messages[2]
        .content
        .as_ref()
        .unwrap()
        .blocks
        .as_ref()
        .unwrap()[0]
        .payload;
    assert_eq!(
        result_payload,
        r#"{"provider":{"kind":"","id":"call-1"},"tool":"code_interpreter","output":{"interpreter":{"console_output":"hello\n","function_calls":[]}},"succeeded":true}"#
    );
    let result: ToolResult = serde_json::from_str(result_payload).unwrap();
    assert!(result.succeeded);
    assert_eq!(to_go_json(&result).unwrap(), result_payload.as_bytes());
    let created: Vec<_> = all
        .iter()
        .filter_map(|e| e.payload.downcast_ref::<MessageEventPayload>())
        .collect();
    assert_eq!(
        created
            .iter()
            .map(|p| p.message.source.as_ref())
            .collect::<Vec<_>>(),
        ["user", "assistant", "assistant"]
    );
    let chunks: Vec<_> = all
        .iter()
        .filter_map(|e| e.payload.downcast_ref::<MessageChunkPayload>())
        .collect();
    assert_eq!(
        chunks
            .iter()
            .map(|p| (p.chunk.as_str(), p.chunk_index))
            .collect::<Vec<_>>(),
        [("thinking", 0), ("All ", 0), ("done", 1)]
    );
    assert_eq!(chunks[0].message_id, messages[1].id);
    assert_eq!(chunks[1].message_id, message.id);
    let types: Vec<_> = all.iter().map(|event| event.r#type.as_str()).collect();
    assert_eq!(
        types,
        [
            "task.updated",
            "message.created",
            "message.chunk",
            "message.created",
            "task.updated",
            "task.updated",
            "task.updated",
            "task.updated",
            "message.chunk",
            "message.chunk",
            "message.created"
        ]
    );
    let task = h.runtime.store().get_task(h.task).await.unwrap();
    assert_eq!(task.input_tokens, 10);
    assert_eq!(task.output_tokens, 20);
    assert_eq!(task.cache_read_tokens, 40);
    assert_eq!(task.cache_write_tokens, 30);
    assert_eq!(task.cost, 0.00023);
    assert!(task.tool_uses.unwrap().is_empty());
    assert_eq!(
        compute_status(
            &h.runtime.store().get_task(h.task).await.unwrap(),
            &messages
        )
        .phase,
        Phase::AwaitInput
    );
    let calls = h.provider.script.calls();
    assert_eq!(calls.len(), 2);
    assert_eq!(calls[1].messages.len(), 3);
    assert_eq!(calls[0].tools[0].name, "code_interpreter");
    assert!(calls[0].system_prompt.contains("# print"));
    assert!(
        matches!(&calls[1].messages[2].content.as_ref().unwrap()[0], ContentBlock::ToolResult(result) if result.result == "hello\n" && result.succeeded)
    );
    tokio::time::timeout(Duration::from_secs(5), async {
        loop {
            if h.runtime
                .store()
                .get_task(h.task)
                .await
                .unwrap()
                .description
                == "A generated title"
            {
                break;
            }
            tokio::task::yield_now().await;
        }
    })
    .await
    .unwrap();
    for _ in 0..100 {
        h.trigger();
    }
    phase_event(&mut events, "awaiting").await;
    h.stop(handle).await;
    assert_eq!(h.provider.titles.calls().len(), 1);
    assert_eq!(h.provider.script.remaining(), 0);
    assert_eq!(h.provider.max_active.load(Ordering::SeqCst), 1);
    assert!(h.runtime.reconciler().last_error(h.task).is_none());
}

#[tokio::test]
async fn suspend_cancels_running_model_and_deduplicates_concurrent_triggers() {
    let h = Harness::new(
        vec![step(Ok(text("should not persist")))],
        true,
        "already titled",
    )
    .await;
    let mut events = h.subscribe();
    let handle = h.start().await;
    h.trigger();
    h.provider.entered().await;
    let mut triggers = tokio::task::JoinSet::new();
    for _ in 0..100 {
        let router = h.runtime.event_router();
        let id = h.task;
        triggers.spawn(async move {
            router.publish(new_internal_task_trigger_event(id));
        });
    }
    while triggers.join_next().await.is_some() {}
    let store = h.runtime.store();
    let mut task = store.get_task(h.task).await.unwrap();
    task.desired_phase = "suspended".into();
    store.update_task(task).await.unwrap();
    h.runtime
        .event_router()
        .publish(new_internal_task_suspend_event(h.task));
    loop {
        let event = next(&mut events).await;
        if event
            .payload
            .downcast_ref::<TaskEventPayload>()
            .is_some_and(|p| p.task.phase.as_ref() == "suspended")
        {
            break;
        }
    }
    phase_event(&mut events, "awaiting").await;
    h.stop(handle).await;
    assert!(h.provider.cancelled.load(Ordering::SeqCst));
    assert_eq!(h.provider.max_active.load(Ordering::SeqCst), 1);
    assert!(h.provider.script.calls().is_empty());
    let messages = h.messages().await;
    assert_eq!(messages.len(), 1);
    assert!(messages[0].processed_time.is_some());
    assert_eq!(
        compute_status(&store.get_task(h.task).await.unwrap(), &messages).phase,
        Phase::Suspended
    );
}

#[tokio::test]
async fn retryable_provider_error_is_delayed_and_requeued_without_consuming_input() {
    let h = Harness::new(
        vec![
            step(Err(error("rate_limit_exceeded", 60_000_000))),
            step(Ok(text("retried"))),
        ],
        false,
        "already titled",
    )
    .await;
    let mut events = h.subscribe();
    let handle = h.start().await;
    let start = tokio::time::Instant::now();
    h.trigger();
    let (_, _) = final_message(&mut events).await;
    assert!(start.elapsed() >= Duration::from_millis(60));
    phase_event(&mut events, "awaiting").await;
    h.stop(handle).await;
    let calls = h.provider.script.calls();
    assert_eq!(calls.len(), 2);
    assert_eq!(
        to_go_json(&calls[0].messages).unwrap(),
        to_go_json(&calls[1].messages).unwrap()
    );
    assert_eq!(h.messages().await.len(), 2);
    assert_eq!(
        h.runtime
            .metrics()
            .retries
            .with_label_values(&["construct"])
            .get(),
        1
    );
}

#[tokio::test]
async fn terminal_provider_error_does_not_requeue_or_mark_input_processed() {
    let h = Harness::new(
        vec![step(Err(error("invalid_request", 0)))],
        false,
        "already titled",
    )
    .await;
    let mut events = h.subscribe();
    let handle = h.start().await;
    h.trigger();
    phase_event(&mut events, "running").await;
    phase_event(&mut events, "awaiting").await;
    h.stop(handle).await;
    assert_eq!(h.provider.script.calls().len(), 1);
    assert!(h
        .runtime
        .store()
        .get_message(h.user)
        .await
        .unwrap()
        .processed_time
        .is_none());
    assert!(h
        .runtime
        .reconciler()
        .last_error(h.task)
        .unwrap()
        .contains("Invalid request"));
}

#[tokio::test]
async fn failed_tool_result_is_persisted_then_model_finishes() {
    let h = Harness::new(
        vec![
            step(Ok(tool("throw new Error('bad script')"))),
            step(Ok(text("recovered"))),
        ],
        false,
        "already titled",
    )
    .await;
    let mut events = h.subscribe();
    let handle = h.start().await;
    h.trigger();
    final_message(&mut events).await;
    phase_event(&mut events, "awaiting").await;
    h.stop(handle).await;
    let messages = h.messages().await;
    let result: ToolResult = serde_json::from_str(
        &messages[2]
            .content
            .as_ref()
            .unwrap()
            .blocks
            .as_ref()
            .unwrap()[0]
            .payload,
    )
    .unwrap();
    assert!(!result.succeeded);
    assert!(result.output.unwrap().interpreter.is_some());
}

#[tokio::test]
async fn production_interceptors_publish_tool_events_and_persist_durable_calls() {
    let path =
        std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures/tool-result.json");
    let script = format!(
        "read_file({}); print('read');",
        serde_json::to_string(&path).unwrap()
    );
    let h = Harness::new(
        vec![step(Ok(tool(&script))), step(Ok(text("done")))],
        false,
        "already titled",
    )
    .await;
    let mut events = h.subscribe();
    let handle = h.start().await;
    h.trigger();
    let (_, all) = final_message(&mut events).await;
    phase_event(&mut events, "awaiting").await;
    h.stop(handle).await;
    let tool_events: Vec<_> = all
        .iter()
        .filter(|event| event.r#type.starts_with("tool."))
        .map(|event| event.r#type.as_str())
        .collect();
    assert_eq!(tool_events, ["tool.called", "tool.result"]);
    let messages = h.messages().await;
    let result: ToolResult = serde_json::from_str(
        &messages[2]
            .content
            .as_ref()
            .unwrap()
            .blocks
            .as_ref()
            .unwrap()[0]
            .payload,
    )
    .unwrap();
    let output = result.output.unwrap().interpreter.unwrap();
    assert_eq!(output.function_calls.unwrap()[0].tool_name, "read_file");
    assert_eq!(output.tool_stats.unwrap().get("read_file"), Some(&1));
    assert_eq!(
        h.runtime
            .store()
            .get_task(h.task)
            .await
            .unwrap()
            .tool_uses
            .unwrap()
            .get("read_file"),
        Some(&1)
    );
}

#[tokio::test]
async fn suspend_interrupts_running_interpreter_and_waits_for_its_result() {
    let h = Harness::new(
        vec![step(Ok(tool("while (true) {}")))],
        false,
        "already titled",
    )
    .await;
    let mut events = h.subscribe();
    let handle = h.start().await;
    h.trigger();
    loop {
        let event = next(&mut events).await;
        if event
            .payload
            .downcast_ref::<MessageEventPayload>()
            .is_some_and(|payload| payload.message.source.as_ref() == "assistant")
        {
            break;
        }
    }
    phase_event(&mut events, "running").await;
    let store = h.runtime.store();
    let mut task = store.get_task(h.task).await.unwrap();
    task.desired_phase = "suspended".into();
    store.update_task(task).await.unwrap();
    h.runtime
        .event_router()
        .publish(new_internal_task_suspend_event(h.task));
    phase_event(&mut events, "suspended").await;
    phase_event(&mut events, "awaiting").await;
    h.stop(handle).await;
    let messages = h.messages().await;
    assert_eq!(messages.len(), 3);
    assert!(messages[1].processed_time.is_some());
    let result: ToolResult = serde_json::from_str(
        &messages[2]
            .content
            .as_ref()
            .unwrap()
            .blocks
            .as_ref()
            .unwrap()[0]
            .payload,
    )
    .unwrap();
    assert!(!result.succeeded);
    assert_eq!(h.provider.script.calls().len(), 1);
}

#[tokio::test]
async fn empty_history_hooks_accessors_and_shutdown_of_active_work() {
    assert_eq!(RuntimeOptions::default().concurrency, 50);
    let h = Harness::new(vec![], true, "already titled").await;
    assert!(Runtime::new(
        h.runtime.store(),
        h.runtime.encryption(),
        RuntimeOptions {
            concurrency: 0,
            ..Default::default()
        }
    )
    .is_err());
    let store = h.runtime.store();
    store.delete_message(h.user).await.unwrap();
    let mut events = h.subscribe();
    let handle = h.start().await;
    h.trigger();
    phase_event(&mut events, "awaiting").await;
    let created = store.create_task(Task {
        id: Uuid::new_v4(),
        project_directory: "/project".into(),
        phase: "awaiting".into(),
        desired_phase: "running".into(),
        create_time: now(),
        update_time: now(),
        ..Default::default()
    });
    let mut hooks = h.runtime.event_router().subscribe(SubscribeOptions {
        event_types: vec!["task.created".into()],
        ..Default::default()
    });
    let task = created.await.unwrap();
    assert_eq!(
        next(&mut hooks)
            .await
            .payload
            .downcast_ref::<TaskEventPayload>()
            .unwrap()
            .task
            .id,
        task.id
    );
    assert!(Arc::ptr_eq(&h.runtime.store(), &h.runtime.store()));
    assert!(Arc::ptr_eq(
        &h.runtime.encryption(),
        &h.runtime.encryption()
    ));
    store
        .create_message(Message {
            id: Uuid::new_v4(),
            task_id: h.task,
            source: "user".into(),
            create_time: now(),
            update_time: now(),
            ..Default::default()
        })
        .await
        .unwrap();
    h.trigger();
    h.provider.entered().await;
    h.stop(handle).await;
    assert!(h.provider.cancelled.load(Ordering::SeqCst));
    assert!(h
        .runtime
        .clone()
        .run(CancellationToken::new())
        .await
        .is_err());
}
