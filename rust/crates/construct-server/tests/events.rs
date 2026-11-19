use construct_event::*;
use construct_proto::construct::v1::{self as pb, event::Payload};
use construct_server::conv::convert_stream_event_to_proto;
use construct_types::{entity::*, tools::*, Timestamp};
use std::sync::Arc;
use uuid::Uuid;

#[test]
fn every_entity_event_kind_converts_and_preserves_envelope() {
    let id = Uuid::from_u128(1);
    let task = Task {
        id,
        ..Default::default()
    };
    let message = Message {
        id,
        task_id: id,
        source: "user".into(),
        ..Default::default()
    };
    let agent = Agent {
        id,
        ..Default::default()
    };
    let model = Model {
        id,
        ..Default::default()
    };
    let provider = ModelProvider {
        id,
        provider_type: "anthropic".into(),
        secret: b"private".to_vec(),
        ..Default::default()
    };
    let events = vec![
        new_task_created_event(task.clone()),
        new_task_updated_event(task, "running"),
        new_task_deleted_event(id),
        new_message_created_event(message.clone()),
        new_message_updated_event(message),
        new_message_deleted_event(id, id),
        new_agent_created_event(agent.clone()),
        new_agent_updated_event(agent),
        new_agent_deleted_event(id),
        new_model_created_event(model.clone()),
        new_model_updated_event(model),
        new_model_deleted_event(id),
        new_model_provider_created_event(provider.clone()),
        new_model_provider_updated_event(provider),
        new_model_provider_deleted_event(id),
    ];
    for event in events {
        let proto = convert_stream_event_to_proto(Some(&event)).unwrap();
        assert_eq!(proto.r#type, event.r#type);
        assert_eq!(
            proto.timestamp.as_ref().unwrap().seconds,
            event.timestamp.0.timestamp()
        );
        assert_eq!(
            proto.action,
            match event.action.as_str() {
                "created" => pb::EventAction::Created,
                "updated" => pb::EventAction::Updated,
                "deleted" => pb::EventAction::Deleted,
                _ => unreachable!(),
            } as i32
        );
        let deleted = event.action == "deleted";
        let entity_id = match proto.payload.unwrap() {
            Payload::Task(t) => {
                assert_eq!(
                    t.previous_phase,
                    (event.r#type == "task.updated").then(|| "running".into())
                );
                let t = t.task.unwrap();
                assert_eq!(t.spec.is_none(), deleted);
                t.metadata.unwrap().id
            }
            Payload::Message(m) => {
                let m = m.message.unwrap();
                assert_eq!(m.spec.is_none(), deleted);
                assert_eq!(m.metadata.as_ref().unwrap().task_id, id.to_string());
                m.metadata.unwrap().id
            }
            Payload::Agent(a) => {
                let a = a.agent.unwrap();
                assert_eq!(a.spec.is_none(), deleted);
                a.metadata.unwrap().id
            }
            Payload::Model(m) => {
                let m = m.model.unwrap();
                assert_eq!(m.spec.is_none(), deleted);
                m.metadata.unwrap().id
            }
            Payload::ModelProvider(p) => {
                let p = p.model_provider.unwrap();
                assert_eq!(p.spec.is_none(), deleted);
                assert!(!serde_json::to_string(&p).unwrap().contains("private"));
                p.metadata.unwrap().id
            }
            _ => panic!("wrong payload"),
        };
        assert_eq!(entity_id, id.to_string());
    }
}

#[test]
fn chunks_and_tool_events_preserve_scope_ids_and_content() {
    let id = Uuid::from_u128(1);
    let chunk = Event {
        r#type: "message.chunk".into(),
        action: "".into(),
        timestamp: Timestamp::default(),
        task_id: Some(id),
        payload: Arc::new(MessageChunkPayload {
            task_id: id,
            message_id: id,
            chunk: "delta".into(),
            chunk_index: 7,
        }),
    };
    let proto = convert_stream_event_to_proto(Some(&chunk)).unwrap();
    assert_eq!(proto.action, pb::EventAction::Unspecified as i32);
    assert_eq!(
        proto.payload,
        Some(Payload::MessageChunk(pb::MessageChunkEvent {
            task_id: id.to_string(),
            message_id: id.to_string(),
            chunk: "delta".into(),
            chunk_index: 7
        }))
    );
    for task_id in [None, Some(id)] {
        let event = Event {
            r#type: "tool.called".into(),
            action: "".into(),
            timestamp: Timestamp::default(),
            task_id,
            payload: Arc::new(ToolCallEvent {
                id: "call-id".into(),
                tool: "read_file".into(),
                input: ToolInput {
                    read_file: Some(Box::new(ReadFileInput {
                        path: "path".into(),
                        ..Default::default()
                    })),
                    ..Default::default()
                },
            }),
        };
        let Some(Payload::ToolCalled(p)) =
            convert_stream_event_to_proto(Some(&event)).unwrap().payload
        else {
            panic!()
        };
        assert_eq!(
            p.task_id,
            task_id.map(|id| id.to_string()).unwrap_or_default()
        );
        assert_eq!(p.tool_call.as_ref().unwrap().id, "call-id");
        assert!(
            matches!(&p.tool_call.unwrap().input, Some(pb::tool_call::Input::ReadFile(i)) if i.path == "path")
        );
        let event = Event {
            r#type: "tool.result".into(),
            payload: Arc::new(ToolResultEvent {
                id: "call-id".into(),
                tool: "read_file".into(),
                output: ToolOutput {
                    read_file: Some(Box::new(ReadFileResult {
                        path: "path".into(),
                        content: "contents".into(),
                    })),
                    ..Default::default()
                },
            }),
            ..event
        };
        let Some(Payload::ToolResult(p)) =
            convert_stream_event_to_proto(Some(&event)).unwrap().payload
        else {
            panic!()
        };
        assert_eq!(
            p.task_id,
            task_id.map(|id| id.to_string()).unwrap_or_default()
        );
        assert_eq!(p.tool_result.as_ref().unwrap().id, "call-id");
        assert!(
            matches!(&p.tool_result.unwrap().result, Some(pb::tool_result::Result::ReadFile(o)) if o.content == "contents" && o.path.is_empty())
        );
    }
}

#[test]
fn absent_unknown_and_mismatched_events_fail() {
    assert_eq!(
        convert_stream_event_to_proto(None).unwrap_err().to_string(),
        "event is nil"
    );
    for kind in [
        "task.created",
        "message.created",
        "message.chunk",
        "agent.created",
        "model.created",
        "modelprovider.created",
        "tool.called",
        "tool.result",
        "unknown",
        "internal.task.trigger",
    ] {
        let event = Event {
            r#type: kind.into(),
            action: "created".into(),
            timestamp: Timestamp::default(),
            task_id: None,
            payload: Arc::new(()),
        };
        assert!(
            convert_stream_event_to_proto(Some(&event)).is_err(),
            "{kind}"
        );
    }
}
