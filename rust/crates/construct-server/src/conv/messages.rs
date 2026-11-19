use super::{
    convert_string_ptr_to_uuid, convert_string_to_uuid, convert_time_to_timestamp as ts,
    convert_timestamp_to_time, convert_uuid_ptr_to_string_ptr, tools::function_call_parts,
    ConversionError,
};
use construct_proto::construct::v1::{self as pb, message_part::Data};
use construct_types::{entity as db, memory as mem, tools};

/// Convert a stored message, expanding persisted interpreter function calls like Go.
pub fn convert_memory_message_to_proto(
    m: Option<&db::Message>,
) -> Result<pb::Message, ConversionError> {
    let m = m.ok_or_else(|| ConversionError("message is nil".into()))?;
    let role = match m.source.as_ref() {
        "user" => pb::MessageRole::User,
        "assistant" => pb::MessageRole::Assistant,
        "system" => pb::MessageRole::System,
        _ => {
            return Err(ConversionError(format!(
                "unknown message source: {}",
                m.source
            )))
        }
    };
    let blocks = m
        .content
        .as_ref()
        .and_then(|c| c.blocks.as_deref())
        .unwrap_or_default();
    let mut content = Vec::new();
    for block in blocks {
        match block.kind.as_ref() {
            "text" => content.push(pb::MessagePart {
                data: Some(Data::Text(pb::message_part::Text {
                    content: block.payload.clone(),
                })),
            }),
            "tool_call" => {
                let call = serde_json::from_str::<Option<tools::ToolCall>>(&block.payload)
                    .map_err(|e| {
                        ConversionError(format!("failed to unmarshal tool call block: {e}"))
                    })?
                    .unwrap_or_default();
                if call.tool == "code_interpreter" {
                    if let Some(input) = call.input.as_ref().and_then(|i| i.interpreter.as_ref()) {
                        content.push(pb::MessagePart {
                            data: Some(Data::ToolCall(pb::ToolCall {
                                tool_name: call.tool,
                                input: Some(pb::tool_call::Input::CodeInterpreter(
                                    pb::tool_call::CodeInterpreterInput {
                                        code: input.script.clone(),
                                    },
                                )),
                                ..Default::default()
                            })),
                        });
                    }
                }
            }
            "tool_result" => {
                let result = serde_json::from_str::<Option<tools::ToolResult>>(&block.payload)
                    .map_err(|e| ConversionError(format!("failed to unmarshal tool result: {e}")))?
                    .unwrap_or_default();
                if result.tool == "code_interpreter" {
                    if let Some(output) =
                        result.output.as_ref().and_then(|o| o.interpreter.as_ref())
                    {
                        content.push(pb::MessagePart {
                            data: Some(Data::ToolResult(pb::ToolResult {
                                tool_name: result.tool.clone(),
                                result: Some(pb::tool_result::Result::CodeInterpreter(
                                    pb::tool_result::CodeInterpreterResult {
                                        output: output.console_output.clone(),
                                    },
                                )),
                                ..Default::default()
                            })),
                        });
                        for call in output.function_calls.iter().flatten() {
                            content.extend(function_call_parts(call));
                        }
                    }
                }
            }
            _ => {}
        }
    }
    Ok(pb::Message {
        metadata: Some(pb::MessageMetadata {
            id: m.id.to_string(),
            created_at: Some(ts(&m.create_time)),
            updated_at: Some(ts(&m.update_time)),
            task_id: m.task_id.to_string(),
            agent_id: m.agent_id.and_then(convert_uuid_ptr_to_string_ptr),
            model_id: m.model_id.and_then(convert_uuid_ptr_to_string_ptr),
            role: role as i32,
        }),
        spec: Some(pb::MessageSpec { content }),
        status: Some(pb::MessageStatus {
            usage: m.usage.as_ref().map(|u| pb::MessageUsage {
                input_tokens: u.input_tokens,
                output_tokens: u.output_tokens,
                cache_write_tokens: u.cache_write_tokens,
                cache_read_tokens: u.cache_read_tokens,
                cost: u.cost,
            }),
            is_final_response: role == pb::MessageRole::Assistant
                && !blocks.is_empty()
                && !blocks.iter().any(|b| b.kind.as_ref() == "tool_call"),
        }),
    })
}

/// Convert a wire message back to a database scalar record.
pub fn convert_proto_message_to_memory(
    m: Option<&pb::Message>,
) -> Result<db::Message, ConversionError> {
    let m = m.ok_or_else(|| ConversionError("message is nil".into()))?;
    let metadata = m
        .metadata
        .as_ref()
        .ok_or_else(|| ConversionError("missing related entity: message metadata".into()))?;
    let invalid =
        |field: &str, e: ConversionError| ConversionError(format!("invalid {field}: {e}"));
    let id = convert_string_to_uuid(&metadata.id).map_err(|e| invalid("message ID", e))?;
    let task_id = convert_string_to_uuid(&metadata.task_id).map_err(|e| invalid("task ID", e))?;
    let agent_id = convert_string_ptr_to_uuid(metadata.agent_id.as_deref())
        .map_err(|e| invalid("agent ID", e))?;
    let model_id = convert_string_ptr_to_uuid(metadata.model_id.as_deref())
        .map_err(|e| invalid("model ID", e))?;
    Ok(db::Message {
        id,
        task_id,
        agent_id: (!agent_id.is_nil()).then_some(agent_id),
        model_id: (!model_id.is_nil()).then_some(model_id),
        source: convert_proto_role_to_memory_role(metadata.role)
            .map_err(|e| invalid("message role", e))?,
        create_time: convert_timestamp_to_time(metadata.created_at.as_ref())?,
        update_time: convert_timestamp_to_time(metadata.updated_at.as_ref())?,
        content: convert_proto_content_to_memory(
            m.spec
                .as_ref()
                .map(|s| s.content.as_slice())
                .unwrap_or_default(),
        ),
        usage: convert_proto_usage_to_memory_usage(
            m.status.as_ref().and_then(|s| s.usage.as_ref()),
        ),
        processed_time: None,
    })
}

/// Decode writable user/assistant roles, rejecting system and unspecified like Go.
pub fn convert_proto_role_to_memory_role(role: i32) -> Result<mem::MessageSource, ConversionError> {
    match pb::MessageRole::try_from(role) {
        Ok(pb::MessageRole::User) => Ok("user".into()),
        Ok(pb::MessageRole::Assistant) => Ok("assistant".into()),
        _ => {
            let name = pb::MessageRole::try_from(role)
                .map(|r| r.as_str_name().to_owned())
                .unwrap_or_else(|_| role.to_string());
            Err(ConversionError(format!("invalid message role: {name}")))
        }
    }
}
/// Convert input text parts only, retaining nil versus allocated-empty content.
pub fn convert_proto_content_to_memory(content: &[pb::MessagePart]) -> Option<mem::MessageContent> {
    if content.is_empty() {
        return None;
    }
    Some(mem::MessageContent {
        blocks: Some(
            content
                .iter()
                .filter_map(|p| match &p.data {
                    Some(Data::Text(t)) => Some(mem::MessageBlock {
                        kind: "text".into(),
                        payload: t.content.clone(),
                    }),
                    _ => None,
                })
                .collect(),
        ),
    })
}
/// Copy usage fields, preserving an absent usage message.
pub fn convert_proto_usage_to_memory_usage(
    usage: Option<&pb::MessageUsage>,
) -> Option<mem::MessageUsage> {
    usage.map(|u| mem::MessageUsage {
        input_tokens: u.input_tokens,
        output_tokens: u.output_tokens,
        cache_write_tokens: u.cache_write_tokens,
        cache_read_tokens: u.cache_read_tokens,
        cost: u.cost,
    })
}
