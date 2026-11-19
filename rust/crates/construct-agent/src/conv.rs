//! Conversion between persisted memory records and provider messages.
use crate::Result;
use construct_types::{entity, memory as m, provider as p, to_go_json, tools as t};

pub fn memory_source_to_model(source: &m::MessageSource) -> Result<p::MessageSource> {
    Ok(match source.as_ref() {
        m::MessageSource::ASSISTANT => "model",
        m::MessageSource::USER => "user",
        m::MessageSource::SYSTEM => "system",
        _ => return Err(format!("unknown message source: {source}").into()),
    }
    .into())
}

pub fn model_source_to_memory(source: &p::MessageSource) -> m::MessageSource {
    if source.as_ref() == p::MessageSource::MODEL {
        "assistant"
    } else {
        "user"
    }
    .into()
}

pub fn memory_message_to_model(message: &entity::Message) -> Result<p::Message> {
    let source = memory_source_to_model(&message.source)
        .map_err(|e| format!("failed to convert memory message source to model: {e}"))?;
    let content = memory_blocks_to_model(
        message
            .content
            .as_ref()
            .and_then(|c| c.blocks.as_deref())
            .unwrap_or_default(),
    )
    .map_err(|e| format!("failed to convert memory message blocks to model: {e}"))?;
    Ok(p::Message {
        source,
        content,
        ..Default::default()
    })
}

pub fn memory_blocks_to_model(blocks: &[m::MessageBlock]) -> Result<Option<Vec<p::ContentBlock>>> {
    let mut content = Vec::new();
    for block in blocks {
        content.push(match block.kind.as_ref() {
            m::MessageBlockKind::TEXT => p::ContentBlock::Text(p::TextBlock {
                text: block.payload.clone(),
            }),
            m::MessageBlockKind::TOOL_CALL => {
                let call: t::ToolCall = serde_json::from_str(&block.payload)
                    .map_err(|e| format!("failed to unmarshal tool call block: {e}"))?;
                let provider = call.provider.ok_or("tool call provider is nil")?;
                let args = call
                    .input
                    .and_then(|i| i.interpreter)
                    .map(|i| to_go_json(&i).and_then(|s| serde_json::from_slice(&s)))
                    .transpose()?;
                p::ContentBlock::ToolCall(p::ToolCallBlock {
                    id: provider.id,
                    tool: call.tool,
                    args,
                })
            }
            m::MessageBlockKind::TOOL_RESULT => {
                let result: t::ToolResult = serde_json::from_str(&block.payload)
                    .map_err(|e| format!("failed to unmarshal tool result block: {e}"))?;
                let provider = result.provider.ok_or("tool result provider is nil")?;
                let mut text = String::new();
                if let Some(output) = result.output.and_then(|o| o.interpreter) {
                    text = output.console_output;
                    for call in output.function_calls.unwrap_or_default() {
                        if let Some(command) = call.output.execute_command {
                            if !command.stderr.is_empty() {
                                text.push('\n');
                                text.push_str(&command.stderr);
                            }
                        }
                    }
                }
                p::ContentBlock::ToolResult(p::ToolResultBlock {
                    id: provider.id,
                    name: result.tool,
                    result: text,
                    succeeded: result.succeeded,
                })
            }
            _ => return Err(format!("unknown message block kind: {}", block.kind).into()),
        });
    }
    Ok((!content.is_empty()).then_some(content))
}

pub fn memory_blocks_to_agent(
    blocks: &[m::MessageBlock],
) -> Result<(Vec<t::ToolCall>, Vec<t::ToolResult>)> {
    let (mut calls, mut results) = (Vec::new(), Vec::new());
    for block in blocks {
        match block.kind.as_ref() {
            m::MessageBlockKind::TOOL_CALL => calls.push(
                serde_json::from_str(&block.payload)
                    .map_err(|e| format!("failed to unmarshal tool call block: {e}"))?,
            ),
            m::MessageBlockKind::TOOL_RESULT => results.push(
                serde_json::from_str(&block.payload)
                    .map_err(|e| format!("failed to unmarshal tool result block: {e}"))?,
            ),
            _ => {}
        }
    }
    Ok((calls, results))
}

pub fn model_message_to_memory(message: &p::Message) -> Result<entity::Message> {
    Ok(entity::Message {
        source: model_source_to_memory(&message.source),
        content: Some(
            model_blocks_to_memory(message.content.as_deref().unwrap_or_default(), "")
                .map_err(|e| format!("failed to convert model content blocks to memory: {e}"))?,
        ),
        ..Default::default()
    })
}

pub fn model_blocks_to_memory(
    blocks: &[p::ContentBlock],
    provider_kind: &str,
) -> Result<m::MessageContent> {
    let mut result = Vec::new();
    for block in blocks {
        let (kind, payload) = match block {
            p::ContentBlock::Text(text) => ("text", text.text.clone()),
            p::ContentBlock::ToolCall(call) => {
                let input = if call.tool == "code_interpreter" {
                    call.args
                        .as_ref()
                        .and_then(|args| {
                            serde_json::from_str::<Option<t::InterpreterInput>>(args.get()).ok()
                        })
                        .map(|i| {
                            Box::new(t::ToolInput {
                                interpreter: Some(Box::new(i.unwrap_or_default())),
                                ..Default::default()
                            })
                        })
                } else {
                    None
                };
                (
                    "tool_call",
                    String::from_utf8(to_go_json(&t::ToolCall {
                        provider: Some(Box::new(t::ProviderData {
                            kind: provider_kind.into(),
                            id: call.id.clone(),
                        })),
                        tool: call.tool.clone(),
                        input,
                    })?)?,
                )
            }
            p::ContentBlock::ToolResult(result) => (
                "tool_result",
                String::from_utf8(to_go_json(&t::ToolResult {
                    provider: Some(Box::new(t::ProviderData {
                        kind: provider_kind.into(),
                        id: result.id.clone(),
                    })),
                    tool: result.name.clone(),
                    succeeded: result.succeeded,
                    ..Default::default()
                })?)?,
            ),
            p::ContentBlock::Null => return Err("unknown content block type: <nil>".into()),
        };
        result.push(m::MessageBlock {
            kind: kind.into(),
            payload,
        });
    }
    Ok(m::MessageContent {
        blocks: (!result.is_empty()).then_some(result),
    })
}

pub fn model_usage_to_memory(usage: &p::Usage) -> m::MessageUsage {
    m::MessageUsage {
        input_tokens: usage.input_tokens,
        output_tokens: usage.output_tokens,
        cache_write_tokens: usage.cache_write_tokens,
        ..Default::default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn empty_and_errors() {
        assert_eq!(
            to_go_json(&model_blocks_to_memory(&[], "").unwrap()).unwrap(),
            br#"{"blocks":null}"#
        );
        assert!(memory_blocks_to_model(&[]).unwrap().is_none());
        assert!(memory_source_to_model(&"bad".into()).is_err());
        assert_eq!(model_source_to_memory(&"system".into()).as_ref(), "user");
        for (kind, payload) in [("unknown", ""), ("tool_call", "{"), ("tool_result", "{}")] {
            assert!(memory_blocks_to_model(&[m::MessageBlock {
                kind: kind.into(),
                payload: payload.into()
            }])
            .is_err());
        }
        assert!(model_blocks_to_memory(&[p::ContentBlock::Null], "").is_err());
        let usage = model_usage_to_memory(&p::Usage {
            input_tokens: 1,
            output_tokens: 2,
            cache_write_tokens: 3,
            cache_read_tokens: 4,
        });
        assert_eq!(
            (
                usage.input_tokens,
                usage.output_tokens,
                usage.cache_write_tokens,
                usage.cache_read_tokens
            ),
            (1, 2, 3, 0)
        );
    }
}
