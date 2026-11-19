use crate::{
    auth::{AuthInterceptor, TokenProvider},
    handlers::Services,
};
use construct_connect::{async_trait, with_request_interceptor, Router};
use construct_core::store::Store;
use construct_event::EventRouter;
use construct_proto::construct::v1 as pb;
use construct_secret::Encryption;
use construct_skill::{service::SkillHandler, SkillManager};
use std::sync::Arc;

/// Services required from the agent runtime by the API; no model execution is implied.
#[async_trait]
pub trait AgentRuntime: Send + Sync + 'static {
    /// Shared transactional repositories.
    fn store(&self) -> Arc<dyn Store>;
    /// Go-compatible encryption for provider credentials.
    fn encryption(&self) -> Arc<Encryption>;
    /// Live event publisher and subscriber.
    fn event_router(&self) -> Arc<EventRouter>;
}

/// Runtime assembled from existing implementations until agent execution is ported.
pub struct StaticRuntime {
    store: Arc<dyn Store>,
    encryption: Arc<Encryption>,
    events: Arc<EventRouter>,
}
impl StaticRuntime {
    /// Assemble a runtime; SQLite callers should register an EventHook on the same router.
    pub fn new(
        store: Arc<dyn Store>,
        encryption: Arc<Encryption>,
        events: Arc<EventRouter>,
    ) -> Self {
        Self {
            store,
            encryption,
            events,
        }
    }
}
#[async_trait]
impl AgentRuntime for StaticRuntime {
    fn store(&self) -> Arc<dyn Store> {
        self.store.clone()
    }
    fn encryption(&self) -> Arc<Encryption> {
        self.encryption.clone()
    }
    fn event_router(&self) -> Arc<EventRouter> {
        self.events.clone()
    }
}

/// Nonblocking analytics emitted after successful handler mutations, matching Go Emit calls.
pub trait Analytics: Send + Sync + 'static {
    /// Agent creation.
    fn emit_agent_created(&self, agent_id: &str, agent_name: &str, model_name: &str);
    /// Agent configuration update.
    fn emit_agent_updated(&self, agent_id: &str, agent_name: &str, updated_fields: &[String]);
    /// Agent deletion.
    fn emit_agent_deleted(&self, agent_id: &str, agent_name: &str);
    /// Task creation.
    fn emit_task_created(&self, task_id: &str, agent_id: &str);
    /// Task configuration update.
    fn emit_task_updated(&self, task_id: &str, updated_fields: &[String]);
}
/// Analytics implementation that discards all events.
#[derive(Default)]
pub struct NoopAnalytics;
impl Analytics for NoopAnalytics {
    fn emit_agent_created(&self, _: &str, _: &str, _: &str) {}
    fn emit_agent_updated(&self, _: &str, _: &str, _: &[String]) {}
    fn emit_agent_deleted(&self, _: &str, _: &str) {}
    fn emit_task_created(&self, _: &str, _: &str) {}
    fn emit_task_updated(&self, _: &str, _: &[String]) {}
}
/// Dependencies for all eight API services. Mount the resulting router at `/api` if desired.
pub struct HandlerOptions {
    /// Shared runtime dependencies.
    pub runtime: Arc<dyn AgentRuntime>,
    /// Analytics sink.
    pub analytics: Arc<dyn Analytics>,
    /// Process-local token and setup code provider.
    pub token_provider: Arc<TokenProvider>,
    /// Installed skill manager.
    pub skills: Arc<SkillManager>,
}
impl HandlerOptions {
    /// Configure services with fresh setup code storage and disabled analytics.
    pub fn new(runtime: Arc<dyn AgentRuntime>, skills: Arc<SkillManager>) -> Self {
        Self {
            runtime,
            skills,
            analytics: Arc::new(NoopAnalytics),
            token_provider: Arc::new(TokenProvider::new()),
        }
    }
}
/// Wire all eight generated service routers behind transport-aware authentication.
pub fn api_router(opts: HandlerOptions) -> Router {
    let store = opts.runtime.store();
    let auth = Arc::new(AuthInterceptor::new(
        store.clone(),
        opts.token_provider.clone(),
    ));
    let h = Services {
        store,
        encryption: opts.runtime.encryption(),
        events: opts.runtime.event_router(),
        analytics: opts.analytics,
        tokens: opts.token_provider,
    };
    let router = Router::new()
        .merge(pb::AuthServiceServer::new(h.clone()).router())
        .merge(pb::ModelProviderServiceServer::new(h.clone()).router())
        .merge(pb::ModelServiceServer::new(h.clone()).router())
        .merge(pb::AgentServiceServer::new(h.clone()).router())
        .merge(pb::TaskServiceServer::new(h.clone()).router())
        .merge(pb::MessageServiceServer::new(h.clone()).router())
        .merge(pb::SkillServiceServer::new(SkillHandler::new(opts.skills)).router())
        .merge(pb::EventServiceServer::new(h).router());
    with_request_interceptor(router, auth)
}
