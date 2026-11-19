use crate::{
    codec::{self, EndStream, MAX_MESSAGE_SIZE},
    Code, Codec, ConnectError, Request, Response, Streaming, Transport,
};
use axum::{
    body::{to_bytes, Body},
    extract::Request as HttpRequest,
    middleware::{self, Next},
    response::Response as HttpResponse,
    Router,
};
use bytes::BytesMut;
use futures_util::StreamExt;
use http::{HeaderMap, StatusCode};
use hyper_util::{
    rt::{TokioExecutor, TokioIo},
    server::conn::auto::Builder,
    service::TowerToHyperService,
};
use prost::Message;
use serde::{de::DeserializeOwned, Serialize};
use std::{convert::Infallible, future::Future, io, path::PathBuf, sync::Arc, time::Duration};
use tokio::{
    net::{TcpListener, UnixListener},
    sync::watch,
    task::JoinSet,
    time::{timeout_at, Instant},
};
use tower::ServiceExt;

#[crate::async_trait]
pub trait Interceptor: Send + Sync + 'static {
    async fn intercept(
        &self,
        headers: &HeaderMap,
        transport: Option<Transport>,
    ) -> Result<(), ConnectError>;
}

/// Request-aware hook supporting procedure authorization and handler-local context.
#[crate::async_trait]
pub trait RequestInterceptor: Send + Sync + 'static {
    /// Inspect the request and optionally insert typed extensions before decoding.
    async fn intercept(&self, request: &mut http::request::Parts) -> Result<(), ConnectError>;
}

/// Apply a request-aware hook to unary and streaming service routes.
pub fn with_request_interceptor(
    router: Router,
    interceptor: Arc<dyn RequestInterceptor>,
) -> Router {
    router.layer(middleware::from_fn(
        move |request: HttpRequest, next: Next| {
            let interceptor = interceptor.clone();
            async move {
                let (mut parts, body) = request.into_parts();
                match interceptor.intercept(&mut parts).await {
                    Ok(()) => next.run(HttpRequest::from_parts(parts, body)).await,
                    Err(error) => {
                        let streaming = parts
                            .headers
                            .get("content-type")
                            .and_then(|h| h.to_str().ok())
                            .is_some_and(|h| h.starts_with("application/connect+"));
                        error_response(
                            error,
                            streaming,
                            Codec::from_headers(&parts.headers, streaming).unwrap_or_default(),
                        )
                    }
                }
            }
        },
    ))
}

/// Apply an authentication/authorization hook before any service handler runs.
pub fn with_interceptor(router: Router, interceptor: Arc<dyn Interceptor>) -> Router {
    router.layer(middleware::from_fn(
        move |request: HttpRequest, next: Next| {
            let interceptor = interceptor.clone();
            async move {
                match interceptor
                    .intercept(
                        request.headers(),
                        request.extensions().get::<Transport>().copied(),
                    )
                    .await
                {
                    Ok(()) => next.run(request).await,
                    Err(error) => {
                        let streaming = request
                            .headers()
                            .get("content-type")
                            .and_then(|h| h.to_str().ok())
                            .is_some_and(|h| h.starts_with("application/connect+"));
                        error_response(
                            error,
                            streaming,
                            Codec::from_headers(request.headers(), streaming).unwrap_or_default(),
                        )
                    }
                }
            }
        },
    ))
}

fn deadline(headers: &HeaderMap) -> Result<Option<Instant>, ConnectError> {
    let Some(value) = headers.get("connect-timeout-ms") else {
        return Ok(None);
    };
    let value = value
        .to_str()
        .map_err(|_| ConnectError::new(Code::InvalidArgument, "invalid timeout"))?;
    if value.is_empty() || value.len() > 10 || !value.bytes().all(|c| c.is_ascii_digit()) {
        return Err(ConnectError::new(Code::InvalidArgument, "invalid timeout"));
    }
    let millis: u64 = value.parse().map_err(ConnectError::internal)?;
    if millis == 0 {
        return Err(ConnectError::deadline());
    }
    Ok(Some(Instant::now() + Duration::from_millis(millis)))
}

async fn within<F: Future>(
    deadline: Option<Instant>,
    future: F,
) -> Result<F::Output, ConnectError> {
    match deadline {
        Some(deadline) => timeout_at(deadline, future)
            .await
            .map_err(|_| ConnectError::deadline()),
        None => Ok(future.await),
    }
}

async fn decode_request<I: Message + Default + DeserializeOwned>(
    request: HttpRequest,
    codec: Codec,
    streaming: bool,
) -> Result<Request<I>, ConnectError> {
    check_version(request.headers())?;
    codec::check_encoding(request.headers(), streaming)?;
    let (parts, body) = request.into_parts();
    let bytes = to_bytes(body, MAX_MESSAGE_SIZE + if streaming { 5 } else { 0 })
        .await
        .map_err(|e| ConnectError::new(Code::ResourceExhausted, e.to_string()))?;
    let message = if streaming {
        let mut buffer = BytesMut::from(bytes.as_ref());
        let (flag, bytes) = codec::take_envelope(&mut buffer)?.ok_or_else(|| {
            ConnectError::new(Code::InvalidArgument, "incomplete request envelope")
        })?;
        if flag != 0 || !buffer.is_empty() {
            return Err(ConnectError::new(
                Code::InvalidArgument,
                "expected exactly one request message",
            ));
        }
        codec.decode(&bytes)?
    } else {
        codec.decode(&bytes)?
    };
    Ok(Request {
        message,
        metadata: parts.headers,
        transport: parts.extensions.get::<Transport>().copied(),
        timeout: None,
        extensions: parts.extensions,
    })
}

fn check_version(headers: &HeaderMap) -> Result<(), ConnectError> {
    if let Some(version) = headers.get("connect-protocol-version") {
        if version != "1" {
            return Err(ConnectError::new(
                Code::InvalidArgument,
                "unsupported Connect protocol version",
            ));
        }
    }
    Ok(())
}

fn response(body: Body, headers: HeaderMap, codec: Codec, streaming: bool) -> HttpResponse {
    let mut response = HttpResponse::new(body);
    *response.headers_mut() = headers;
    response.headers_mut().insert(
        "content-type",
        codec.content_type(streaming).parse().unwrap(),
    );
    response.headers_mut().remove("content-length");
    response.headers_mut().remove("content-encoding");
    response.headers_mut().remove("connect-content-encoding");
    response
}

fn error_response(error: ConnectError, streaming: bool, codec: Codec) -> HttpResponse {
    if streaming {
        let metadata = error.metadata.as_ref().clone();
        response(
            Body::from(EndStream::from_error(Some(error)).encode()),
            metadata,
            codec,
            true,
        )
    } else {
        let mut result = response(
            Body::from(serde_json::to_vec(&error).unwrap()),
            error.metadata.as_ref().clone(),
            Codec::Json,
            false,
        );
        *result.status_mut() = error.code.http_status();
        result
    }
}

fn unsupported_content_type(error: ConnectError) -> HttpResponse {
    let mut response = error_response(error, false, Codec::Json);
    *response.status_mut() = StatusCode::UNSUPPORTED_MEDIA_TYPE;
    response
}

pub async fn unary<I, O, F, Fut>(request: HttpRequest, handler: F) -> HttpResponse
where
    I: Message + Default + DeserializeOwned,
    O: Message + Serialize,
    F: FnOnce(Request<I>) -> Fut,
    Fut: Future<Output = Result<Response<O>, ConnectError>>,
{
    let codec = match Codec::from_headers(request.headers(), false) {
        Ok(codec) => codec,
        Err(e) => return unsupported_content_type(e),
    };
    let result = async {
        let deadline = deadline(request.headers())?;
        within(deadline, async {
            handler(decode_request(request, codec, false).await?).await
        })
        .await?
    }
    .await;
    match result {
        Ok(reply) => match codec.encode(&reply.message) {
            Ok(bytes) => response(Body::from(bytes), reply.metadata, codec, false),
            Err(error) => error_response(error, false, codec),
        },
        Err(error) => error_response(error, false, codec),
    }
}

pub async fn server_streaming<I, O, F, Fut>(request: HttpRequest, handler: F) -> HttpResponse
where
    I: Message + Default + DeserializeOwned,
    O: Message + Serialize + Send + 'static,
    F: FnOnce(Request<I>) -> Fut,
    Fut: Future<Output = Result<Response<Streaming<O>>, ConnectError>>,
{
    let codec = match Codec::from_headers(request.headers(), true) {
        Ok(codec) => codec,
        Err(e) => return unsupported_content_type(e),
    };
    let deadline = match deadline(request.headers()) {
        Ok(value) => value,
        Err(e) => return error_response(e, true, codec),
    };
    let result = within(deadline, async {
        handler(decode_request(request, codec, true).await?).await
    })
    .await;
    let reply = match result {
        Ok(Ok(reply)) => reply,
        Ok(Err(e)) | Err(e) => return error_response(e, true, codec),
    };
    let mut stream = reply.message;
    let body = async_stream::stream! {
        let error = loop {
            match within(deadline, stream.next()).await {
                Ok(Some(Ok(message))) => match codec.encode(&message) {
                    Ok(bytes) => yield Ok::<_, Infallible>(codec::envelope(0, &bytes)),
                    Err(error) => break Some(error),
                },
                Ok(Some(Err(error))) | Err(error) => break Some(error),
                Ok(None) => break None,
            }
        };
        yield Ok(EndStream::from_error(error).encode());
    };
    response(Body::from_stream(body), reply.metadata, codec, true)
}

fn spawn_connection<S>(
    tasks: &mut JoinSet<()>,
    io: S,
    router: Router,
    transport: Transport,
    mut shutdown: watch::Receiver<bool>,
) where
    S: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin + Send + 'static,
{
    tasks.spawn(async move {
        let service = tower::service_fn(move |mut request: http::Request<hyper::body::Incoming>| {
            request.extensions_mut().insert(transport);
            router.clone().oneshot(request)
        });
        let builder = Builder::new(TokioExecutor::new());
        let connection = builder.serve_connection(TokioIo::new(io), TowerToHyperService::new(service));
        tokio::pin!(connection);
        tokio::select! {
            _ = &mut connection => {},
            _ = shutdown.changed() => { connection.as_mut().graceful_shutdown(); let _ = connection.await; }
        }
    });
}

/// Serve HTTP/1.1 and cleartext HTTP/2; stop accepting and drain on shutdown.
pub async fn serve_tcp(
    listener: TcpListener,
    router: Router,
    shutdown: impl Future<Output = ()>,
) -> io::Result<()> {
    let (sender, receiver) = watch::channel(false);
    let mut tasks = JoinSet::new();
    tokio::pin!(shutdown);
    let result = loop {
        tokio::select! {
            _ = &mut shutdown => break Ok(()),
            incoming = listener.accept() => match incoming {
                Ok((io, _)) => spawn_connection(&mut tasks, io, router.clone(), Transport::Tcp, receiver.clone()),
                Err(error) => break Err(error),
            },
            _ = tasks.join_next(), if !tasks.is_empty() => {},
        }
    };
    let _ = sender.send(true);
    while tasks.join_next().await.is_some() {}
    result
}

pub enum UnixListenerSource {
    Path(PathBuf),
    Listener(UnixListener),
}

impl From<UnixListener> for UnixListenerSource {
    fn from(value: UnixListener) -> Self {
        Self::Listener(value)
    }
}
impl From<PathBuf> for UnixListenerSource {
    fn from(value: PathBuf) -> Self {
        Self::Path(value)
    }
}
impl From<&std::path::Path> for UnixListenerSource {
    fn from(value: &std::path::Path) -> Self {
        Self::Path(value.to_owned())
    }
}
impl From<&str> for UnixListenerSource {
    fn from(value: &str) -> Self {
        Self::Path(value.into())
    }
}

/// Serve a caller-owned Unix listener, or bind a new path (never unlinking an existing socket).
pub async fn serve_unix(
    source: impl Into<UnixListenerSource>,
    router: Router,
    shutdown: impl Future<Output = ()>,
) -> io::Result<()> {
    let listener = match source.into() {
        UnixListenerSource::Path(path) => UnixListener::bind(path)?,
        UnixListenerSource::Listener(listener) => listener,
    };
    let (sender, receiver) = watch::channel(false);
    let mut tasks = JoinSet::new();
    tokio::pin!(shutdown);
    let result = loop {
        tokio::select! {
            _ = &mut shutdown => break Ok(()),
            incoming = listener.accept() => match incoming {
                Ok((io, _)) => spawn_connection(&mut tasks, io, router.clone(), Transport::Unix, receiver.clone()),
                Err(error) => break Err(error),
            },
            _ = tasks.join_next(), if !tasks.is_empty() => {},
        }
    };
    let _ = sender.send(true);
    while tasks.join_next().await.is_some() {}
    result
}
