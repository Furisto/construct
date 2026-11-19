//! Status projection shared by message events and RPC adapters.
use construct_proto::construct::v1::{MessageStatus, MessageUsage};
use construct_types::{entity::Message, memory::MessageBlockKind, Timestamp};

/// Only a completed assistant response without tool calls ends the turn.
pub fn message_status(message: &Message) -> MessageStatus {
    MessageStatus {
        usage: message.usage.as_ref().map(|usage| MessageUsage {
            input_tokens: usage.input_tokens,
            output_tokens: usage.output_tokens,
            cache_write_tokens: usage.cache_write_tokens,
            cache_read_tokens: usage.cache_read_tokens,
            cost: usage.cost,
        }),
        is_final_response: message.source.as_ref() == "assistant"
            && message
                .processed_time
                .as_ref()
                .is_some_and(|time| *time != Timestamp::default())
            && !message.content.as_ref().is_some_and(|content| {
                content
                    .blocks
                    .as_deref()
                    .unwrap_or_default()
                    .iter()
                    .any(|block| block.kind == MessageBlockKind::from("tool_call"))
            }),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use construct_types::memory::{MessageBlock, MessageContent};
    #[test]
    fn final_intermediate_empty_user_and_system_status() {
        let mut message = Message {
            source: "assistant".into(),
            ..Default::default()
        };
        assert!(!message_status(&message).is_final_response);
        message.processed_time = Some(Timestamp::default());
        assert!(!message_status(&message).is_final_response);
        message.processed_time = Some(crate::reconciler::now());
        assert!(message_status(&message).is_final_response);
        message.content = Some(MessageContent {
            blocks: Some(vec![MessageBlock {
                kind: "tool_call".into(),
                payload: String::new(),
            }]),
        });
        assert!(!message_status(&message).is_final_response);
        message.content = None;
        for source in ["system", "user"] {
            message.source = source.into();
            assert!(!message_status(&message).is_final_response);
        }
    }
}
