//! Provider invocation contracts; transport, retries and circuit breakers are implementation-owned.
use crate::CancellationToken;
use async_trait::async_trait;
use construct_types::provider::{Message, ProviderError};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::{sync::Arc, time::Duration};

/// A native tool's metadata/schema, independent from its execution adapter.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct ToolDefinition {
    /// Provider-visible tool name.
    pub name: String,
    /// Provider-visible description.
    pub description: String,
    /// Go native.Tool.Schema(): object with properties and optional required array.
    pub input_schema: Value,
}

/// Synchronous, non-blocking text-chunk observer, called in generation order.
/// Implementations must not hold internal locks while invoking user callbacks.
pub type TextSink = Arc<dyn Fn(&str) + Send + Sync>;
/// Observer called before a provider-owned retry, with the error and next delay.
pub type RetryCallback = Arc<dyn Fn(&ProviderError, Duration) + Send + Sync>;

/// Per-call configuration. None profile means use the provider's Go defaults.
#[derive(Clone, Default)]
pub struct InvokeOptions {
    /// Tool metadata (execution is the orchestrator's responsibility).
    pub tools: Vec<ToolDefinition>,
    /// Optional ordered text stream observer.
    pub text_sink: Option<TextSink>,
    /// Optional retry notification callback; does not decide retry policy.
    pub retry_callback: Option<RetryCallback>,
    /// Concrete provider profile, checked by the selected implementation.
    pub profile: Option<ModelProfile>,
    /// Equivalent to Go's invocation context cancellation.
    pub cancellation: CancellationToken,
}

/// Object-safe equivalent of Go's model.ModelProvider.
#[async_trait]
pub trait ModelProvider: Send + Sync {
    /// Invoke a model and return its final message (including usage/tool calls).
    /// Honor cancellation; streaming and retry notifications do not replace the
    /// final result. Providers reject mismatched/invalid profiles before I/O.
    async fn invoke_model(
        &self,
        model: &str,
        system_prompt: &str,
        messages: &[Message],
        options: InvokeOptions,
    ) -> Result<Message, ProviderError>;
}

/// Go has three typed profiles. They are not stored in Ent or sent as an enum;
/// only the concrete profile structs have Go JSON tags. The enum stays Rust-only.
#[derive(Clone, Debug)]
pub enum ModelProfile {
    /// Anthropic API profile.
    Anthropic(AnthropicProfile),
    /// OpenAI completions/responses profile (also used by OpenAI-compatible backends).
    OpenAI(OpenAIProfile),
    /// Gemini profile.
    Gemini(GeminiProfile),
}

/// Invalid provider configuration.
#[derive(Clone, Debug, PartialEq, Eq, thiserror::Error)]
#[error("{0}")]
pub struct ProfileError(
    /// Validation diagnostic.
    pub String,
);

impl ModelProfile {
    /// Provider kind corresponding to Go's ModelProfile.Kind().
    pub fn kind(&self) -> construct_types::provider::ProviderKind {
        match self {
            Self::Anthropic(_) => "anthropic",
            Self::OpenAI(_) => "openai",
            Self::Gemini(_) => "gemini",
        }
        .into()
    }

    /// Validate ranges and apply precisely the defaults in Go's Validate methods.
    /// The default invocation profiles (e.g. Anthropic max_tokens=8192) are a
    /// separate provider responsibility when InvokeOptions.profile is None.
    pub fn validate(&mut self) -> Result<(), ProfileError> {
        fn range(value: f64, min: f64, max: f64, name: &str) -> Result<(), ProfileError> {
            if !value.is_finite() || value < min || value > max {
                Err(ProfileError(format!(
                    "invalid {name}: expected [{min}, {max}]"
                )))
            } else {
                Ok(())
            }
        }
        match self {
            Self::Anthropic(p) => {
                range(p.temperature, 0.0, 1.0, "temperature")?;
                if p.top_k < 0 {
                    return Err(ProfileError("top_k must be non-negative".into()));
                }
                if p.timeout == 0 {
                    p.timeout = 60_000_000_000;
                }
                if p.anthropic_version.is_empty() {
                    p.anthropic_version = "2024-01-01".into();
                }
                for (enabled, beta) in [
                    (p.enable_prompt_caching, "prompt-caching-2024-07-31"),
                    (p.enable_computer_use, "computer-use-2024-10-22"),
                ] {
                    if enabled && !p.anthropic_beta.iter().any(|v| v == beta) {
                        p.anthropic_beta.push(beta.into());
                    }
                }
            }
            Self::OpenAI(p) => {
                if p.api_url.is_empty() {
                    p.api_url = "https://api.openai.com/v1".into();
                }
                range(p.temperature, 0.0, 2.0, "temperature")?;
                range(p.frequency_penalty.into(), -2.0, 2.0, "frequency_penalty")?;
                range(p.presence_penalty.into(), -2.0, 2.0, "presence_penalty")?;
                if p.timeout == 0 {
                    p.timeout = 30_000_000_000;
                }
                if p.max_retries == 0 {
                    p.max_retries = 3;
                }
            }
            Self::Gemini(p) => {
                if let Some(t) = p.default_temperature {
                    range(t, 0.0, 1.0, "default_temperature")?;
                }
                if let Some(t) = p.default_top_p {
                    range(t.into(), 0.0, 1.0, "default_top_p")?;
                }
                if p.default_max_tokens.is_some_and(|v| v < 0)
                    || p.default_top_k.is_some_and(|v| v < 0)
                {
                    return Err(ProfileError(
                        "token/top_k limits must be non-negative".into(),
                    ));
                }
            }
        }
        Ok(())
    }
}

fn is_zero<T: Default + PartialEq>(value: &T) -> bool {
    value == &T::default()
}

/// Go `backend/model/anthropic_models.go` `AnthropicModelProfile`; duration fields are signed nanoseconds.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct AnthropicProfile {
    /// Go `AnthropicVersion` field.
    #[serde(rename = "anthropic_version", skip_serializing_if = "String::is_empty")]
    pub anthropic_version: String,
    /// Go `AnthropicBeta` field.
    #[serde(rename = "anthropic_beta", skip_serializing_if = "Vec::is_empty")]
    pub anthropic_beta: Vec<String>,
    /// Go `Timeout` field.
    #[serde(rename = "timeout", skip_serializing_if = "is_zero")]
    pub timeout: i64,
    /// Go `MaxRetries` field.
    #[serde(rename = "max_retries", skip_serializing_if = "is_zero")]
    pub max_retries: i64,
    /// Go `Temperature` field.
    #[serde(rename = "temperature", skip_serializing_if = "is_zero")]
    pub temperature: f64,
    /// Go `MaxTokens` field.
    #[serde(rename = "max_tokens", skip_serializing_if = "is_zero")]
    pub max_tokens: i64,
    /// Go `DefaultTopP` field.
    #[serde(rename = "default_top_p", skip_serializing_if = "is_zero")]
    pub default_top_p: f32,
    /// Go `TopK` field.
    #[serde(rename = "top_k", skip_serializing_if = "is_zero")]
    pub top_k: i64,
    /// Go `StopSequences` field.
    #[serde(rename = "stop_sequences", skip_serializing_if = "Vec::is_empty")]
    pub stop_sequences: Vec<String>,
    /// Go `EnablePromptCaching` field.
    #[serde(rename = "enable_prompt_caching", skip_serializing_if = "is_zero")]
    pub enable_prompt_caching: bool,
    /// Go `EnableThinkingMode` field.
    #[serde(rename = "enable_thinking_mode", skip_serializing_if = "is_zero")]
    pub enable_thinking_mode: bool,
    /// Go `EnableAnalysisMode` field.
    #[serde(rename = "enable_analysis_mode", skip_serializing_if = "is_zero")]
    pub enable_analysis_mode: bool,
    /// Go `EnableComputerUse` field.
    #[serde(rename = "enable_computer_use", skip_serializing_if = "is_zero")]
    pub enable_computer_use: bool,
}

/// Go `backend/model/openai_responses.go` `OpenAIModelProfile`; duration fields are signed nanoseconds.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct OpenAIProfile {
    /// Go `APIURL` field.
    #[serde(rename = "api_url", skip_serializing_if = "String::is_empty")]
    pub api_url: String,
    /// Go `Organization` field.
    #[serde(rename = "organization", skip_serializing_if = "String::is_empty")]
    pub organization: String,
    /// Go `APIVersion` field.
    #[serde(rename = "api_version", skip_serializing_if = "String::is_empty")]
    pub api_version: String,
    /// Go `Timeout` field.
    #[serde(rename = "timeout", skip_serializing_if = "is_zero")]
    pub timeout: i64,
    /// Go `MaxRetries` field.
    #[serde(rename = "max_retries", skip_serializing_if = "is_zero")]
    pub max_retries: i64,
    /// Go `Temperature` field.
    #[serde(rename = "temperature", skip_serializing_if = "is_zero")]
    pub temperature: f64,
    /// Go `MaxTokens` field.
    #[serde(rename = "max_tokens", skip_serializing_if = "is_zero")]
    pub max_tokens: i64,
    /// Go `TopP` field.
    #[serde(rename = "top_p", skip_serializing_if = "is_zero")]
    pub top_p: f32,
    /// Go `FrequencyPenalty` field.
    #[serde(rename = "frequency_penalty", skip_serializing_if = "is_zero")]
    pub frequency_penalty: f32,
    /// Go `PresencePenalty` field.
    #[serde(rename = "presence_penalty", skip_serializing_if = "is_zero")]
    pub presence_penalty: f32,
    /// Go `EnableJSONMode` field.
    #[serde(rename = "enable_json_mode", skip_serializing_if = "is_zero")]
    pub enable_json_mode: bool,
    /// Go `EnableFunctionCalling` field.
    #[serde(rename = "enable_function_calling", skip_serializing_if = "is_zero")]
    pub enable_function_calling: bool,
    /// Go `ParallelToolCalls` field.
    #[serde(rename = "parallel_tool_calls", skip_serializing_if = "is_zero")]
    pub parallel_tool_calls: bool,
    /// Go `EnableVision` field.
    #[serde(rename = "enable_vision", skip_serializing_if = "is_zero")]
    pub enable_vision: bool,
    /// Go `StopSequences` field.
    #[serde(rename = "stop_sequences", skip_serializing_if = "Vec::is_empty")]
    pub stop_sequences: Vec<String>,
    /// Go `RequestsPerMinute` field.
    #[serde(rename = "requests_per_minute", skip_serializing_if = "is_zero")]
    pub requests_per_minute: i64,
    /// Go `TokensPerMinute` field.
    #[serde(rename = "tokens_per_minute", skip_serializing_if = "is_zero")]
    pub tokens_per_minute: i64,
}

/// Go `backend/model/gemini.go` `GeminiModelProfile`; duration fields are signed nanoseconds.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct GeminiProfile {
    /// Go `APIKey` field.
    #[serde(rename = "api_key", skip_serializing_if = "String::is_empty")]
    pub api_key: String,
    /// Go `BaseURL` field.
    #[serde(rename = "base_url", skip_serializing_if = "String::is_empty")]
    pub base_url: String,
    /// Go `MaxRetries` field.
    #[serde(rename = "max_retries", skip_serializing_if = "is_zero")]
    pub max_retries: i64,
    /// Go `DefaultTemperature` field.
    #[serde(
        rename = "default_temperature",
        skip_serializing_if = "Option::is_none"
    )]
    pub default_temperature: Option<f64>,
    /// Go `DefaultMaxTokens` field.
    #[serde(rename = "default_max_tokens", skip_serializing_if = "Option::is_none")]
    pub default_max_tokens: Option<i32>,
    /// Go `DefaultTopP` field.
    #[serde(rename = "default_top_p", skip_serializing_if = "Option::is_none")]
    pub default_top_p: Option<f32>,
    /// Go `DefaultTopK` field.
    #[serde(rename = "default_top_k", skip_serializing_if = "Option::is_none")]
    pub default_top_k: Option<i32>,
}
