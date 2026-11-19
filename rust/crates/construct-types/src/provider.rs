//! Go provider domain values; declaration order is JSON field order.
use serde::{Deserialize, Serialize};
use serde_json::value::RawValue;
use uuid::Uuid;

/// Provider failure with Go's public retry policy. Error payloads are Rust-only:
/// Go's arbitrary `error` interface has no stable JSON representation.
#[derive(Clone, Debug, Default)]
pub struct ProviderError {
    /// Provider name.
    pub provider: String,
    /// Signed Go duration in nanoseconds.
    pub retry_after: i64,
    /// Underlying error message, if any.
    pub err: Option<String>,
    /// Failure classification.
    pub kind: ProviderErrorKind,
}

impl ProviderError {
    /// Match Go's public Retryable method (internal/timeout are not retried).
    /// Returns retry eligibility and signed delay in nanoseconds.
    pub fn retryable(&self) -> (bool, i64) {
        match self.kind.as_ref() {
            ProviderErrorKind::RATE_LIMIT_EXCEEDED => (true, self.retry_after),
            ProviderErrorKind::OVERLOADED => (true, 20_000_000_000),
            _ => (false, 0),
        }
    }

    /// User-facing message matching Go's ProviderError.Message.
    pub fn message(&self) -> String {
        match self.kind.as_ref() {
            ProviderErrorKind::INVALID_REQUEST => "Invalid request format or content",
            ProviderErrorKind::RATE_LIMIT_EXCEEDED if self.retry_after > 0 => {
                return format!(
                    "Rate limit exceeded, retry after {}",
                    go_duration(self.retry_after)
                );
            }
            ProviderErrorKind::RATE_LIMIT_EXCEEDED => "Rate limit exceeded",
            ProviderErrorKind::OVERLOADED => "API temporarily overloaded",
            ProviderErrorKind::INTERNAL => "Internal server error",
            ProviderErrorKind::TIMEOUT => "Request timeout",
            ProviderErrorKind::CANCELED => "Request canceled",
            _ => "Unknown error",
        }
        .into()
    }
}

fn go_duration(nanos: i64) -> String {
    if nanos < 1_000_000_000 {
        let (unit, divisor) = if nanos < 1_000 {
            ("ns", 1.0)
        } else if nanos < 1_000_000 {
            ("µs", 1_000.0)
        } else {
            ("ms", 1_000_000.0)
        };
        return format!("{}{unit}", nanos as f64 / divisor);
    }
    let seconds = nanos / 1_000_000_000;
    let remainder = nanos % 1_000_000_000;
    let fraction = if remainder == 0 {
        String::new()
    } else {
        format!(".{remainder:09}").trim_end_matches('0').to_owned()
    };
    let suffix = format!("{}{fraction}s", seconds % 60);
    if seconds >= 3600 {
        format!("{}h{}m{suffix}", seconds / 3600, seconds / 60 % 60)
    } else if seconds >= 60 {
        format!("{}m{suffix}", seconds / 60)
    } else {
        suffix
    }
}

impl std::fmt::Display for ProviderError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}: {}", self.provider, self.message())?;
        if let Some(err) = &self.err {
            write!(f, ": {err}")?;
        }
        Ok(())
    }
}
impl std::error::Error for ProviderError {}
string_type!(
    MessageSource,
    USER = "user",
    MODEL = "model",
    SYSTEM = "system"
);
string_type!(
    ContentBlockType,
    TEXT = "text",
    TOOL_REQUEST = "tool_request",
    TOOL_RESULT = "tool_result"
);
string_type!(
    ProviderErrorKind,
    INVALID_REQUEST = "invalid_request",
    RATE_LIMIT_EXCEEDED = "rate_limit_exceeded",
    OVERLOADED = "overloaded",
    INTERNAL = "internal",
    TIMEOUT = "timeout",
    CANCELED = "canceled",
    UNKNOWN = "unknown"
);
string_type!(
    Capability,
    IMAGE = "image",
    AUDIO = "audio",
    COMPUTER_USE = "computer_use",
    PROMPT_CACHE = "prompt_cache",
    EXTENDED_THINKING = "extended_thinking"
);
string_type!(
    ProviderKind,
    ANTHROPIC = "anthropic",
    OPEN_AI = "openai",
    DEEP_SEEK = "deepseek",
    GEMINI = "gemini",
    XAI = "xai",
    BEDROCK = "bedrock"
);

/// Go `backend/model/provider.go` `Message`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct Message {
    /// Go `Source` field.
    #[serde(rename = "source")]
    pub source: MessageSource,
    /// Go `Content` field (`None` is a nil slice).
    #[serde(rename = "content")]
    pub content: Option<Vec<ContentBlock>>,
    /// Go `Usage` field.
    #[serde(rename = "usage")]
    pub usage: Usage,
}

/// Go `backend/model/provider.go` `TextBlock`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub struct TextBlock {
    /// Go `Text` field.
    #[serde(rename = "Text")]
    pub text: String,
}

/// Untagged Go interface payload: no synthetic type discriminator is serialized.
#[derive(Clone, Debug, Default, Serialize)]
#[serde(untagged)]
pub enum ContentBlock {
    /// Provider tool invocation.
    ToolCall(ToolCallBlock),
    /// Provider tool result.
    ToolResult(ToolResultBlock),
    /// Plain text (Go spells its untagged field `Text`).
    Text(TextBlock),
    /// Nil Go interface or nil concrete content block pointer.
    #[default]
    Null,
}

impl<'de> Deserialize<'de> for ContentBlock {
    fn deserialize<D: serde::Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        let raw = Box::<RawValue>::deserialize(deserializer)?;
        let text = raw.get();
        if text == "null" {
            return Ok(Self::Null);
        }
        let keys: std::collections::BTreeMap<String, serde::de::IgnoredAny> =
            serde_json::from_str(text).map_err(serde::de::Error::custom)?;
        // Deserialize directly from raw JSON: serde's untagged buffering loses
        // RawValue support and would reorder/normalize tool arguments.
        if keys.contains_key("Text") {
            serde_json::from_str(text)
                .map(Self::Text)
                .map_err(serde::de::Error::custom)
        } else if keys.contains_key("tool") {
            serde_json::from_str(text)
                .map(Self::ToolCall)
                .map_err(serde::de::Error::custom)
        } else if keys.contains_key("succeeded") {
            serde_json::from_str(text)
                .map(Self::ToolResult)
                .map_err(serde::de::Error::custom)
        } else {
            Err(serde::de::Error::custom(
                "unrecognized provider content block",
            ))
        }
    }
}

impl ContentBlock {
    /// Return the discriminator supplied by Go's `ContentBlock.Type()` method.
    pub fn block_type(&self) -> Option<ContentBlockType> {
        Some(
            match self {
                Self::Text(_) => ContentBlockType::TEXT,
                Self::ToolCall(_) => ContentBlockType::TOOL_REQUEST,
                Self::ToolResult(_) => ContentBlockType::TOOL_RESULT,
                Self::Null => return None,
            }
            .into(),
        )
    }
}

/// Go `backend/model/provider.go` `ToolCallBlock`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub struct ToolCallBlock {
    /// Go `ID` field.
    #[serde(rename = "id")]
    pub id: String,
    /// Go `Tool` field.
    #[serde(rename = "tool")]
    pub tool: String,
    /// Go `Args` field.
    #[serde(rename = "args")]
    pub args: Option<Box<RawValue>>,
}

/// Go `backend/model/provider.go` `ToolResultBlock`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub struct ToolResultBlock {
    /// Go `ID` field.
    #[serde(rename = "id")]
    pub id: String,
    /// Go `Name` field.
    #[serde(rename = "name")]
    pub name: String,
    /// Go `Result` field.
    #[serde(rename = "result")]
    pub result: String,
    /// Go `Succeeded` field.
    #[serde(rename = "succeeded")]
    pub succeeded: bool,
}

/// Go `backend/model/provider.go` `Usage`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct Usage {
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
}

/// Go `backend/model/model.go` `Model`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct Model {
    /// Go `ID` field.
    #[serde(rename = "ID")]
    pub id: Uuid,
    /// Go `Provider` field.
    #[serde(rename = "Provider")]
    pub provider: ProviderKind,
    /// Go `Name` field.
    #[serde(rename = "Name")]
    pub name: String,
    /// Go `Capabilities` field (`None` is a nil slice).
    #[serde(rename = "Capabilities")]
    pub capabilities: Option<Vec<Capability>>,
    /// Go `ContextWindow` field.
    #[serde(rename = "ContextWindow")]
    pub context_window: i64,
    /// Go `Pricing` field.
    #[serde(rename = "Pricing")]
    pub pricing: ModelPricing,
}

/// Go `backend/model/model.go` `ModelPricing`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct ModelPricing {
    /// Go `Input` field.
    #[serde(rename = "Input", with = "crate::json::finite_float")]
    pub input: f64,
    /// Go `Output` field.
    #[serde(rename = "Output", with = "crate::json::finite_float")]
    pub output: f64,
    /// Go `CacheWrite` field.
    #[serde(rename = "CacheWrite", with = "crate::json::finite_float")]
    pub cache_write: f64,
    /// Go `CacheRead` field.
    #[serde(rename = "CacheRead", with = "crate::json::finite_float")]
    pub cache_read: f64,
}
