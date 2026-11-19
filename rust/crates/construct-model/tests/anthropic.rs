use axum::{
    body::{Body, Bytes},
    extract::State,
    http::{HeaderMap, StatusCode},
    response::Response,
    routing::post,
    Router,
};
use construct_core::{
    model::{
        AnthropicProfile, InvokeOptions, ModelProfile, ModelProvider, OpenAIProfile, ToolDefinition,
    },
    CancellationToken,
};
use construct_model::{anthropic::AnthropicProvider, ProviderOptions};
use construct_types::provider::{
    ContentBlock, Message, ProviderErrorKind, TextBlock, ToolCallBlock, ToolResultBlock,
};
use serde_json::{json, value::RawValue, Value};
use std::{
    collections::VecDeque,
    sync::{Arc, Mutex},
};
use tokio::sync::Notify;

#[derive(Clone)]
struct Reply {
    status: StatusCode,
    headers: HeaderMap,
    body: String,
    stall: bool,
}
impl Reply {
    fn stream() -> Self {
        Self {
            status: StatusCode::OK,
            headers: HeaderMap::new(),
            body: fixture(),
            stall: false,
        }
    }
    fn error(status: u16) -> Self {
        let mut headers = HeaderMap::new();
        headers.insert("retry-after", "0".parse().unwrap());
        Self {
            status: StatusCode::from_u16(status).unwrap(),
            headers,
            body: r#"{"type":"error","error":{"type":"api_error","message":"failure"}}"#.into(),
            stall: false,
        }
    }
}
type Recorded = Arc<Mutex<Vec<(HeaderMap, String)>>>;
#[derive(Clone)]
struct Mock {
    replies: Arc<Mutex<VecDeque<Reply>>>,
    recorded: Recorded,
    received: Arc<Notify>,
}
struct Server {
    url: String,
    mock: Mock,
    task: tokio::task::JoinHandle<()>,
}
impl Drop for Server {
    fn drop(&mut self) {
        self.task.abort();
    }
}
async fn server(replies: Vec<Reply>) -> Server {
    let mock = Mock {
        replies: Arc::new(Mutex::new(replies.into())),
        recorded: Arc::default(),
        received: Arc::default(),
    };
    let app = Router::new()
        .route(
            "/v1/messages",
            post(
                |State(mock): State<Mock>, headers: HeaderMap, body: Bytes| async move {
                    mock.recorded
                        .lock()
                        .unwrap()
                        .push((headers, String::from_utf8(body.to_vec()).unwrap()));
                    mock.received.notify_one();
                    let reply = mock
                        .replies
                        .lock()
                        .unwrap()
                        .pop_front()
                        .expect("unexpected HTTP attempt");
                    let body = if reply.stall {
                        Body::from_stream(futures_util::stream::pending::<
                            Result<Bytes, std::io::Error>,
                        >())
                    } else {
                        // Byte-sized chunks exercise SSE framing independently from HTTP chunk boundaries.
                        Body::from_stream(futures_util::stream::iter(
                            reply
                                .body
                                .into_bytes()
                                .into_iter()
                                .map(|b| Ok::<_, std::io::Error>(Bytes::from(vec![b]))),
                        ))
                    };
                    let mut response = Response::new(body);
                    *response.status_mut() = reply.status;
                    *response.headers_mut() = reply.headers;
                    response
                },
            ),
        )
        .with_state(mock.clone());
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let url = format!("http://{}", listener.local_addr().unwrap());
    let task = tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });
    Server { url, mock, task }
}
fn provider(server: &Server) -> AnthropicProvider {
    let mut options = ProviderOptions::new("anthropic");
    options.url = server.url.clone();
    AnthropicProvider::new("test-key", options).unwrap()
}
fn user(text: &str) -> Message {
    Message {
        source: "user".into(),
        content: Some(vec![ContentBlock::Text(TextBlock { text: text.into() })]),
        ..Default::default()
    }
}
fn fixture() -> String {
    [
        json!({"type":"message_start","message":{"content":[],"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":3,"cache_read_input_tokens":4}}}),
        json!({"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}),
        json!({"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hé"}}),
        json!({"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"llo"}}),
        json!({"type":"content_block_stop","index":0}),
        json!({"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call2","name":"lookup","input":{}}}),
        json!({"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"z\":1e2,"}}),
        json!({"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"a\":\"<&>\"}"}}),
        json!({"type":"content_block_stop","index":1}),
        json!({"type":"message_delta","usage":{"output_tokens":9}}),
        json!({"type":"message_stop"}),
    ].iter().map(|v| format!("event: {}\ndata: {}\n\n", v["type"].as_str().unwrap(), v)).collect()
}

#[tokio::test]
async fn exact_request_and_streamed_response() {
    let server = server(vec![Reply::stream()]).await;
    let provider = provider(&server);
    let chunks = Arc::new(Mutex::new(Vec::new()));
    let observer = chunks.clone();
    let messages = vec![
        user("oldest"),
        Message {
            source: "model".into(),
            content: Some(vec![ContentBlock::ToolCall(ToolCallBlock {
                id: "call1".into(),
                tool: "lookup".into(),
                args: Some(RawValue::from_string(r#"{"z":1e2,"a":2}"#.into()).unwrap()),
            })]),
            ..Default::default()
        },
        Message {
            source: "user".into(),
            content: Some(vec![ContentBlock::ToolResult(ToolResultBlock {
                id: "call1".into(),
                name: "ignored".into(),
                result: "found".into(),
                succeeded: false,
            })]),
            ..Default::default()
        },
        Message {
            source: "system".into(),
            content: Some(vec![ContentBlock::Text(TextBlock {
                text: "system turn".into(),
            })]),
            ..Default::default()
        },
        user("continue"),
    ];
    let options = InvokeOptions {
        tools: vec![ToolDefinition {
            name: "lookup".into(),
            description: "Find a thing".into(),
            input_schema: json!({"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}),
        }],
        text_sink: Some(Arc::new(move |text| {
            observer.lock().unwrap().push(text.to_owned())
        })),
        ..Default::default()
    };
    let result = provider
        .invoke_model("claude-test", "system", &messages, options)
        .await
        .unwrap();
    assert_eq!(*chunks.lock().unwrap(), ["hé", "llo"]);
    assert_eq!(result.source.as_ref(), "model");
    assert_eq!(result.usage.input_tokens, 10);
    assert_eq!(result.usage.output_tokens, 9);
    assert_eq!(result.usage.cache_write_tokens, 3);
    assert_eq!(result.usage.cache_read_tokens, 4);
    let content = result.content.unwrap();
    let ContentBlock::Text(text) = &content[0] else {
        panic!("expected text")
    };
    assert_eq!(text.text, "héllo");
    let ContentBlock::ToolCall(tool) = &content[1] else {
        panic!("expected tool")
    };
    assert_eq!(tool.id, "call2");
    assert_eq!(tool.tool, "lookup");
    assert_eq!(tool.args.as_ref().unwrap().get(), r#"{"z":1e2,"a":"<&>"}"#);
    let recorded = server.mock.recorded.lock().unwrap();
    assert_eq!(recorded.len(), 1);
    let (headers, body) = &recorded[0];
    assert_eq!(headers["x-api-key"], "test-key");
    assert_eq!(headers["anthropic-version"], "2023-06-01");
    assert_eq!(headers["accept"], "application/json");
    assert_eq!(headers["content-type"], "application/json");
    assert_eq!(headers["x-stainless-retry-count"], "0");
    assert!(!headers.contains_key("anthropic-beta"));
    assert!(body.contains(r#""input":{"z":1e2,"a":2}"#));
    assert_eq!(
        serde_json::from_str::<Value>(body).unwrap(),
        json!({
            "model":"claude-test","max_tokens":8192,"stream":true,
            "system":[{"type":"text","text":"system","cache_control":{"type":"ephemeral"}}],
            "messages":[
                {"role":"user","content":[{"type":"text","text":"oldest"}]},
                {"role":"assistant","content":[{"type":"tool_use","id":"call1","name":"lookup","input":{"z":100.0,"a":2}}]},
                {"role":"user","content":[{"type":"tool_result","tool_use_id":"call1","is_error":true,"content":[{"type":"text","text":"found"}],"cache_control":{"type":"ephemeral"}}]},
                {"role":"user","content":[{"type":"text","text":"system turn"}]},
                {"role":"user","content":[{"type":"text","text":"continue","cache_control":{"type":"ephemeral"}}]}
            ],
            "tools":[{"name":"lookup","description":"Find a thing","input_schema":{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false},"cache_control":{"type":"ephemeral"}}],
            "tool_choice":{"type":"auto"}
        })
    );
}

#[tokio::test]
async fn explicit_profile_only_changes_max_tokens_like_go() {
    let server = server(vec![Reply::stream()]).await;
    let profile = AnthropicProfile {
        max_tokens: 123,
        temperature: 0.7,
        anthropic_version: "custom".into(),
        anthropic_beta: vec!["custom-beta".into()],
        enable_prompt_caching: true,
        ..Default::default()
    };
    provider(&server)
        .invoke_model(
            "model",
            "system",
            &[user("hello")],
            InvokeOptions {
                profile: Some(ModelProfile::Anthropic(profile)),
                ..Default::default()
            },
        )
        .await
        .unwrap();
    let recorded = server.mock.recorded.lock().unwrap();
    let (headers, body) = &recorded[0];
    assert_eq!(headers["anthropic-version"], "2023-06-01");
    assert!(!headers.contains_key("anthropic-beta"));
    let value: Value = serde_json::from_str(body).unwrap();
    assert_eq!(value["max_tokens"], 123);
    assert!(value.get("temperature").is_none());
    assert!(value.get("tools").is_none());
    assert!(value.get("tool_choice").is_none());
}

#[tokio::test]
async fn http_errors_and_sdk_then_outer_retries() {
    for (status, kind, requests) in [
        (400, ProviderErrorKind::INVALID_REQUEST, 1),
        (429, ProviderErrorKind::RATE_LIMIT_EXCEEDED, 3),
    ] {
        let mut reply = Reply::error(status);
        reply
            .headers
            .insert("retry-after", "0.001".parse().unwrap());
        let server = server(vec![reply; requests]).await;
        let error = provider(&server)
            .invoke_model(
                "model",
                "system",
                &[user("hello")],
                InvokeOptions::default(),
            )
            .await
            .unwrap_err();
        assert_eq!(error.kind.as_ref(), kind);
        assert_eq!(error.retry_after, if status == 429 { 1_000_000 } else { 0 });
        assert_eq!(server.mock.recorded.lock().unwrap().len(), requests);
    }
    let mut replies = vec![Reply::error(529); 3];
    replies.push(Reply::stream());
    let server = server(replies).await;
    let callbacks = Arc::new(Mutex::new(0));
    let observer = callbacks.clone();
    provider(&server)
        .invoke_model(
            "model",
            "system",
            &[user("hello")],
            InvokeOptions {
                retry_callback: Some(Arc::new(move |error, _| {
                    assert_eq!(error.kind.as_ref(), ProviderErrorKind::OVERLOADED);
                    *observer.lock().unwrap() += 1;
                })),
                ..Default::default()
            },
        )
        .await
        .unwrap();
    assert_eq!(*callbacks.lock().unwrap(), 1);
    let recorded = server.mock.recorded.lock().unwrap();
    assert_eq!(recorded.len(), 4);
    assert_eq!(
        recorded
            .iter()
            .map(|(h, _)| h["x-stainless-retry-count"].to_str().unwrap())
            .collect::<Vec<_>>(),
        ["0", "1", "2", "0"]
    );
}

#[tokio::test]
async fn validation_before_io_and_cancellation() {
    let server = server(vec![]).await;
    let provider = provider(&server);
    assert!(AnthropicProvider::new("", ProviderOptions::new("anthropic")).is_err());
    for (model, system, messages, options) in [
        ("", "system", vec![user("hi")], InvokeOptions::default()),
        ("model", "", vec![user("hi")], InvokeOptions::default()),
        ("model", "system", vec![], InvokeOptions::default()),
        (
            "model",
            "system",
            vec![user("hi")],
            InvokeOptions {
                profile: Some(ModelProfile::OpenAI(OpenAIProfile::default())),
                ..Default::default()
            },
        ),
        (
            "model",
            "system",
            vec![user("hi")],
            InvokeOptions {
                profile: Some(ModelProfile::Anthropic(AnthropicProfile {
                    temperature: 2.0,
                    ..Default::default()
                })),
                ..Default::default()
            },
        ),
    ] {
        assert_eq!(
            provider
                .invoke_model(model, system, &messages, options)
                .await
                .unwrap_err()
                .kind
                .as_ref(),
            ProviderErrorKind::INVALID_REQUEST
        );
    }
    let token = CancellationToken::new();
    token.cancel();
    let error = provider
        .invoke_model(
            "model",
            "system",
            &[user("hi")],
            InvokeOptions {
                cancellation: token,
                ..Default::default()
            },
        )
        .await
        .unwrap_err();
    assert_eq!(error.kind.as_ref(), ProviderErrorKind::CANCELED);
    assert!(server.mock.recorded.lock().unwrap().is_empty());
    let mut reply = Reply::stream();
    reply.stall = true;
    let stalled = self::server(vec![reply]).await;
    let token = CancellationToken::new();
    let cancel = token.clone();
    let notified = stalled.mock.received.clone();
    let task = tokio::spawn(async move {
        notified.notified().await;
        cancel.cancel();
    });
    let error = self::provider(&stalled)
        .invoke_model(
            "model",
            "system",
            &[user("hi")],
            InvokeOptions {
                cancellation: token,
                ..Default::default()
            },
        )
        .await
        .unwrap_err();
    assert_eq!(error.kind.as_ref(), ProviderErrorKind::CANCELED);
    task.await.unwrap();
}

#[tokio::test]
#[ignore = "requires ANTHROPIC_API_KEY and makes a billed API call"]
async fn live_anthropic() {
    let Ok(key) = std::env::var("ANTHROPIC_API_KEY") else {
        return;
    };
    let provider = AnthropicProvider::new(key, ProviderOptions::new("anthropic")).unwrap();
    let result = provider
        .invoke_model(
            construct_model::catalog::anthropic::BUDGET_MODEL,
            "Reply briefly.",
            &[user("Say hello.")],
            InvokeOptions::default(),
        )
        .await
        .unwrap();
    assert!(!result.content.unwrap().is_empty());
    assert!(result.usage.output_tokens > 0);
}
