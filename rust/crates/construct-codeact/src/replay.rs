use construct_core::tool::ToolError;
use construct_types::tools::{FunctionCall, ToolInput, ToolOutput};

pub(crate) struct Replay {
    calls: Vec<FunctionCall>,
    cursor: usize,
}

impl Replay {
    pub(crate) fn new(calls: Vec<FunctionCall>) -> Self {
        Self { calls, cursor: 0 }
    }

    pub(crate) fn take(
        &mut self,
        name: &str,
        input: &ToolInput,
    ) -> Result<Option<ToolOutput>, ToolError> {
        if self.cursor == self.calls.len() {
            return Ok(None);
        }
        let input = serde_json::to_value(input).expect("converted tool input");
        let offset = self.calls[self.cursor..].iter().position(|call| {
            call.tool_name == name
                && serde_json::to_value(&call.input).expect("stored tool input") == input
        });
        let Some(offset) = offset else {
            return Err(ToolError::custom(
                "durable replay input does not match stored calls",
                vec!["Resume the original script with its original tool inputs".into()],
            ));
        };
        self.cursor += offset + 1;
        Ok(Some(self.calls[self.cursor - 1].output.clone()))
    }
}
