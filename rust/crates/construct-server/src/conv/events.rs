use super::*;
use construct_event as domain;
use construct_proto::construct::v1::{self as pb, event::Payload};
use construct_types::tools;

/// Convert a live domain event into a public wire envelope, checking payload types.
pub fn convert_stream_event_to_proto(
    event: Option<&domain::Event>,
) -> Result<pb::Event, ConversionError> {
    let e = event.ok_or_else(|| ConversionError("event is nil".into()))?;
    let deleted = e.payload.downcast_ref::<domain::DeletedEntityPayload>();
    let unexpected = |name: &str| ConversionError(format!("unexpected {name} payload type"));
    let payload = match e.r#type.as_str() {
        "task.created" | "task.updated" | "task.deleted" => {
            let task = if let Some(p) = e.payload.downcast_ref::<domain::TaskEventPayload>() {
                pb::TaskEvent {
                    task: Some(convert_task_to_proto(&p.task)),
                    previous_phase: (!p.previous_phase.is_empty())
                        .then(|| p.previous_phase.clone()),
                }
            } else if let Some(p) = deleted {
                pb::TaskEvent {
                    task: Some(pb::Task {
                        metadata: Some(pb::TaskMetadata {
                            id: p.id.to_string(),
                            ..Default::default()
                        }),
                        ..Default::default()
                    }),
                    ..Default::default()
                }
            } else {
                return Err(unexpected("task event"));
            };
            Payload::Task(task)
        }
        "message.created" | "message.updated" | "message.deleted" => {
            let message = if let Some(p) = e.payload.downcast_ref::<domain::MessageEventPayload>() {
                convert_memory_message_to_proto(Some(&p.message))?
            } else if let Some(p) = deleted {
                pb::Message {
                    metadata: Some(pb::MessageMetadata {
                        id: p.id.to_string(),
                        task_id: convert_uuid_ptr_to_string(p.task_id),
                        ..Default::default()
                    }),
                    ..Default::default()
                }
            } else {
                return Err(unexpected("message event"));
            };
            Payload::Message(pb::MessageEvent {
                message: Some(message),
            })
        }
        "message.chunk" => {
            let p = e
                .payload
                .downcast_ref::<domain::MessageChunkPayload>()
                .ok_or_else(|| unexpected("message chunk"))?;
            Payload::MessageChunk(pb::MessageChunkEvent {
                task_id: p.task_id.to_string(),
                message_id: p.message_id.to_string(),
                chunk: p.chunk.clone(),
                chunk_index: p.chunk_index as i32,
            })
        }
        "agent.created" | "agent.updated" | "agent.deleted" => {
            let agent = if let Some(p) = e.payload.downcast_ref::<domain::AgentEventPayload>() {
                convert_agent_to_proto(&p.agent)
            } else if let Some(p) = deleted {
                pb::Agent {
                    metadata: Some(pb::AgentMetadata {
                        id: p.id.to_string(),
                        ..Default::default()
                    }),
                    ..Default::default()
                }
            } else {
                return Err(unexpected("agent event"));
            };
            Payload::Agent(pb::AgentEvent { agent: Some(agent) })
        }
        "model.created" | "model.updated" | "model.deleted" => {
            let model = if let Some(p) = e.payload.downcast_ref::<domain::ModelEventPayload>() {
                memory_model_to_proto(&p.model)
            } else if let Some(p) = deleted {
                pb::Model {
                    metadata: Some(pb::ModelMetadata {
                        id: p.id.to_string(),
                        ..Default::default()
                    }),
                    ..Default::default()
                }
            } else {
                return Err(unexpected("model event"));
            };
            Payload::Model(pb::ModelEvent { model: Some(model) })
        }
        "modelprovider.created" | "modelprovider.updated" | "modelprovider.deleted" => {
            let model_provider = if let Some(p) = e
                .payload
                .downcast_ref::<domain::ModelProviderEventPayload>()
            {
                convert_model_provider_into_proto(Some(&p.model_provider))?
            } else if let Some(p) = deleted {
                Some(pb::ModelProvider {
                    metadata: Some(pb::ModelProviderMetadata {
                        id: p.id.to_string(),
                        ..Default::default()
                    }),
                    ..Default::default()
                })
            } else {
                return Err(unexpected("model provider event"));
            };
            Payload::ModelProvider(pb::ModelProviderEvent { model_provider })
        }
        "tool.called" => {
            let p = e
                .payload
                .downcast_ref::<tools::ToolCallEvent>()
                .ok_or_else(|| unexpected("tool called"))?;
            Payload::ToolCalled(pb::ToolCalledEvent {
                task_id: convert_uuid_ptr_to_string(e.task_id),
                tool_call: Some(convert_tool_input_to_proto(&p.id, &p.tool, Some(&p.input))),
            })
        }
        "tool.result" => {
            let p = e
                .payload
                .downcast_ref::<tools::ToolResultEvent>()
                .ok_or_else(|| unexpected("tool result"))?;
            Payload::ToolResult(pb::ToolResultEvent {
                task_id: convert_uuid_ptr_to_string(e.task_id),
                tool_result: Some(convert_tool_output_to_proto(
                    &p.id,
                    &p.tool,
                    Some(&p.output),
                )),
            })
        }
        _ => return Err(ConversionError(format!("unknown event type: {}", e.r#type))),
    };
    let action = match e.action.as_str() {
        "created" => pb::EventAction::Created,
        "updated" => pb::EventAction::Updated,
        "deleted" => pb::EventAction::Deleted,
        _ => pb::EventAction::Unspecified,
    };
    Ok(pb::Event {
        r#type: e.r#type.clone(),
        action: action as i32,
        timestamp: Some(convert_time_to_timestamp(&e.timestamp)),
        payload: Some(payload),
    })
}
