//! Connect protocol runtime used by the generated `construct-proto` services.
//!
//! Implement a generated async service trait with `#[construct_connect::async_trait]`,
//! then merge `TaskServiceServer::new(service).router()` into an axum `Router`.
//! Use `Router::nest("/api", router)` to match the Go daemon's prefix. Serve it
//! with [`serve_tcp`] or [`serve_unix`]. Traits are object-safe: servers accept
//! either an implementation with `new` or an `Arc<dyn Service>` with `from_arc`.
//!
//! Create a [`Channel`] with [`ClientConfig::new`] and pass it to a generated
//! `TaskServiceClient::new(channel)`. Calls accept [`Request`] and return
//! [`Response`], including headers; set `Request::timeout` for a per-call limit.
//!
//! Messages are limited to 16 MiB. Only identity compression is supported.
//! Response metadata contains leading headers; streaming errors also carry the
//! end-stream metadata in `ConnectError::metadata`. Successful end-stream metadata
//! is validated but not surfaced separately. Unix socket paths are caller-owned:
//! remove the socket file after the server exits before binding it again.

mod channel;
mod codec;
mod error;
mod server;

pub use async_trait::async_trait;
pub use axum::{self, Router};
pub use channel::{Channel, ClientConfig, Endpoint};
pub use codec::Codec;
pub use error::{Code, ConnectError};
pub use http;
pub use server::{
    serve_tcp, with_interceptor, with_request_interceptor, Interceptor, RequestInterceptor,
};
#[cfg(unix)]
pub use server::{serve_unix, UnixListenerSource};

use futures_util::Stream;
use http::HeaderMap;
use std::{pin::Pin, time::Duration};

pub type Streaming<T> = Pin<Box<dyn Stream<Item = Result<T, ConnectError>> + Send>>;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Transport {
    Tcp,
    Unix,
}

#[derive(Debug)]
pub struct Request<T> {
    pub message: T,
    pub metadata: HeaderMap,
    pub transport: Option<Transport>,
    pub timeout: Option<Duration>,
    /// Server-local request context inserted by request interceptors.
    pub extensions: http::Extensions,
}

impl<T> Request<T> {
    pub fn new(message: T) -> Self {
        Self {
            message,
            metadata: HeaderMap::new(),
            transport: None,
            timeout: None,
            extensions: http::Extensions::new(),
        }
    }
}

#[derive(Debug)]
pub struct Response<T> {
    pub message: T,
    pub metadata: HeaderMap,
}

impl<T> Response<T> {
    pub fn new(message: T) -> Self {
        Self {
            message,
            metadata: HeaderMap::new(),
        }
    }
}

/// Entry points used by generated service handlers.
#[doc(hidden)]
pub mod generated {
    pub use crate::server::{server_streaming, unary};
}
