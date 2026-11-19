//! OpenAI chat completions and the compatible xAI endpoint.
use crate::{http, resilience, ProviderOptions};
use construct_core::async_trait;
use construct_core::model::{InvokeOptions, ModelProfile, ModelProvider, OpenAIProfile};
use construct_types::provider::{
    ContentBlock, Message, MessageSource, ProviderError, ProviderErrorKind, TextBlock,
    ToolCallBlock, Usage,
};
use futures_util::StreamExt;
use serde_json::{json, Value};
use std::collections::BTreeMap;

/// Streaming chat-completions provider, also usable with compatible endpoints.
#[derive(Clone)]
pub struct OpenAICompletionProvider {
    client: reqwest::Client,
    api_key: String,
    base_url: String,
    name: &'static str,
    options: ProviderOptions,
}

impl OpenAICompletionProvider {
    /// Construct an OpenAI provider with optional endpoint and resilience overrides.
    pub fn new(
        api_key: impl Into<String>,
        options: ProviderOptions,
    ) -> Result<Self, ProviderError> {
        Self::build(
            api_key.into(),
            options,
            "openai",
            "https://api.openai.com/v1",
        )
    }

    /// Construct an xAI provider using OpenAI-compatible chat completions.
    pub fn xai(
        api_key: impl Into<String>,
        options: ProviderOptions,
    ) -> Result<Self, ProviderError> {
        // Unlike Go's api.xai.com, api.x.ai is xAI's actual API host.
        Self::build(api_key.into(), options, "xai", "https://api.x.ai/v1")
    }

    fn build(
        api_key: String,
        options: ProviderOptions,
        name: &'static str,
        url: &str,
    ) -> Result<Self, ProviderError> {
        if api_key.is_empty() {
            return Err(invalid(name, format!("{name} API key is required")));
        }
        Ok(Self {
            client: http::client_builder()
                .build()
                .map_err(|e| http::transport_error(name, e))?,
            api_key,
            base_url: if options.url.is_empty() {
                url.into()
            } else {
                options.url.trim_end_matches('/').into()
            },
            name,
            options,
        })
    }

    async fn attempt(
        &self,
        body: &Value,
        options: &InvokeOptions,
    ) -> Result<Message, ProviderError> {
        let response = http::send(
            self.name,
            self.client
                .post(format!("{}/chat/completions", self.base_url))
                .bearer_auth(&self.api_key)
                .json(body),
            &options.cancellation,
        )
        .await?;
        if !response.status().is_success() {
            let status = response.status();
            let headers = response.headers().clone();
            let detail = response
                .text()
                .await
                .map_err(|e| http::transport_error(self.name, e))?;
            return Err(http::status_error(self.name, status, &headers, detail));
        }
        let stream = http::event_stream(
            self.name,
            response.bytes_stream(),
            options.cancellation.clone(),
        );
        futures_util::pin_mut!(stream);
        let mut choices: BTreeMap<u64, Choice> = BTreeMap::new();
        let mut usage = Usage::default();
        while let Some(event) = stream.next().await {
            let event = event?;
            if event.data == "[DONE]" {
                break;
            }
            let chunk: Value = serde_json::from_str(&event.data)
                .map_err(|e| http::transport_error(self.name, e))?;
            if let Some(error) = chunk.get("error") {
                return Err(http::transport_error(self.name, error));
            }
            if let Some(u) = chunk.get("usage").filter(|u| !u.is_null()) {
                usage.input_tokens = u["prompt_tokens"].as_i64().unwrap_or(0);
                usage.output_tokens = u["completion_tokens"].as_i64().unwrap_or(0);
                usage.cache_read_tokens = u["prompt_tokens_details"]["cached_tokens"]
                    .as_i64()
                    .unwrap_or(0);
            }
            for c in chunk["choices"].as_array().into_iter().flatten() {
                let choice = choices.entry(c["index"].as_u64().unwrap_or(0)).or_default();
                let delta = &c["delta"];
                if let Some(text) = delta["content"].as_str().filter(|s| !s.is_empty()) {
                    choice.text.push_str(text);
                    if let Some(sink) = &options.text_sink {
                        sink(text);
                    }
                }
                for call in delta["tool_calls"].as_array().into_iter().flatten() {
                    let tool = choice
                        .tools
                        .entry(call["index"].as_u64().unwrap_or(0))
                        .or_default();
                    tool.id.push_str(call["id"].as_str().unwrap_or(""));
                    tool.name
                        .push_str(call["function"]["name"].as_str().unwrap_or(""));
                    tool.args
                        .push_str(call["function"]["arguments"].as_str().unwrap_or(""));
                }
            }
        }
        let mut content = Vec::new();
        for choice in choices.into_values() {
            if !choice.text.is_empty() {
                content.push(ContentBlock::Text(TextBlock { text: choice.text }));
            }
            // Unlike Go's switch, retain tool calls even when the choice also has text.
            for tool in choice.tools.into_values() {
                let args = serde_json::from_str(&tool.args)
                    .map_err(|e| http::transport_error(self.name, e))?;
                content.push(ContentBlock::ToolCall(ToolCallBlock {
                    id: tool.id,
                    tool: tool.name,
                    args: Some(args),
                }));
            }
        }
        Ok(Message {
            source: MessageSource::MODEL.into(),
            content: Some(content),
            usage,
        })
    }
}

#[derive(Default)]
struct Choice {
    text: String,
    tools: BTreeMap<u64, Tool>,
}
#[derive(Default)]
struct Tool {
    id: String,
    name: String,
    args: String,
}

fn invalid(provider: &str, detail: impl Into<String>) -> ProviderError {
    ProviderError {
        provider: provider.into(),
        kind: ProviderErrorKind::INVALID_REQUEST.into(),
        err: Some(detail.into()),
        ..Default::default()
    }
}

fn request(
    model: &str,
    system: &str,
    messages: &[Message],
    profile: &OpenAIProfile,
    options: &InvokeOptions,
) -> Value {
    // Go validates but omits the system prompt; send it so instructions take effect.
    let mut mapped = vec![json!({"role":"system", "content":system})];
    for message in messages {
        let blocks = message.content.as_deref().unwrap_or_default();
        match message.source.as_ref() {
            MessageSource::USER => {
                let parts: Vec<_> = blocks
                    .iter()
                    .filter_map(|b| {
                        if let ContentBlock::Text(t) = b {
                            Some(json!({"type":"text","text":t.text}))
                        } else {
                            None
                        }
                    })
                    .collect();
                mapped.push(json!({"role":"user","content":parts}));
            }
            MessageSource::MODEL => {
                let mut m = json!({"role":"assistant"});
                let mut tools = Vec::new();
                for b in blocks {
                    match b {
                        ContentBlock::Text(t) => m["content"] = json!(t.text),
                        ContentBlock::ToolCall(t) => tools.push(json!({"id":t.id,"type":"function","function":{"name":t.tool,"arguments":t.args.as_ref().map(|a| a.get()).unwrap_or("")}})),
                        _ => {}
                    }
                }
                if !tools.is_empty() {
                    m["tool_calls"] = json!(tools);
                }
                mapped.push(m);
            }
            MessageSource::SYSTEM => {
                for b in blocks {
                    if let ContentBlock::ToolResult(t) = b {
                        mapped.push(json!({"role":"tool","tool_call_id":t.id,"content":t.result}));
                    }
                }
            }
            _ => {}
        }
    }
    let mut body = json!({"model":model,"max_completion_tokens":profile.max_tokens,"messages":mapped,"stream":true,"stream_options":{"include_usage":true}});
    if !options.tools.is_empty() {
        body["tools"] = json!(options.tools.iter().map(|t| json!({"type":"function","function":{"name":t.name,"description":t.description,"parameters":t.input_schema}})).collect::<Vec<_>>());
        body["tool_choice"] = json!(if profile.enable_function_calling {
            "auto"
        } else {
            "none"
        });
        body["parallel_tool_calls"] = json!(profile.parallel_tool_calls);
    }
    body
}

#[async_trait]
impl ModelProvider for OpenAICompletionProvider {
    async fn invoke_model(
        &self,
        model: &str,
        system_prompt: &str,
        messages: &[Message],
        options: InvokeOptions,
    ) -> Result<Message, ProviderError> {
        for (missing, error) in [
            (model.is_empty(), "model is required"),
            (system_prompt.is_empty(), "system prompt is required"),
            (messages.is_empty(), "at least one message is required"),
        ] {
            if missing {
                return Err(invalid(self.name, error));
            }
        }
        let mut profile = options
            .profile
            .clone()
            .unwrap_or(ModelProfile::OpenAI(OpenAIProfile {
                max_tokens: 8192,
                enable_function_calling: true,
                parallel_tool_calls: true,
                ..Default::default()
            }));
        if !matches!(profile, ModelProfile::OpenAI(_)) {
            return Err(invalid(
                self.name,
                "model profile is not an **model.OpenAIModelProfile profile",
            ));
        }
        profile
            .validate()
            .map_err(|e| invalid(self.name, format!("model profile is invalid: {e}")))?;
        let ModelProfile::OpenAI(profile) = profile else {
            unreachable!()
        };
        let body = request(model, system_prompt, messages, &profile, &options);
        resilience::retry(
            &self.options.retry_config,
            &self.options.circuit_breaker,
            &options,
            || self.attempt(&body, &options),
        )
        .await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::{
        body::Bytes,
        extract::State,
        http::{HeaderMap, StatusCode},
        routing::post,
        Router,
    };
    use construct_core::model::ToolDefinition;
    use std::sync::{Arc, Mutex};

    type Captured = Arc<Mutex<Vec<(HeaderMap, Value)>>>;
    type Responses = Arc<Mutex<std::collections::VecDeque<(StatusCode, &'static str)>>>;
    type ServerState = (Captured, Responses);

    async fn server(
        responses: Vec<(StatusCode, &'static str)>,
    ) -> (String, Captured, tokio::task::JoinHandle<()>) {
        let captured = Captured::default();
        let state = (
            captured.clone(),
            Arc::new(Mutex::new(std::collections::VecDeque::from(responses))),
        );
        let app = Router::new()
            .route(
                "/chat/completions",
                post(
                    |State((captured, responses)): State<ServerState>,
                     headers: HeaderMap,
                     body: Bytes| async move {
                        captured
                            .lock()
                            .unwrap()
                            .push((headers, serde_json::from_slice(&body).unwrap()));
                        let (status, body) = responses.lock().unwrap().pop_front().unwrap();
                        (
                            status,
                            [
                                ("content-type", "text/event-stream"),
                                ("retry-after", "1.25"),
                            ],
                            body,
                        )
                    },
                ),
            )
            .with_state(state);
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let url = format!("http://{}", listener.local_addr().unwrap());
        let task = tokio::spawn(async move {
            axum::serve(listener, app).await.unwrap();
        });
        (url, captured, task)
    }

    fn conversation() -> Vec<Message> {
        serde_json::from_value(json!([
            {"source":"user","content":[{"Text":"hello"}]},
            {"source":"model","content":[{"Text":"checking"},{"id":"call_1","tool":"lookup","args":{"q":"value"}}]},
            {"source":"system","content":[{"id":"call_1","result":"found","succeeded":true}]}
        ])).unwrap()
    }

    fn options() -> InvokeOptions {
        InvokeOptions {
            tools: vec![ToolDefinition {
                name: "lookup".into(),
                description: "Look up a value".into(),
                input_schema: json!({"type":"object","properties":{"q":{"type":"string"}}}),
            }],
            ..Default::default()
        }
    }

    const STREAM: &str = concat!(
        "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hel\",\"tool_calls\":[{\"index\":0,\"id\":\"call_\",\"function\":{\"name\":\"look\",\"arguments\":\"{\\\"q\\\":\"}}]}}]}\n\n",
        "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo\",\"tool_calls\":[{\"index\":0,\"id\":\"1\",\"function\":{\"name\":\"up\",\"arguments\":\"\\\"value\\\"}\"}}]}}]}\n\n",
        "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":42,\"completion_tokens\":7,\"prompt_tokens_details\":{\"cached_tokens\":12}}}\n\n",
        "data: [DONE]\n\ndata: invalid ignored\n\n"
    );

    #[tokio::test]
    async fn golden_request_and_stream() {
        let (url, captured, server) = server(vec![(StatusCode::OK, STREAM)]).await;
        let mut config = ProviderOptions::new("openai");
        config.url = url;
        let provider = OpenAICompletionProvider::new("test-key", config).unwrap();
        let chunks = Arc::new(Mutex::new(Vec::new()));
        let observed = chunks.clone();
        let mut opts = options();
        opts.text_sink = Some(Arc::new(move |s| {
            observed.lock().unwrap().push(s.to_owned())
        }));
        let result = provider
            .invoke_model("test-model", "instructions", &conversation(), opts)
            .await
            .unwrap();
        assert_eq!(*chunks.lock().unwrap(), ["hel", "lo"]);
        let captured = captured.lock().unwrap();
        assert_eq!(captured[0].0["authorization"], "Bearer test-key");
        let mut actual = captured[0].1.clone();
        let system = actual["messages"].as_array_mut().unwrap().remove(0);
        assert_eq!(system, json!({"role":"system","content":"instructions"}));
        let golden: Value =
            serde_json::from_str(include_str!("openai_request_golden.json")).unwrap();
        assert_eq!(actual, golden);
        let blocks = result.content.unwrap();
        assert!(matches!(&blocks[0], ContentBlock::Text(t) if t.text == "hello"));
        assert!(
            matches!(&blocks[1], ContentBlock::ToolCall(t) if t.id == "call_1" && t.tool == "lookup" && t.args.as_ref().unwrap().get() == "{\"q\":\"value\"}")
        );
        assert_eq!(
            (
                result.usage.input_tokens,
                result.usage.output_tokens,
                result.usage.cache_read_tokens,
                result.usage.cache_write_tokens
            ),
            (42, 7, 12, 0)
        );
        server.abort();
    }

    #[tokio::test]
    async fn tools_and_empty_stream() {
        let (url, captured, server) = server(vec![(StatusCode::OK, "data: [DONE]\n\n"); 2]).await;
        let mut config = ProviderOptions::new("xai");
        config.url = url;
        let provider = OpenAICompletionProvider::xai("key", config).unwrap();
        let mut opts = options();
        opts.profile = Some(ModelProfile::OpenAI(OpenAIProfile::default()));
        provider
            .invoke_model("m", "s", &conversation(), opts)
            .await
            .unwrap();
        let result = provider
            .invoke_model("m", "s", &conversation(), InvokeOptions::default())
            .await
            .unwrap();
        assert!(result.content.unwrap().is_empty());
        let captured = captured.lock().unwrap();
        assert_eq!(captured[0].1["tool_choice"], "none");
        assert_eq!(captured[0].1["parallel_tool_calls"], false);
        for field in ["tools", "tool_choice", "parallel_tool_calls"] {
            assert!(captured[1].1.get(field).is_none());
        }
        assert_eq!(captured[1].1["max_completion_tokens"], 8192);
        server.abort();
    }

    #[tokio::test]
    async fn errors_retry_and_cancellation() {
        let (url, captured, server) = server(vec![
            (StatusCode::INTERNAL_SERVER_ERROR, "failure"),
            (StatusCode::OK, "data: [DONE]\n\n"),
            (StatusCode::TOO_MANY_REQUESTS, "slow down"),
            (StatusCode::OK, "data: malformed\n\n"),
        ])
        .await;
        let mut config = ProviderOptions::new("xai");
        config.url = url;
        let provider = OpenAICompletionProvider::xai("key", config).unwrap();
        provider
            .invoke_model("m", "s", &conversation(), InvokeOptions::default())
            .await
            .unwrap();
        assert_eq!(captured.lock().unwrap().len(), 2);
        let error = provider
            .invoke_model("m", "s", &conversation(), InvokeOptions::default())
            .await
            .unwrap_err();
        assert_eq!(error.provider, "xai");
        assert_eq!(error.kind.as_ref(), ProviderErrorKind::RATE_LIMIT_EXCEEDED);
        assert_eq!(error.retry_after, 1_250_000_000);
        assert_eq!(
            provider
                .invoke_model("m", "s", &conversation(), InvokeOptions::default())
                .await
                .unwrap_err()
                .kind
                .as_ref(),
            ProviderErrorKind::UNKNOWN
        );
        let opts = InvokeOptions::default();
        opts.cancellation.cancel();
        assert_eq!(
            provider
                .invoke_model("m", "s", &conversation(), opts)
                .await
                .unwrap_err()
                .kind
                .as_ref(),
            ProviderErrorKind::CANCELED
        );
        assert_eq!(captured.lock().unwrap().len(), 4);
        server.abort();
    }

    #[tokio::test]
    async fn validation_and_endpoints() {
        assert_eq!(
            OpenAICompletionProvider::new("", ProviderOptions::new("openai"))
                .err()
                .unwrap()
                .err
                .unwrap(),
            "openai API key is required"
        );
        let provider =
            OpenAICompletionProvider::new("key", ProviderOptions::new("openai")).unwrap();
        assert_eq!(provider.base_url, "https://api.openai.com/v1");
        let xai = OpenAICompletionProvider::xai("key", ProviderOptions::new("xai")).unwrap();
        assert_eq!(xai.base_url, "https://api.x.ai/v1");
        for (m, s, msgs, expected) in [
            ("", "s", conversation(), "model is required"),
            ("m", "", conversation(), "system prompt is required"),
            ("m", "s", vec![], "at least one message is required"),
        ] {
            assert_eq!(
                provider
                    .invoke_model(m, s, &msgs, InvokeOptions::default())
                    .await
                    .unwrap_err()
                    .err
                    .unwrap(),
                expected
            );
        }
        let opts = InvokeOptions {
            profile: Some(ModelProfile::Anthropic(Default::default())),
            ..Default::default()
        };
        assert!(provider
            .invoke_model("m", "s", &conversation(), opts)
            .await
            .unwrap_err()
            .err
            .unwrap()
            .contains("not an"));
        let opts = InvokeOptions {
            profile: Some(ModelProfile::OpenAI(OpenAIProfile {
                temperature: 3.0,
                ..Default::default()
            })),
            ..Default::default()
        };
        assert!(provider
            .invoke_model("m", "s", &conversation(), opts)
            .await
            .unwrap_err()
            .err
            .unwrap()
            .contains("model profile is invalid"));
    }

    #[tokio::test]
    async fn cancellation_during_stream() {
        let opts = InvokeOptions::default();
        let cancellation = opts.cancellation.clone();
        let (started_tx, started_rx) = tokio::sync::oneshot::channel();
        let started = Arc::new(Mutex::new(Some(started_tx)));
        let app = Router::new().route("/chat/completions", post(move || {
            let started = started.clone();
            async move {
                let body = axum::body::Body::from_stream(async_stream::stream! {
                    yield Ok::<_, std::io::Error>(Bytes::from_static(b"data: {\"choices\":[]}\n\n"));
                    if let Some(tx) = started.lock().unwrap().take() { let _ = tx.send(()); }
                    std::future::pending::<()>().await;
                });
                ([("content-type","text/event-stream")],body)
            }
        }));
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let mut config = ProviderOptions::new("openai");
        config.url = format!("http://{}", listener.local_addr().unwrap());
        let server = tokio::spawn(async move {
            axum::serve(listener, app).await.unwrap();
        });
        let provider = OpenAICompletionProvider::new("key", config).unwrap();
        let call =
            tokio::spawn(
                async move { provider.invoke_model("m", "s", &conversation(), opts).await },
            );
        started_rx.await.unwrap();
        cancellation.cancel();
        let result = tokio::time::timeout(std::time::Duration::from_secs(2), call)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(
            result.unwrap_err().kind.as_ref(),
            ProviderErrorKind::CANCELED
        );
        server.abort();
    }
}
