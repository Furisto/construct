use crate::{
    codec::{self, EndStream, MAX_MESSAGE_SIZE},
    Code, Codec, ConnectError, Request, Response, Streaming,
};
use bytes::{Bytes, BytesMut};
use futures_util::{future::BoxFuture, StreamExt};
use http::{HeaderMap, Uri};
use http_body_util::{BodyExt, Full};
use hyper::{
    body::Incoming,
    rt::{Read, ReadBufCursor, Write},
};
use hyper_rustls::{HttpsConnector, HttpsConnectorBuilder};
use hyper_util::{
    client::legacy::{
        connect::{Connected, Connection, HttpConnector},
        Client,
    },
    rt::{TokioExecutor, TokioIo},
};
use prost::Message;
use serde::{de::DeserializeOwned, Serialize};
use std::{
    error::Error,
    future::Future,
    io,
    path::PathBuf,
    pin::Pin,
    sync::Arc,
    task::{Context, Poll},
    time::Duration,
};
use tokio::{
    net::UnixStream,
    time::{timeout_at, Instant},
};
use tower_service::Service;

type BoxError = Box<dyn Error + Send + Sync>;

#[derive(Debug, Clone)]
pub enum Endpoint {
    Http(String),
    Unix { path: PathBuf, prefix: String },
}

impl From<&str> for Endpoint {
    fn from(value: &str) -> Self {
        Self::Http(value.to_owned())
    }
}
impl From<String> for Endpoint {
    fn from(value: String) -> Self {
        Self::Http(value)
    }
}

#[derive(Debug, Clone)]
pub struct ClientConfig {
    pub endpoint: Endpoint,
    pub codec: Codec,
    pub default_headers: HeaderMap,
    pub timeout: Option<Duration>,
}

impl ClientConfig {
    pub fn new(endpoint: impl Into<Endpoint>) -> Self {
        Self {
            endpoint: endpoint.into(),
            codec: Codec::Proto,
            default_headers: HeaderMap::new(),
            timeout: None,
        }
    }
}

#[derive(Clone)]
pub struct Channel {
    client: Client<Connector, Full<Bytes>>,
    base: Arc<str>,
    config: Arc<ClientConfig>,
}

impl Channel {
    pub fn new(config: ClientConfig) -> Result<Self, ConnectError> {
        let (base, unix) = match &config.endpoint {
            Endpoint::Http(base) => (base.trim_end_matches('/').to_owned(), None),
            Endpoint::Unix { path, prefix } => {
                let prefix = prefix.trim_matches('/');
                (
                    if prefix.is_empty() {
                        "http://unix".into()
                    } else {
                        format!("http://unix/{prefix}")
                    },
                    Some(path.clone()),
                )
            }
        };
        let uri: Uri = base.parse().map_err(|e| {
            ConnectError::new(Code::InvalidArgument, format!("invalid endpoint: {e}"))
        })?;
        if !matches!(uri.scheme_str(), Some("http" | "https"))
            || uri.authority().is_none()
            || uri.query().is_some()
        {
            return Err(ConnectError::new(
                Code::InvalidArgument,
                "endpoint must be an http(s) URL without a query",
            ));
        }
        let https = HttpsConnectorBuilder::new()
            .with_webpki_roots()
            .https_or_http()
            .enable_http1()
            .enable_http2()
            .build();
        Ok(Self {
            client: Client::builder(TokioExecutor::new()).build(Connector { https, unix }),
            base: base.into(),
            config: Arc::new(config),
        })
    }

    async fn start<I: Message + Serialize>(
        &self,
        path: &str,
        request: Request<I>,
        streaming: bool,
    ) -> Result<(http::Response<Incoming>, Option<Instant>), ConnectError> {
        let timeout = request.timeout.or(self.config.timeout);
        if timeout.is_some_and(|value| value.as_millis() > 9_999_999_999) {
            return Err(ConnectError::new(
                Code::InvalidArgument,
                "timeout exceeds 10 digits",
            ));
        }
        let deadline = timeout.map(|duration| Instant::now() + duration);
        if timeout == Some(Duration::ZERO) {
            return Err(ConnectError::deadline());
        }
        let bytes = self.config.codec.encode(&request.message)?;
        let bytes = if streaming {
            codec::envelope(0, &bytes)
        } else {
            bytes
        };
        let mut http_request = http::Request::builder()
            .method("POST")
            .uri(format!("{}{}", self.base, path))
            .body(Full::new(bytes))
            .map_err(ConnectError::internal)?;
        *http_request.headers_mut() = self.config.default_headers.clone();
        http_request.headers_mut().extend(request.metadata);
        http_request.headers_mut().insert(
            "content-type",
            self.config.codec.content_type(streaming).parse().unwrap(),
        );
        http_request
            .headers_mut()
            .insert("connect-protocol-version", "1".parse().unwrap());
        http_request.headers_mut().remove("content-length");
        if let Some(timeout) = timeout {
            let millis = timeout.as_millis().max(1);
            http_request
                .headers_mut()
                .insert("connect-timeout-ms", millis.to_string().parse().unwrap());
        }
        let response = within(deadline, self.client.request(http_request))
            .await?
            .map_err(|e| ConnectError::new(Code::Unavailable, e.to_string()))?;
        codec::check_encoding(response.headers(), streaming)?;
        if response.status() != http::StatusCode::OK {
            let (parts, body) = response.into_parts();
            let bytes = within(
                deadline,
                axum::body::to_bytes(axum::body::Body::new(body), MAX_MESSAGE_SIZE),
            )
            .await?
            .map_err(ConnectError::internal)?;
            let mut error = serde_json::from_slice::<ConnectError>(&bytes)
                .unwrap_or_else(|_| ConnectError::from_status(parts.status));
            *error.metadata = parts.headers;
            return Err(error);
        }
        let codec = Codec::from_headers(response.headers(), streaming)?;
        if codec != self.config.codec {
            return Err(ConnectError::new(
                Code::Internal,
                "response codec does not match request",
            ));
        }
        Ok((response, deadline))
    }

    pub async fn unary<I, O>(
        &self,
        path: &str,
        request: Request<I>,
    ) -> Result<Response<O>, ConnectError>
    where
        I: Message + Serialize,
        O: Message + Default + DeserializeOwned,
    {
        let (response, deadline) = self.start(path, request, false).await?;
        let (parts, body) = response.into_parts();
        let bytes = within(
            deadline,
            axum::body::to_bytes(axum::body::Body::new(body), MAX_MESSAGE_SIZE),
        )
        .await?
        .map_err(ConnectError::internal)?;
        let message = self
            .config
            .codec
            .decode(&bytes)
            .map_err(ConnectError::internal)?;
        Ok(Response {
            message,
            metadata: parts.headers,
        })
    }

    pub async fn server_streaming<I, O>(
        &self,
        path: &str,
        request: Request<I>,
    ) -> Result<Response<Streaming<O>>, ConnectError>
    where
        I: Message + Serialize,
        O: Message + Default + DeserializeOwned + Send + 'static,
    {
        let (response, deadline) = self.start(path, request, true).await?;
        let (parts, body) = response.into_parts();
        let codec = self.config.codec;
        let leading_metadata = parts.headers.clone();
        let stream = async_stream::try_stream! {
            let mut data = body.into_data_stream();
            let mut buffer = BytesMut::new();
            loop {
                while let Some((flag, bytes)) = codec::take_envelope(&mut buffer)? {
                    if flag == 2 {
                        if !buffer.is_empty() { Err(ConnectError::new(Code::Internal, "data after end-stream envelope"))?; }
                        let end: EndStream = serde_json::from_slice(&bytes).map_err(ConnectError::internal)?;
                        if let Some(mut error) = end.into_error()? {
                            let mut metadata = leading_metadata.clone();
                            for (name, value) in error.metadata.iter() { metadata.append(name.clone(), value.clone()); }
                            *error.metadata = metadata;
                            Err(error)?;
                        }
                        return;
                    }
                    yield codec.decode::<O>(&bytes).map_err(ConnectError::internal)?;
                }
                match within(deadline, data.next()).await? {
                    Some(Ok(bytes)) => buffer.extend_from_slice(&bytes),
                    Some(Err(error)) => Err(ConnectError::new(Code::Unavailable, error.to_string()))?,
                    None => Err(ConnectError::new(Code::Internal, "missing or incomplete end-stream envelope"))?,
                }
            }
        };
        Ok(Response {
            message: Box::pin(stream),
            metadata: parts.headers,
        })
    }
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

#[derive(Clone)]
struct Connector {
    https: HttpsConnector<HttpConnector>,
    unix: Option<PathBuf>,
}

enum ConnectedIo {
    Http(Box<<HttpsConnector<HttpConnector> as Service<Uri>>::Response>),
    Unix(TokioIo<UnixStream>),
}

impl Service<Uri> for Connector {
    type Response = ConnectedIo;
    type Error = BoxError;
    type Future = BoxFuture<'static, Result<Self::Response, Self::Error>>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        self.https.poll_ready(cx)
    }

    fn call(&mut self, uri: Uri) -> Self::Future {
        if let Some(path) = self.unix.clone() {
            Box::pin(async move {
                Ok(ConnectedIo::Unix(TokioIo::new(
                    UnixStream::connect(path).await?,
                )))
            })
        } else {
            let future = self.https.call(uri);
            Box::pin(async move { future.await.map(|io| ConnectedIo::Http(Box::new(io))) })
        }
    }
}

impl Connection for ConnectedIo {
    fn connected(&self) -> Connected {
        match self {
            Self::Http(io) => io.connected(),
            Self::Unix(_) => Connected::new(),
        }
    }
}

impl Read for ConnectedIo {
    fn poll_read(
        self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: ReadBufCursor<'_>,
    ) -> Poll<io::Result<()>> {
        match self.get_mut() {
            Self::Http(io) => Pin::new(io).poll_read(cx, buf),
            Self::Unix(io) => Pin::new(io).poll_read(cx, buf),
        }
    }
}

impl Write for ConnectedIo {
    fn poll_write(
        self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &[u8],
    ) -> Poll<io::Result<usize>> {
        match self.get_mut() {
            Self::Http(io) => Pin::new(io).poll_write(cx, buf),
            Self::Unix(io) => Pin::new(io).poll_write(cx, buf),
        }
    }
    fn poll_flush(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        match self.get_mut() {
            Self::Http(io) => Pin::new(io).poll_flush(cx),
            Self::Unix(io) => Pin::new(io).poll_flush(cx),
        }
    }
    fn poll_shutdown(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        match self.get_mut() {
            Self::Http(io) => Pin::new(io).poll_shutdown(cx),
            Self::Unix(io) => Pin::new(io).poll_shutdown(cx),
        }
    }
}
