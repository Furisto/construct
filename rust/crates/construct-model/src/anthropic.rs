//! Anthropic streaming Messages provider, matching the Go adapter and SDK defaults.
use crate::{http, resilience, ProviderOptions};
use construct_core::{
    async_trait,
    model::{AnthropicProfile, InvokeOptions, ModelProfile, ModelProvider},
};
use construct_types::provider::{
    ContentBlock, Message, MessageSource, ProviderError, ProviderErrorKind, TextBlock,
    ToolCallBlock, Usage,
};
use futures_util::StreamExt;
use serde::Serialize;
use serde_json::{
    json,
    value::{to_raw_value, RawValue},
    Value,
};
use std::{collections::BTreeMap, time::Duration};

/// Anthropic provider with shared connection pool and circuit breaker.
pub struct AnthropicProvider {
    client: reqwest::Client,
    api_key: String,
    options: ProviderOptions,
}

fn invalid(detail: impl Into<String>) -> ProviderError {
    ProviderError {
        provider: "anthropic".into(),
        kind: ProviderErrorKind::INVALID_REQUEST.into(),
        err: Some(detail.into()),
        ..Default::default()
    }
}
fn unknown(error: impl std::fmt::Display) -> ProviderError {
    http::transport_error("anthropic", error)
}

impl AnthropicProvider {
    /// Create a provider. An empty key is rejected before any I/O.
    pub fn new(
        api_key: impl Into<String>,
        options: ProviderOptions,
    ) -> Result<Self, ProviderError> {
        let api_key = api_key.into();
        if api_key.is_empty() {
            return Err(invalid("anthropic API key is required"));
        }
        Ok(Self {
            client: http::client_builder().build().map_err(unknown)?,
            api_key,
            options,
        })
    }

    async fn attempt(&self, body: &str, options: &InvokeOptions) -> Result<Message, ProviderError> {
        let base = if self.options.url.is_empty() {
            "https://api.anthropic.com"
        } else {
            &self.options.url
        };
        let url = format!("{}/v1/messages", base.trim_end_matches('/'));
        // The Go SDK retries HTTP/connection failures twice before the outer policy.
        let mut count = 0;
        let response = loop {
            let request = self
                .client
                .post(&url)
                .header("x-api-key", &self.api_key)
                .header("anthropic-version", "2023-06-01")
                .header("content-type", "application/json")
                .header("accept", "application/json")
                .header("x-stainless-retry-count", count.to_string())
                .header("x-stainless-timeout", "0")
                .body(body.to_owned());
            let response = http::send("anthropic", request, &options.cancellation).await;
            let should_retry = match &response {
                Ok(res) => match res
                    .headers()
                    .get("x-should-retry")
                    .and_then(|v| v.to_str().ok())
                {
                    Some("true") => true,
                    Some("false") => false,
                    _ => matches!(res.status().as_u16(), 408 | 409 | 429 | 500..=599),
                },
                Err(error) => error.kind.as_ref() != ProviderErrorKind::CANCELED,
            };
            if count == 2 || !should_retry {
                break response?;
            }
            let delay = sdk_delay(response.as_ref().ok(), count);
            drop(response);
            resilience::cancelable("anthropic", &options.cancellation, async {
                tokio::time::sleep(delay).await;
                Ok(())
            })
            .await?;
            count += 1;
        };
        if !response.status().is_success() {
            let status = response.status();
            let headers = response.headers().clone();
            let detail = resilience::cancelable("anthropic", &options.cancellation, async {
                response.text().await.map_err(unknown)
            })
            .await?;
            return Err(http::status_error("anthropic", status, &headers, detail));
        }
        let events = http::event_stream(
            "anthropic",
            response.bytes_stream(),
            options.cancellation.clone(),
        );
        futures_util::pin_mut!(events);
        let mut blocks: Vec<AccumulatedBlock> = Vec::new();
        let mut usage = Usage::default();
        while let Some(event) = events.next().await {
            let event = event?;
            if event.event == "error" {
                return Err(unknown(event.data));
            }
            let value: Value = serde_json::from_str(&event.data).map_err(unknown)?;
            match value["type"].as_str().unwrap_or("") {
                "message_start" => {
                    usage = read_usage(&value["message"]["usage"]);
                    blocks.clear();
                    if let Some(message) = raw_field(&event.data, "message") {
                        if let Some(content) = raw_field(message.get(), "content") {
                            let content: Vec<Box<RawValue>> =
                                serde_json::from_str(content.get()).map_err(unknown)?;
                            for block in content {
                                blocks.push(AccumulatedBlock::from_raw(block.get())?);
                            }
                        }
                    }
                }
                "content_block_start" => {
                    if let Some(block) = raw_field(&event.data, "content_block") {
                        blocks.push(AccumulatedBlock::from_raw(block.get())?);
                    }
                }
                "content_block_delta" => {
                    let delta = &value["delta"];
                    if let Some(block) = blocks.last_mut() {
                        match delta["type"].as_str().unwrap_or("") {
                            "text_delta" => {
                                let text = delta["text"].as_str().unwrap_or("");
                                block.text.push_str(text);
                                if !text.is_empty() {
                                    if let Some(sink) = &options.text_sink {
                                        sink(text);
                                    }
                                }
                            }
                            "input_json_delta" => {
                                let partial = delta["partial_json"].as_str().unwrap_or("");
                                if !partial.is_empty() {
                                    if block.input == "{}" {
                                        block.input.clear();
                                    }
                                    block.input.push_str(partial);
                                }
                            }
                            _ => {}
                        }
                    }
                }
                "message_delta" => {
                    usage.output_tokens = value["usage"]["output_tokens"].as_i64().unwrap_or(0)
                }
                _ => {}
            }
        }
        let content = blocks
            .into_iter()
            .map(|block| {
                Ok(match block.kind.as_str() {
                    "text" => ContentBlock::Text(TextBlock { text: block.text }),
                    "tool_use" => ContentBlock::ToolCall(ToolCallBlock {
                        id: block.id,
                        tool: block.name,
                        args: Some(RawValue::from_string(block.input).map_err(unknown)?),
                    }),
                    _ => ContentBlock::Null,
                })
            })
            .collect::<Result<Vec<_>, ProviderError>>()?;
        Ok(Message {
            source: MessageSource::MODEL.into(),
            content: Some(content),
            usage,
        })
    }
}

fn sdk_delay(response: Option<&reqwest::Response>, count: u32) -> Duration {
    if let Some(response) = response {
        for (header, scale) in [("retry-after-ms", 0.001), ("retry-after", 1.0)] {
            if let Some(value) = response.headers().get(header).and_then(|v| v.to_str().ok()) {
                if let Ok(number) = value.parse::<f64>() {
                    let seconds = number * scale;
                    if (0.0..60.0).contains(&seconds) {
                        return Duration::from_secs_f64(seconds);
                    }
                } else if header == "retry-after" {
                    if let Ok(date) = httpdate::parse_http_date(value) {
                        if let Ok(delay) = date.duration_since(std::time::SystemTime::now()) {
                            if delay < Duration::from_secs(60) {
                                return delay;
                            }
                        }
                    }
                }
            }
        }
    }
    Duration::from_secs_f64(0.5 * 2_f64.powi(count as i32) * rand::random_range(0.75..=1.0))
}

struct AccumulatedBlock {
    kind: String,
    text: String,
    id: String,
    name: String,
    input: String,
}
impl AccumulatedBlock {
    fn from_raw(raw: &str) -> Result<Self, ProviderError> {
        let value: Value = serde_json::from_str(raw).map_err(unknown)?;
        Ok(Self {
            kind: value["type"].as_str().unwrap_or("").into(),
            text: value["text"].as_str().unwrap_or("").into(),
            id: value["id"].as_str().unwrap_or("").into(),
            name: value["name"].as_str().unwrap_or("").into(),
            input: raw_field(raw, "input")
                .map(|input| input.get().to_owned())
                .unwrap_or_else(|| "null".into()),
        })
    }
}
fn raw_field(raw: &str, field: &str) -> Option<Box<RawValue>> {
    serde_json::from_str::<BTreeMap<String, Box<RawValue>>>(raw)
        .ok()?
        .remove(field)
}
fn read_usage(value: &Value) -> Usage {
    Usage {
        input_tokens: value["input_tokens"].as_i64().unwrap_or(0),
        output_tokens: value["output_tokens"].as_i64().unwrap_or(0),
        cache_write_tokens: value["cache_creation_input_tokens"].as_i64().unwrap_or(0),
        cache_read_tokens: value["cache_read_input_tokens"].as_i64().unwrap_or(0),
    }
}

#[derive(Serialize)]
struct RequestMessage {
    role: String,
    content: Vec<BTreeMap<String, Box<RawValue>>>,
}
#[derive(Serialize)]
struct Request {
    model: String,
    max_tokens: i64,
    system: Value,
    messages: Vec<RequestMessage>,
    stream: bool,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    tools: Vec<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    tool_choice: Option<Value>,
}

fn request_body(
    model: &str,
    system: &str,
    messages: &[Message],
    options: &InvokeOptions,
    profile: &AnthropicProfile,
) -> Result<String, ProviderError> {
    let cached = messages
        .iter()
        .enumerate()
        .rev()
        .filter(|(_, m)| m.source.as_ref() == MessageSource::USER)
        .take(2)
        .map(|(i, _)| i)
        .collect::<Vec<_>>();
    let messages = messages.iter().enumerate().map(|(i, message)| {
        let content = message.content.as_deref().unwrap_or_default();
        let blocks = content.iter().enumerate().map(|(j, block)| {
            let cache = cached.contains(&i) && j + 1 == content.len();
            let mut value = match block {
                ContentBlock::Text(text) => json!({"type":"text","text":text.text}),
                ContentBlock::ToolCall(tool) => json!({"type":"tool_use","id":tool.id,"name":tool.tool}),
                ContentBlock::ToolResult(result) => json!({"type":"tool_result","tool_use_id":result.id,"is_error":!result.succeeded,"content":[{"type":"text","text":result.result}]}),
                ContentBlock::Null => json!({}),
            };
            if cache && matches!(block, ContentBlock::Text(_) | ContentBlock::ToolResult(_)) { value["cache_control"] = json!({"type":"ephemeral"}); }
            let mut fields: BTreeMap<String, Box<RawValue>> = value.as_object().unwrap().iter()
                .map(|(key, value)| Ok((key.clone(), to_raw_value(value).map_err(unknown)?)))
                .collect::<Result<_, ProviderError>>()?;
            if let ContentBlock::ToolCall(tool) = block { fields.insert("input".into(), tool.args.clone().unwrap_or(to_raw_value(&Value::Null).map_err(unknown)?)); }
            Ok(fields)
        }).collect::<Result<Vec<_>, ProviderError>>()?;
        let role = match message.source.as_ref() {
            MessageSource::MODEL => "assistant",
            MessageSource::USER | MessageSource::SYSTEM => "user",
            _ => "",
        };
        Ok(RequestMessage { role: role.into(), content: blocks })
    }).collect::<Result<Vec<_>, ProviderError>>()?;
    let tools = options
        .tools
        .iter()
        .enumerate()
        .map(|(i, tool)| {
            let mut schema = tool.input_schema.clone();
            schema["type"] = json!("object");
            let mut value =
                json!({"name":tool.name,"description":tool.description,"input_schema":schema});
            if i + 1 == options.tools.len() {
                value["cache_control"] = json!({"type":"ephemeral"});
            }
            value
        })
        .collect::<Vec<_>>();
    serde_json::to_string(&Request {
        model: model.into(),
        max_tokens: profile.max_tokens,
        system: json!([{"type":"text","text":system,"cache_control":{"type":"ephemeral"}}]),
        messages,
        stream: true,
        tool_choice: (!tools.is_empty()).then(|| json!({"type":"auto"})),
        tools,
    })
    .map_err(unknown)
}

#[async_trait]
impl ModelProvider for AnthropicProvider {
    async fn invoke_model(
        &self,
        model: &str,
        system_prompt: &str,
        messages: &[Message],
        options: InvokeOptions,
    ) -> Result<Message, ProviderError> {
        if model.is_empty() {
            return Err(invalid("model is required"));
        }
        if system_prompt.is_empty() {
            return Err(invalid("system prompt is required"));
        }
        if messages.is_empty() {
            return Err(invalid("at least one message is required"));
        }
        let mut profile =
            options
                .profile
                .clone()
                .unwrap_or(ModelProfile::Anthropic(AnthropicProfile {
                    max_tokens: 8192,
                    ..Default::default()
                }));
        if !matches!(profile, ModelProfile::Anthropic(_)) {
            return Err(invalid("model profile is not an Anthropic profile"));
        }
        profile
            .validate()
            .map_err(|error| invalid(error.to_string()))?;
        let ModelProfile::Anthropic(profile) = profile else {
            unreachable!()
        };
        let body = request_body(model, system_prompt, messages, &options, &profile)?;
        resilience::retry(
            &self.options.retry_config,
            &self.options.circuit_breaker,
            &options,
            || self.attempt(&body, &options),
        )
        .await
    }
}
