//! Go memory domain values; declaration order is JSON field order.
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

/// Decimal database value represented by its canonical Go decimal string.
/// No floating-point conversion, precision limit, or arithmetic is imposed.
/// Callers constructing values must supply canonical shopspring/decimal spelling
/// (for example `"1.23"`, not `"1.2300"` or `"1.23e0"`).
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(transparent)]
pub struct Decimal(
    /// Canonical, arbitrary-precision decimal string.
    pub String,
);

impl Default for Decimal {
    fn default() -> Self {
        Self("0".into())
    }
}
string_type!(
    MessageBlockKind,
    TEXT = "text",
    TOOL_CALL = "tool_call",
    TOOL_RESULT = "tool_result"
);
string_type!(
    MessageSource,
    USER = "user",
    ASSISTANT = "assistant",
    SYSTEM = "system"
);
string_type!(
    ModelCapability,
    IMAGE = "image",
    COMPUTER_USE = "computer_use",
    PROMPT_CACHE = "prompt_cache",
    EXTENDED_THINKING = "extended_thinking",
    AUDIO = "audio"
);
string_type!(
    ModelProviderType,
    ANTHROPIC = "anthropic",
    OPEN_AI = "openai",
    GEMINI = "gemini",
    XAI = "xai"
);
string_type!(
    TaskPhase,
    UNSPECIFIED = "unspecified",
    RUNNING = "running",
    AWAITING = "awaiting",
    SUSPENDED = "suspended"
);
string_type!(
    TokenType,
    SETUP_CODE = "setup_code",
    API_TOKEN = "api_token"
);

/// Go `backend/memory/schema/types/task.go` `TaskSpec`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct TaskSpec {
    /// Go `Workspace` field.
    #[serde(rename = "workspace", skip_serializing_if = "String::is_empty")]
    pub workspace: String,
}

/// Go `backend/memory/schema/types/task.go` `TaskStatus`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct TaskStatus {
    /// Go `Turns` field.
    #[serde(rename = "turns", skip_serializing_if = "crate::json::is_zero")]
    pub turns: i64,
    /// Go `ToolUses` field.
    #[serde(rename = "tool_uses", skip_serializing_if = "crate::json::is_empty")]
    pub tool_uses: Option<BTreeMap<String, i64>>,
}

/// Go `backend/memory/schema/types/task.go` `TaskUsage`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct TaskUsage {
    /// Go `InputTokens` field.
    #[serde(rename = "input_tokens", skip_serializing_if = "crate::json::is_zero")]
    pub input_tokens: i64,
    /// Go `OutputTokens` field.
    #[serde(rename = "output_tokens", skip_serializing_if = "crate::json::is_zero")]
    pub output_tokens: i64,
    /// Go `CacheWriteTokens` field.
    #[serde(
        rename = "cache_write_tokens",
        skip_serializing_if = "crate::json::is_zero"
    )]
    pub cache_write_tokens: i64,
    /// Go `CacheReadTokens` field.
    #[serde(
        rename = "cache_read_tokens",
        skip_serializing_if = "crate::json::is_zero"
    )]
    pub cache_read_tokens: i64,
    /// Go `Cost` field.
    #[serde(
        rename = "cost",
        skip_serializing_if = "crate::json::is_zero",
        with = "crate::json::finite_float"
    )]
    pub cost: f64,
}

/// Go `backend/memory/schema/types/message.go` `MessageContent`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct MessageContent {
    /// Go `Blocks` field (`None` is a nil slice).
    #[serde(rename = "blocks")]
    pub blocks: Option<Vec<MessageBlock>>,
}

/// Go `backend/memory/schema/types/message.go` `MessageBlock`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct MessageBlock {
    /// Go `Kind` field.
    #[serde(rename = "kind")]
    pub kind: MessageBlockKind,
    /// Go `Payload` field.
    #[serde(rename = "payload")]
    pub payload: String,
}

/// Go `backend/memory/schema/types/message.go` `MessageUsage`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct MessageUsage {
    /// Go `InputTokens` field.
    #[serde(rename = "input_tokens")]
    pub input_tokens: i64,
    /// Go `OutputTokens` field.
    #[serde(rename = "output_tokens")]
    pub output_tokens: i64,
    /// Go `CacheWriteTokens` field.
    #[serde(rename = "cache_write_tokens")]
    pub cache_write_tokens: i64,
    /// Go `CacheReadTokens` field.
    #[serde(rename = "cache_read_tokens")]
    pub cache_read_tokens: i64,
    /// Go `Cost` field.
    #[serde(rename = "cost", with = "crate::json::finite_float")]
    pub cost: f64,
}
