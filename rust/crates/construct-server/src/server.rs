use crate::{
    api_router,
    listener::{Listener, Provider},
    HandlerOptions,
};
use construct_connect::{
    axum::{routing::get, Router},
    serve_tcp, serve_unix,
};
use std::{future::Future, io, sync::Arc};

/// Top-level HTTP server, exposing `/api/` services and unauthenticated `/healthz`.
pub struct Server {
    router: Router,
    listener: Listener,
    provider: Option<Arc<dyn Provider>>,
    events: Arc<construct_event::EventRouter>,
}
impl Server {
    /// Build around an already acquired listener; its family determines authentication trust.
    pub fn new(opts: HandlerOptions, listener: Listener) -> Self {
        let events = opts.runtime.event_router();
        Self {
            router: Router::new()
                .nest("/api", api_router(opts))
                .route("/healthz", get(|| async { "{\"status\":\"ok\"}" })),
            listener,
            provider: None,
            events,
        }
    }
    /// Acquire a provider's listener and arrange provider cleanup after graceful shutdown.
    pub async fn from_provider(
        opts: HandlerOptions,
        provider: Arc<dyn Provider>,
    ) -> Result<Self, crate::listener::Error> {
        let listener = provider.create().await?;
        let mut server = Self::new(opts, listener);
        server.provider = Some(provider);
        Ok(server)
    }
    /// The listener address, including the assigned port when bound with port zero.
    pub fn address(&self) -> io::Result<String> {
        self.listener.address()
    }
    /// Serve until shutdown, close the runtime event router, and drain active requests.
    pub async fn serve(self, shutdown: impl Future<Output = ()>) -> io::Result<()> {
        let shutdown = async {
            shutdown.await;
            self.events.close();
        };
        let result = match self.listener {
            Listener::Unix(listener) => serve_unix(listener, self.router, shutdown).await,
            Listener::Tcp(listener) => serve_tcp(listener, self.router, shutdown).await,
        };
        if let Some(provider) = self.provider {
            result.and(provider.close().map_err(io::Error::other))
        } else {
            result
        }
    }
}
