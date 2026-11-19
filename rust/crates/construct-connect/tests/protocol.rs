use axum::{
    body::{to_bytes, Body},
    extract::Request as HttpRequest,
    response::Response as HttpResponse,
    routing::post,
};
use bytes::Bytes;
use construct_connect::{
    generated, serve_tcp, Channel, ClientConfig, Code, ConnectError, Request, Response, Router,
    Streaming,
};
use futures_util::{stream, StreamExt};
use prost::Message;
use serde::{Deserialize, Serialize};
use std::{convert::Infallible, time::Duration};
use tokio::{net::TcpListener, sync::oneshot};

#[derive(Clone, PartialEq, Message, Serialize, Deserialize)]
struct Empty {}

fn request(content_type: &str, bytes: impl Into<Body>) -> HttpRequest {
    HttpRequest::builder()
        .method("POST")
        .uri("/example.Service/Call")
        .header("content-type", content_type)
        .header("connect-protocol-version", "1")
        .body(bytes.into())
        .unwrap()
}

async fn unary(request: HttpRequest) -> HttpResponse {
    generated::unary(request, |req: Request<Empty>| async {
        Ok(Response::new(req.message))
    })
    .await
}

async fn streaming(request: HttpRequest) -> HttpResponse {
    generated::server_streaming(request, |_: Request<Empty>| async {
        Ok(Response::new(Box::pin(stream::empty()) as Streaming<Empty>))
    })
    .await
}

#[tokio::test]
async fn unary_error_statuses_and_names_match_the_spec() {
    let expected = [
        (Code::Canceled, "canceled", 499),
        (Code::Unknown, "unknown", 500),
        (Code::InvalidArgument, "invalid_argument", 400),
        (Code::DeadlineExceeded, "deadline_exceeded", 504),
        (Code::NotFound, "not_found", 404),
        (Code::AlreadyExists, "already_exists", 409),
        (Code::PermissionDenied, "permission_denied", 403),
        (Code::ResourceExhausted, "resource_exhausted", 429),
        (Code::FailedPrecondition, "failed_precondition", 400),
        (Code::Aborted, "aborted", 409),
        (Code::OutOfRange, "out_of_range", 400),
        (Code::Unimplemented, "unimplemented", 501),
        (Code::Internal, "internal", 500),
        (Code::Unavailable, "unavailable", 503),
        (Code::DataLoss, "data_loss", 500),
        (Code::Unauthenticated, "unauthenticated", 401),
    ];
    assert_eq!(Code::ALL.len(), expected.len());
    for (code, name, status) in expected {
        assert_eq!(code.as_str(), name);
        let response = generated::unary(
            request("application/proto", Body::empty()),
            |_: Request<Empty>| async move {
                Err::<Response<Empty>, _>(ConnectError::new(code, "example failure"))
            },
        )
        .await;
        assert_eq!(response.status().as_u16(), status);
        assert_eq!(response.headers()["content-type"], "application/json");
        let body = to_bytes(response.into_body(), 1024).await.unwrap();
        assert_eq!(
            serde_json::from_slice::<serde_json::Value>(&body).unwrap(),
            serde_json::json!({"code": name, "message": "example failure"})
        );
    }
}

#[tokio::test]
async fn wire_framing_and_content_types() {
    let response = unary(request("application/json; charset=utf-8", "{}")).await;
    assert_eq!(response.status(), 200);
    assert_eq!(to_bytes(response.into_body(), 1024).await.unwrap(), "{}");
    assert_eq!(unary(request("text/plain", "{}")).await.status(), 415);
    let response = unary(request("application/json", "{")).await;
    assert_eq!(response.status(), 400);
    let response = streaming(request(
        "application/connect+proto",
        Bytes::from_static(&[0, 0, 0, 0, 0]),
    ))
    .await;
    assert_eq!(response.status(), 200);
    assert_eq!(
        response.headers()["content-type"],
        "application/connect+proto"
    );
    assert_eq!(
        to_bytes(response.into_body(), 1024).await.unwrap().as_ref(),
        &[2, 0, 0, 0, 2, b'{', b'}']
    );
    for (bytes, code) in [
        (vec![], Code::InvalidArgument),
        (vec![0, 0, 0, 0, 1], Code::InvalidArgument),
        (vec![0, 0, 0, 0, 0, 0, 0, 0, 0, 0], Code::InvalidArgument),
        (vec![1, 0, 0, 0, 0], Code::Unimplemented),
        (vec![4, 0, 0, 0, 0], Code::InvalidArgument),
        (vec![2, 0, 0, 0, 0], Code::InvalidArgument),
        (vec![0, 1, 0, 0, 1], Code::ResourceExhausted),
    ] {
        let response = streaming(request("application/connect+proto", Bytes::from(bytes))).await;
        assert_eq!(response.status(), 200);
        let body = to_bytes(response.into_body(), 1024).await.unwrap();
        assert_eq!(body[0], 2);
        assert_eq!(
            u32::from_be_bytes(body[1..5].try_into().unwrap()) as usize,
            body.len() - 5
        );
        let value: serde_json::Value = serde_json::from_slice(&body[5..]).unwrap();
        assert_eq!(value["error"]["code"], code.as_str());
    }
}

#[tokio::test]
async fn invalid_and_zero_timeouts_never_run_the_handler() {
    for (timeout, status) in [
        ("0", 504),
        ("-1", 400),
        ("1.5", 400),
        ("10000000000", 400),
        ("", 400),
    ] {
        let mut req = request("application/proto", Body::empty());
        req.headers_mut()
            .insert("connect-timeout-ms", timeout.parse().unwrap());
        let response = generated::unary(req, |_: Request<Empty>| async {
            panic!("handler must not execute");
            #[allow(unreachable_code)]
            Ok::<_, ConnectError>(Response::new(Empty {}))
        })
        .await;
        assert_eq!(response.status().as_u16(), status);
    }
}

#[tokio::test]
async fn client_handles_fragmented_and_invalid_streams() {
    let router = Router::new().route(
        "/{mode}",
        post(|req: HttpRequest| async move {
            let mode = req.uri().path();
            let payload = match mode {
                "/ok" => vec![0, 0, 0, 0, 0, 2, 0, 0, 0, 2, b'{', b'}'],
                "/missing-end" => vec![0, 0, 0, 0, 0],
                "/compressed" => vec![1, 0, 0, 0, 0],
                "/null-error" => {
                    let json = br#"{"error":null}"#;
                    let mut bytes = vec![2, 0, 0, 0, json.len() as u8];
                    bytes.extend_from_slice(json);
                    bytes
                }
                _ => vec![],
            };
            // Individual HTTP chunks deliberately split both envelope headers and JSON.
            let chunks = payload
                .into_iter()
                .map(|byte| Ok::<_, Infallible>(Bytes::from(vec![byte])))
                .collect::<Vec<_>>();
            HttpResponse::builder()
                .header("content-type", "application/connect+proto")
                .body(Body::from_stream(stream::iter(chunks)))
                .unwrap()
        }),
    );
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let channel = Channel::new(ClientConfig::new(format!(
        "http://{}",
        listener.local_addr().unwrap()
    )))
    .unwrap();
    let (stop, shutdown) = oneshot::channel();
    let task = tokio::spawn(serve_tcp(listener, router, async {
        let _ = shutdown.await;
    }));
    let mut response = channel
        .server_streaming::<_, Empty>("/ok", Request::new(Empty {}))
        .await
        .unwrap();
    assert_eq!(response.message.next().await.unwrap().unwrap(), Empty {});
    assert!(response.message.next().await.is_none());
    for (path, code) in [
        ("/missing-end", Code::Internal),
        ("/compressed", Code::Unimplemented),
        ("/null-error", Code::Internal),
        ("/empty", Code::Internal),
    ] {
        let mut response = channel
            .server_streaming::<_, Empty>(path, Request::new(Empty {}))
            .await
            .unwrap();
        if path == "/missing-end" {
            assert!(response.message.next().await.unwrap().is_ok());
        }
        assert_eq!(
            response.message.next().await.unwrap().unwrap_err().code,
            code
        );
        assert!(response.message.next().await.is_none());
    }
    stop.send(()).unwrap();
    task.await.unwrap().unwrap();
}

#[tokio::test]
async fn invalid_endpoint_and_timeout_are_errors_not_panics() {
    for url in ["ftp://localhost", "localhost", "http://localhost?q=x"] {
        assert!(Channel::new(ClientConfig::new(url)).is_err());
    }
    let mut config = ClientConfig::new("http://127.0.0.1:1");
    config.timeout = Some(Duration::MAX);
    let channel = Channel::new(config).unwrap();
    let result = channel
        .unary::<_, Empty>("/example", Request::new(Empty {}))
        .await;
    assert_eq!(result.unwrap_err().code, Code::InvalidArgument);
}

#[tokio::test]
async fn request_interceptor_passes_path_and_extensions_to_both_handler_kinds() {
    use construct_connect::{with_request_interceptor, RequestInterceptor};
    use std::sync::Arc;
    use tower::ServiceExt;

    struct Context;
    #[construct_connect::async_trait]
    impl RequestInterceptor for Context {
        async fn intercept(&self, parts: &mut http::request::Parts) -> Result<(), ConnectError> {
            assert_eq!(parts.uri.path(), "/example.Service/Call");
            assert_eq!(parts.headers["x-test"], "context");
            if parts.headers.contains_key("x-reject") {
                return Err(ConnectError::new(Code::Unauthenticated, "denied"));
            }
            parts.extensions.insert(String::from("identity"));
            Ok(())
        }
    }

    for streaming in [false, true] {
        let router = Router::new().route(
            "/example.Service/Call",
            post(move |request: HttpRequest| async move {
                if streaming {
                    generated::server_streaming(request, |req: Request<Empty>| async move {
                        assert_eq!(req.extensions.get::<String>().unwrap(), "identity");
                        assert_eq!(req.metadata["x-test"], "context");
                        Ok(Response::new(Box::pin(stream::empty()) as Streaming<Empty>))
                    })
                    .await
                } else {
                    generated::unary(request, |req: Request<Empty>| async move {
                        assert_eq!(req.extensions.get::<String>().unwrap(), "identity");
                        assert_eq!(req.metadata["x-test"], "context");
                        Ok(Response::new(Empty {}))
                    })
                    .await
                }
            }),
        );
        let router = with_request_interceptor(router, Arc::new(Context));
        for reject in [false, true] {
            let mut req = if streaming {
                request(
                    "application/connect+proto",
                    Bytes::from_static(&[0, 0, 0, 0, 0]),
                )
            } else {
                request("application/json", "{}")
            };
            req.headers_mut()
                .insert("x-test", "context".parse().unwrap());
            if reject {
                req.headers_mut().insert("x-reject", "yes".parse().unwrap());
            }
            let response = router.clone().oneshot(req).await.unwrap();
            assert_eq!(
                response.status().as_u16(),
                if reject && !streaming { 401 } else { 200 }
            );
            if reject {
                let body = to_bytes(response.into_body(), 4096).await.unwrap();
                assert!(std::str::from_utf8(&body[if streaming { 5 } else { 0 }..])
                    .unwrap()
                    .contains("unauthenticated"));
            }
        }
    }
}
