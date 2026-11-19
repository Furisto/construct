//! Google Gemini's Go-compatible streaming chat provider.
use crate::{http, resilience, ProviderOptions};
use construct_core::model::{InvokeOptions, ModelProfile, ModelProvider, ToolDefinition};
use construct_types::provider::{
    ContentBlock, Message, ProviderError, TextBlock, ToolCallBlock, Usage,
};
use futures_util::StreamExt;
use serde_json::{json, Map, Value};

/// Gemini client. Like Go, profile API keys, URLs, retry counts and generation
/// defaults are validated but not applied to requests.
#[derive(Clone)]
pub struct GeminiProvider {
    client: reqwest::Client,
    api_key: String,
    base_url: String,
}

fn error(detail: impl std::fmt::Display) -> ProviderError {
    http::transport_error("gemini", detail)
}

impl GeminiProvider {
    /// Construct a provider using Google's public v1beta endpoint.
    pub fn new(api_key: impl Into<String>) -> Result<Self, ProviderError> {
        Self::with_options(api_key, ProviderOptions::new("gemini"))
    }

    /// Construct a provider with a transport base URL override. Gemini in Go
    /// does not use the shared retry policy or circuit breaker.
    pub fn with_options(
        api_key: impl Into<String>,
        options: ProviderOptions,
    ) -> Result<Self, ProviderError> {
        let api_key = api_key.into();
        if api_key.is_empty() {
            return Err(error("gemini API key is required"));
        }
        Ok(Self {
            client: http::client_builder().build().map_err(error)?,
            api_key,
            base_url: if options.url.is_empty() {
                "https://generativelanguage.googleapis.com".into()
            } else {
                options.url
            },
        })
    }
}

#[construct_core::async_trait]
impl ModelProvider for GeminiProvider {
    async fn invoke_model(
        &self,
        model: &str,
        system_prompt: &str,
        messages: &[Message],
        options: InvokeOptions,
    ) -> Result<Message, ProviderError> {
        for (invalid, detail) in [
            (model.is_empty(), "model is required"),
            (system_prompt.is_empty(), "system prompt is required"),
            (messages.is_empty(), "at least one message is required"),
        ] {
            if invalid {
                return Err(error(detail));
            }
        }
        if let Some(mut profile) = options.profile.clone() {
            if !matches!(profile, ModelProfile::Gemini(_)) {
                return Err(error("model profile is not a GeminiModelProfile profile"));
            }
            profile.validate().map_err(error)?;
        }
        let body = request_body(system_prompt, messages, &options.tools)?;
        let model = if model.starts_with("models/") || model.starts_with("tunedModels/") {
            model.to_owned()
        } else {
            format!("models/{model}")
        };
        let url = format!(
            "{}/v1beta/{model}:streamGenerateContent?alt=sse",
            self.base_url.trim_end_matches('/')
        );
        let response = http::send(
            "gemini",
            self.client
                .post(url)
                .header("x-goog-api-key", &self.api_key)
                .json(&body),
            &options.cancellation,
        )
        .await?;
        if !response.status().is_success() {
            let status = response.status();
            let detail = resilience::cancelable("gemini", &options.cancellation, async {
                response.text().await.map_err(error)
            })
            .await?;
            // Go returns genai.APIError directly, not a retryable ProviderError.
            return Err(error(format!("{status}: {detail}")));
        }
        let mut stream = response.bytes_stream();
        let mut pending = Vec::new();
        let mut final_response = Value::Null;
        let mut usage = Usage::default();
        loop {
            let next = resilience::cancelable("gemini", &options.cancellation, async {
                stream.next().await.transpose().map_err(error)
            })
            .await?;
            let eof = next.is_none();
            if let Some(bytes) = next {
                pending.extend_from_slice(&bytes);
            }
            // genai scans double-newline tokens, including an unterminated
            // token at EOF, and rejects non-data fields rather than ignoring them.
            while let Some((end, delimiter)) = pending
                .windows(2)
                .position(|b| b == b"\n\n")
                .map(|i| (i, 2))
                .or_else(|| {
                    pending
                        .windows(4)
                        .position(|b| b == b"\r\n\r\n")
                        .map(|i| (i, 4))
                })
                .or_else(|| (eof && !pending.is_empty()).then_some((pending.len(), 0)))
            {
                if end >= 268435456 {
                    return Err(error("bufio.Scanner: token too long"));
                }
                let line = pending.drain(..end).collect::<Vec<_>>();
                pending.drain(..delimiter);
                let line = line.strip_suffix(b"\r").unwrap_or(&line);
                if line.is_empty() {
                    continue;
                }
                let data = line.strip_prefix(b"data:").ok_or_else(|| {
                    error(format!(
                        "iterateResponseStream: invalid stream chunk: {}",
                        String::from_utf8_lossy(line)
                    ))
                })?;
                let chunk: Value = serde_json::from_slice(data).map_err(error)?;
                if let Some(sink) = &options.text_sink {
                    for block in response_content(&chunk)? {
                        match block {
                            ContentBlock::Text(text) => sink(&text.text),
                            ContentBlock::ToolCall(call) => {
                                sink(&serde_json::to_string(&call).map_err(error)?);
                            }
                            _ => {}
                        }
                    }
                }
                if let Some(metadata) = chunk.get("usageMetadata").filter(|v| !v.is_null()) {
                    usage.input_tokens = metadata["promptTokenCount"].as_i64().unwrap_or(0);
                    usage.output_tokens = metadata["candidatesTokenCount"].as_i64().unwrap_or(0);
                }
                final_response = chunk;
            }
            if pending.len() >= 268435456 {
                return Err(error("bufio.Scanner: token too long"));
            }
            if eof {
                break;
            }
        }
        if final_response
            .pointer("/candidates/0/content")
            .is_none_or(Value::is_null)
        {
            return Err(error("no response from gemini"));
        }
        let content = response_content(&final_response)?;
        Ok(Message {
            source: "model".into(),
            content: (!content.is_empty()).then_some(content),
            usage,
        })
    }
}

fn response_content(response: &Value) -> Result<Vec<ContentBlock>, ProviderError> {
    let mut content = Vec::new();
    if let Some(parts) = response
        .pointer("/candidates/0/content/parts")
        .and_then(Value::as_array)
    {
        for part in parts {
            if let Some(text) = part["text"].as_str().filter(|s| !s.is_empty()) {
                content.push(ContentBlock::Text(TextBlock { text: text.into() }));
            } else if let Some(call) = part.get("functionCall").filter(|v| !v.is_null()) {
                content.push(ContentBlock::ToolCall(ToolCallBlock {
                    id: uuid::Builder::from_random_bytes(rand::random())
                        .into_uuid()
                        .to_string(),
                    tool: call["name"].as_str().unwrap_or_default().into(),
                    args: Some(
                        serde_json::value::RawValue::from_string(
                            call.get("args").unwrap_or(&Value::Null).to_string(),
                        )
                        .map_err(error)?,
                    ),
                }));
            }
        }
    }
    Ok(content)
}

fn object_or_null(raw: &str) -> Value {
    match serde_json::from_str::<Value>(raw) {
        Ok(v) if v.is_object() || v.is_null() => v,
        _ => json!({}),
    }
}

fn request_body(
    system: &str,
    messages: &[Message],
    tools: &[ToolDefinition],
) -> Result<Value, ProviderError> {
    let mut history = Vec::new();
    for (index, message) in messages.iter().enumerate() {
        let role = if index + 1 == messages.len() {
            "user"
        } else {
            match message.source.as_ref() {
                "user" | "system" => "user",
                "model" => "model",
                other => {
                    return Err(error(format!(
                        "Role must be user or model, but got {other}"
                    )))
                }
            }
        };
        let mut parts = Vec::new();
        for block in message.content.iter().flatten() {
            parts.push(match block {
                ContentBlock::Text(text) => {
                    if text.text.is_empty() {
                        json!({})
                    } else {
                        json!({"text": text.text})
                    }
                }
                ContentBlock::ToolCall(call) => {
                    let args = call
                        .args
                        .as_ref()
                        .map(|a| object_or_null(a.get()))
                        .unwrap_or(Value::Null);
                    let mut function = json!({"name":call.tool});
                    if args.as_object().is_some_and(|v| !v.is_empty()) {
                        function["args"] = args;
                    }
                    json!({"functionCall":function})
                }
                ContentBlock::ToolResult(result) => {
                    let payload = match serde_json::from_str::<Value>(&result.result) {
                        Ok(v) if v.is_object() || v.is_null() => v,
                        _ => json!({"result":result.result,"succeeded":result.succeeded}),
                    };
                    let mut function = json!({"name":result.name});
                    if payload.as_object().is_some_and(|v| !v.is_empty()) {
                        function["response"] = payload;
                    }
                    json!({"functionResponse":function})
                }
                ContentBlock::Null => continue,
            });
        }
        let mut content = json!({"role":role});
        if !parts.is_empty() {
            content["parts"] = json!(parts);
        }
        history.push(content);
    }
    let current = history.pop();
    let mut curated: Vec<Value> = Vec::new();
    let mut i = 0;
    while i < history.len() {
        if history[i]["role"] == "user" {
            curated.push(history[i].clone());
            i += 1;
        } else {
            let start = i;
            while i < history.len() && history[i]["role"] == "model" {
                i += 1;
            }
            let valid = history[start..i].iter().all(|c| {
                c["parts"].as_array().is_some_and(|p| {
                    !p.is_empty()
                        && p.iter()
                            .all(|p| p.as_object().is_some_and(|o| !o.is_empty()))
                })
            });
            if valid {
                curated.extend_from_slice(&history[start..i]);
            } else if curated.last().is_some_and(|c| c["role"] == "user") {
                curated.pop();
            }
        }
    }
    curated.extend(current);
    let mut body = json!({"contents":curated,"systemInstruction":{"parts":[{"text":system}]}});
    if !tools.is_empty() {
        body["tools"] = json!([{"functionDeclarations": tools.iter().map(|t| {
            let schema = object_schema(&t.input_schema).unwrap_or_else(|| json!({"type":"OBJECT","properties":{"input":{"type":"STRING"}}}));
            let mut declaration = json!({"name":t.name,"parameters":schema});
            if !t.description.is_empty() { declaration["description"] = json!(t.description); }
            declaration
        }).collect::<Vec<_>>()}]);
    }
    Ok(body)
}

fn object_schema(schema: &Value) -> Option<Value> {
    if schema["type"] != "object" {
        return None;
    }
    if schema
        .get("properties")
        .is_some_and(|v| !v.is_null() && !v.is_object())
    {
        return None;
    }
    let mut result = json!({"type":"OBJECT"});
    if let Some(properties) = schema["properties"].as_object() {
        let properties: Map<String, Value> = properties
            .iter()
            .filter(|(_, v)| v.is_object())
            .map(|(k, v)| (k.clone(), property_schema(v)))
            .collect();
        if !properties.is_empty() {
            result["properties"] = json!(properties);
        }
    }
    if let Some(required) = schema["required"]
        .as_array()
        .filter(|a| !a.is_empty() && a.iter().all(Value::is_string))
    {
        result["required"] = json!(required);
    }
    Some(result)
}

fn property_schema(schema: &Value) -> Value {
    let mut result = json!({});
    if let Some(kind) = schema["type"].as_str() {
        result["type"] = json!(match kind {
            "object" => "OBJECT",
            "number" => "NUMBER",
            "integer" => "INTEGER",
            "boolean" => "BOOLEAN",
            "array" => "ARRAY",
            _ => "STRING",
        });
        if kind == "object" && schema.get("properties").is_some_and(|v| !v.is_null()) {
            if let Some(nested) = object_schema(schema) {
                result = nested;
            }
        }
        if kind == "array" && schema["items"].is_object() {
            result["items"] = property_schema(&schema["items"]);
        }
    }
    if let Some(description) = schema["description"].as_str().filter(|s| !s.is_empty()) {
        result["description"] = json!(description);
    }
    if let Some(values) = schema["enum"]
        .as_array()
        .filter(|a| !a.is_empty() && a.iter().all(Value::is_string))
    {
        result["enum"] = json!(values);
    }
    result
}

#[cfg(test)]
mod tests {
    use super::*;
    use construct_core::model::{GeminiProfile, OpenAIProfile};
    use std::{
        io::{Read, Write},
        net::TcpListener,
        sync::{mpsc, Arc, Mutex},
    };

    fn server(
        status: u16,
        body: &str,
    ) -> (
        GeminiProvider,
        mpsc::Receiver<(String, Value)>,
        std::thread::JoinHandle<()>,
    ) {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let mut options = ProviderOptions::new("gemini");
        options.url = format!("http://{}", listener.local_addr().unwrap());
        let provider = GeminiProvider::with_options("secret-test-key", options).unwrap();
        let (tx, rx) = mpsc::channel();
        let body = body.to_owned();
        let handle = std::thread::spawn(move || {
            let (mut socket, _) = listener.accept().unwrap();
            socket
                .set_read_timeout(Some(std::time::Duration::from_secs(5)))
                .unwrap();
            let mut bytes = Vec::new();
            let mut buf = [0; 4096];
            let (headers, value) = loop {
                let count = socket.read(&mut buf).unwrap();
                assert!(count > 0);
                bytes.extend_from_slice(&buf[..count]);
                if let Some(end) = bytes.windows(4).position(|w| w == b"\r\n\r\n") {
                    let headers = String::from_utf8(bytes[..end].to_vec()).unwrap();
                    let length: usize = headers
                        .lines()
                        .find_map(|l| {
                            l.to_ascii_lowercase()
                                .strip_prefix("content-length: ")
                                .map(str::to_owned)
                        })
                        .unwrap()
                        .parse()
                        .unwrap();
                    if bytes.len() >= end + 4 + length {
                        break (
                            headers,
                            serde_json::from_slice(&bytes[end + 4..end + 4 + length]).unwrap(),
                        );
                    }
                }
            };
            tx.send((headers, value)).unwrap();
            write!(socket, "HTTP/1.1 {status} Test\r\nContent-Type: text/event-stream\r\nContent-Length: {}\r\nConnection: close\r\n\r\n", body.len()).unwrap();
            for chunk in body.as_bytes().chunks(3) {
                socket.write_all(chunk).unwrap();
            }
        });
        (provider, rx, handle)
    }

    fn messages() -> Vec<Message> {
        vec![Message {
            source: "user".into(),
            content: Some(vec![ContentBlock::Text(TextBlock {
                text: "hello".into(),
            })]),
            ..Default::default()
        }]
    }

    #[tokio::test]
    async fn request_and_stream_match_go() {
        let (provider, request, handle) = server(200, concat!(
            "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hé\"}]}}],\"usageMetadata\":{\"promptTokenCount\":12,\"candidatesTokenCount\":1}}\r\n\r\n",
            "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"last\"},{\"functionCall\":{\"name\":\"search\",\"args\":{\"q\":\"rust\"}}}]}}],\"usageMetadata\":{\"promptTokenCount\":13,\"candidatesTokenCount\":4,\"thoughtsTokenCount\":99}}"
        ));
        let seen = Arc::new(Mutex::new(Vec::new()));
        let observed = seen.clone();
        let options = InvokeOptions {
            text_sink: Some(Arc::new(move |s| {
                observed.lock().unwrap().push(s.to_owned())
            })),
            tools: vec![ToolDefinition {
                name: "search".into(),
                description: "Find things".into(),
                input_schema: json!({"type":"object","properties":{"q":{"type":"string","description":"Query"},"n":{"type":"integer"}},"required":["q"]}),
            }],
            profile: Some(ModelProfile::Gemini(GeminiProfile {
                api_key: "ignored".into(),
                base_url: "http://invalid".into(),
                max_retries: 10,
                default_temperature: Some(0.4),
                default_max_tokens: Some(5),
                ..Default::default()
            })),
            ..Default::default()
        };
        let result = provider
            .invoke_model("gemini-2.5-pro", "system", &messages(), options)
            .await
            .unwrap();
        let (headers, body) = request.recv().unwrap();
        handle.join().unwrap();
        assert!(headers.starts_with(
            "POST /v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse HTTP/1.1"
        ));
        assert!(headers.contains("x-goog-api-key: secret-test-key"));
        assert_eq!(
            body,
            json!({"contents":[{"role":"user","parts":[{"text":"hello"}]}],"systemInstruction":{"parts":[{"text":"system"}]},"tools":[{"functionDeclarations":[{"name":"search","description":"Find things","parameters":{"type":"OBJECT","properties":{"q":{"type":"STRING","description":"Query"},"n":{"type":"INTEGER"}},"required":["q"]}}]}]})
        );
        assert_eq!(result.usage.input_tokens, 13);
        assert_eq!(result.usage.output_tokens, 4);
        let content = result.content.unwrap();
        assert_eq!(content.len(), 2); // Go returns only the last chunk.
        let ContentBlock::ToolCall(call) = &content[1] else {
            panic!()
        };
        assert_eq!(call.tool, "search");
        assert_eq!(call.args.as_ref().unwrap().get(), "{\"q\":\"rust\"}");
        assert_eq!(
            uuid::Uuid::parse_str(&call.id).unwrap().get_version_num(),
            4
        );
        let seen = seen.lock().unwrap();
        assert_eq!(&seen[..2], &["hé", "last"]);
        let streamed: Value = serde_json::from_str(&seen[2]).unwrap();
        assert_ne!(streamed["id"], call.id);
        assert_eq!(streamed["args"], json!({"q":"rust"}));
    }

    #[test]
    fn history_and_schema_conversion() {
        let calls: Vec<Message> = serde_json::from_value(json!([
            {"source":"model","content":[{"tool":"search","args":{"q":"x"},"id":"ignored"}]},
            {"source":"system","content":[{"name":"search","result":"{}","succeeded":true}]}
        ]))
        .unwrap();
        assert_eq!(
            request_body("sys", &calls, &[]).unwrap()["contents"],
            json!([
                {"role":"model","parts":[{"functionCall":{"name":"search","args":{"q":"x"}}}]},
                {"role":"user","parts":[{"functionResponse":{"name":"search"}}]}
            ])
        );
        assert_eq!(
            request_body("sys", &[Message::default()], &[]).unwrap()["contents"],
            json!([{"role":"user"}])
        );
        let input: Vec<Message> = serde_json::from_value(json!([
            {"source":"user","content":[{"Text":"discard"}]},
            {"source":"model","content":[{"Text":""}]},
            {"source":"model","content":[{"tool":"search","args":{"q":"x"},"id":"ignored"}]},
            {"source":"system","content":[{"name":"search","result":"not JSON","succeeded":false}]},
            {"source":"model","content":[{"name":"search","result":"{\"ok\":true}","succeeded":true}]}
        ])).unwrap();
        let body = request_body("sys", &input, &[]).unwrap();
        assert_eq!(
            body["contents"],
            json!([
                {"role":"user","parts":[{"functionResponse":{"name":"search","response":{"result":"not JSON","succeeded":false}}}]},
                {"role":"user","parts":[{"functionResponse":{"name":"search","response":{"ok":true}}}]}
            ])
        );
        assert_eq!(
            property_schema(
                &json!({"type":"array","items":{"type":"object","properties":{"v":{"type":"weird","enum":["a","b"]},"invalid":1},"required":["v"]}})
            ),
            json!({"type":"ARRAY","items":{"type":"OBJECT","properties":{"v":{"type":"STRING","enum":["a","b"]}},"required":["v"]}})
        );
        assert_eq!(
            property_schema(&json!({"description":"untyped","enum":[1]})),
            json!({"description":"untyped"})
        );
        let tool = ToolDefinition {
            name: "bad".into(),
            description: String::new(),
            input_schema: json!({"type":"string"}),
        };
        assert_eq!(
            request_body("sys", &messages(), &[tool]).unwrap()["tools"][0]["functionDeclarations"]
                [0]["parameters"],
            json!({"type":"OBJECT","properties":{"input":{"type":"STRING"}}})
        );
    }

    #[tokio::test]
    async fn errors_are_unknown_and_not_retried() {
        for status in [400, 401, 429, 500, 503] {
            let (provider, request, handle) =
                server(status, "{\"error\":{\"message\":\"failure\"}}");
            let result = provider
                .invoke_model("models/test", "sys", &messages(), InvokeOptions::default())
                .await
                .unwrap_err();
            request.recv().unwrap();
            handle.join().unwrap();
            assert_eq!(result.kind.as_ref(), "unknown");
            assert!(!result.retryable().0);
            assert!(result.err.unwrap().contains(&status.to_string()));
        }
    }

    #[tokio::test]
    async fn empty_malformed_and_usage_only_streams() {
        for (body, expected) in [
            ("", "no response from gemini"),
            ("data: {broken}\n\n", "key must be a string"),
            ("event: message\n\n", "invalid stream chunk"),
            ("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hi\"}]}}]}\n\ndata: {\"usageMetadata\":{\"promptTokenCount\":1}}\n\n", "no response from gemini"),
        ] {
            let (provider, request, handle) = server(200, body);
            let result = provider.invoke_model("test", "sys", &messages(), InvokeOptions::default()).await.unwrap_err();
            request.recv().unwrap();
            handle.join().unwrap();
            assert!(result.err.unwrap().contains(expected));
        }
        let (provider, request, handle) =
            server(200, "data: {\"candidates\":[{\"content\":{}}]}\n\n");
        let result = provider
            .invoke_model("test", "sys", &messages(), InvokeOptions::default())
            .await
            .unwrap();
        assert!(result.content.is_none());
        request.recv().unwrap();
        handle.join().unwrap();
    }

    #[tokio::test]
    async fn validation_and_cancellation() {
        assert!(GeminiProvider::new("").is_err());
        let provider = GeminiProvider::new("key").unwrap();
        for (model, system, input, options, expected) in [
            (
                "",
                "sys",
                messages(),
                InvokeOptions::default(),
                "model is required",
            ),
            (
                "test",
                "",
                messages(),
                InvokeOptions::default(),
                "system prompt is required",
            ),
            (
                "test",
                "sys",
                vec![],
                InvokeOptions::default(),
                "at least one message is required",
            ),
            (
                "test",
                "sys",
                messages(),
                InvokeOptions {
                    profile: Some(ModelProfile::OpenAI(OpenAIProfile::default())),
                    ..Default::default()
                },
                "not a Gemini",
            ),
            (
                "test",
                "sys",
                messages(),
                InvokeOptions {
                    profile: Some(ModelProfile::Gemini(GeminiProfile {
                        default_top_k: Some(-1),
                        ..Default::default()
                    })),
                    ..Default::default()
                },
                "non-negative",
            ),
        ] {
            assert!(provider
                .invoke_model(model, system, &input, options)
                .await
                .unwrap_err()
                .err
                .unwrap()
                .contains(expected));
        }
        let options = InvokeOptions::default();
        options.cancellation.cancel();
        assert_eq!(
            provider
                .invoke_model("test", "sys", &messages(), options)
                .await
                .unwrap_err()
                .kind
                .as_ref(),
            "canceled"
        );
    }
}
