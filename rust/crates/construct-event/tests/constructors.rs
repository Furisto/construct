use construct_event::*;
use construct_types::{
    entity::{Agent, Message, Model, ModelProvider, Task},
    tools::{ReadFileInput, ReadFileResult, ToolCallEvent, ToolInput, ToolOutput, ToolResultEvent},
    Timestamp,
};
use std::{any::Any, time::SystemTime};
use uuid::Uuid;

fn check<P: Any>(
    constructor: impl FnOnce() -> Event,
    kind: &str,
    action: &str,
    scope: Option<Uuid>,
    payload_check: impl FnOnce(&P),
) {
    let before = Timestamp(SystemTime::now().into());
    let event = constructor();
    let after = Timestamp(SystemTime::now().into());
    assert_eq!(event.r#type, kind);
    assert_eq!(event.action, action);
    assert_eq!(event.task_id, scope);
    assert!((before..=after).contains(&event.timestamp));
    payload_check(event.payload.downcast_ref::<P>().unwrap());
}

#[test]
fn go_task_lifecycle() {
    let task = Task {
        id: Uuid::from_u128(1),
        phase: "running".into(),
        ..Default::default()
    };
    let scope = Some(task.id);
    for (event, kind, action, previous_phase) in [
        (
            new_task_created_event as fn(Task) -> Event,
            "task.created",
            "created",
            "",
        ),
        (
            |task| new_task_updated_event(task, "awaiting"),
            "task.updated",
            "updated",
            "awaiting",
        ),
    ] {
        check(
            || event(task.clone()),
            kind,
            action,
            scope,
            |p: &TaskEventPayload| {
                assert_eq!(p.task.id, task.id);
                assert_eq!(p.task.phase.as_ref(), "running");
                assert_eq!(p.previous_phase, previous_phase);
            },
        );
    }
    check(
        || new_task_deleted_event(task.id),
        "task.deleted",
        "deleted",
        scope,
        |p: &DeletedEntityPayload| {
            assert_eq!(p.id, task.id);
            assert_eq!(p.task_id, None);
        },
    );
}

#[test]
fn go_message_lifecycle_and_chunk() {
    let message = Message {
        id: Uuid::from_u128(1),
        task_id: Uuid::from_u128(2),
        source: "assistant".into(),
        ..Default::default()
    };
    let scope = Some(message.task_id);
    for (event, kind, action) in [
        (
            new_message_created_event as fn(Message) -> Event,
            "message.created",
            "created",
        ),
        (new_message_updated_event, "message.updated", "updated"),
    ] {
        check(
            || event(message.clone()),
            kind,
            action,
            scope,
            |p: &MessageEventPayload| {
                assert_eq!(p.message.id, message.id);
                assert_eq!(p.message.task_id, message.task_id);
                assert_eq!(p.message.source.as_ref(), "assistant");
            },
        );
    }
    check(
        || new_message_deleted_event(message.id, message.task_id),
        "message.deleted",
        "deleted",
        scope,
        |p: &DeletedEntityPayload| {
            assert_eq!(p.id, message.id);
            assert_eq!(p.task_id, scope);
        },
    );
    check(
        || new_message_chunk_event(message.task_id, message.id, "partial output", 3),
        "message.chunk",
        "created",
        scope,
        |p: &MessageChunkPayload| {
            assert_eq!(p.task_id, message.task_id);
            assert_eq!(p.message_id, message.id);
            assert_eq!(p.chunk, "partial output");
            assert_eq!(p.chunk_index, 3);
        },
    );
}

#[test]
fn go_configuration_lifecycle() {
    let id = Uuid::from_u128(1);
    for (action, agent_kind, model_kind, provider_kind, agent, model, provider) in [
        (
            "created",
            "agent.created",
            "model.created",
            "modelprovider.created",
            new_agent_created_event as fn(Agent) -> Event,
            new_model_created_event as fn(Model) -> Event,
            new_model_provider_created_event as fn(ModelProvider) -> Event,
        ),
        (
            "updated",
            "agent.updated",
            "model.updated",
            "modelprovider.updated",
            new_agent_updated_event,
            new_model_updated_event,
            new_model_provider_updated_event,
        ),
    ] {
        check(
            || {
                agent(Agent {
                    id,
                    name: "agent".into(),
                    ..Default::default()
                })
            },
            agent_kind,
            action,
            None,
            |p: &AgentEventPayload| {
                assert_eq!(p.agent.id, id);
                assert_eq!(p.agent.name, "agent");
            },
        );
        check(
            || {
                model(Model {
                    id,
                    name: "model".into(),
                    ..Default::default()
                })
            },
            model_kind,
            action,
            None,
            |p: &ModelEventPayload| {
                assert_eq!(p.model.id, id);
                assert_eq!(p.model.name, "model");
            },
        );
        check(
            || {
                provider(ModelProvider {
                    id,
                    name: "provider".into(),
                    ..Default::default()
                })
            },
            provider_kind,
            action,
            None,
            |p: &ModelProviderEventPayload| {
                assert_eq!(p.model_provider.id, id);
                assert_eq!(p.model_provider.name, "provider");
            },
        );
    }
    for (event, kind) in [
        (
            new_agent_deleted_event as fn(Uuid) -> Event,
            "agent.deleted",
        ),
        (new_model_deleted_event, "model.deleted"),
        (new_model_provider_deleted_event, "modelprovider.deleted"),
    ] {
        check(
            || event(id),
            kind,
            "deleted",
            None,
            |p: &DeletedEntityPayload| {
                assert_eq!(p.id, id);
                assert_eq!(p.task_id, None);
            },
        );
    }
}

#[test]
fn go_tool_stream_events() {
    let task = Uuid::from_u128(1);
    let call = ToolCallEvent {
        id: "call_123".into(),
        tool: "read_file".into(),
        input: ToolInput {
            read_file: Some(Box::new(ReadFileInput {
                path: "/path/to/file".into(),
                ..Default::default()
            })),
            ..Default::default()
        },
    };
    let result = ToolResultEvent {
        id: "call_123".into(),
        tool: "read_file".into(),
        output: ToolOutput {
            read_file: Some(Box::new(ReadFileResult {
                content: "file content".into(),
                ..Default::default()
            })),
            ..Default::default()
        },
    };
    check(
        || new_tool_called_event(task, call),
        "tool.called",
        "created",
        Some(task),
        |p: &ToolCallEvent| {
            assert_eq!(p.id, "call_123");
            assert_eq!(p.tool, "read_file");
            assert_eq!(p.input.read_file.as_ref().unwrap().path, "/path/to/file");
        },
    );
    check(
        || new_tool_result_event(task, result),
        "tool.result",
        "created",
        Some(task),
        |p: &ToolResultEvent| {
            assert_eq!(p.id, "call_123");
            assert_eq!(p.tool, "read_file");
            assert_eq!(p.output.read_file.as_ref().unwrap().content, "file content");
        },
    );
}

#[test]
fn go_internal_coordination_events() {
    let task = Uuid::from_u128(1);
    check(
        || new_internal_task_trigger_event(task),
        "internal.task.trigger",
        "created",
        Some(task),
        |p: &InternalTaskTriggerPayload| {
            assert_eq!(p.task_id, task);
        },
    );
    check(
        || new_internal_task_suspend_event(task),
        "internal.task.suspend",
        "created",
        Some(task),
        |p: &InternalTaskSuspendPayload| {
            assert_eq!(p.task_id, task);
        },
    );
}
