use crate::{
    config::ConfigStore,
    context::{resolve_context_name, ContextManager, EndpointContext},
    credentials::SystemProvider,
    paths::UserInfo,
    Result,
};
use construct_connect::{Channel, ClientConfig, Endpoint};
use construct_proto::construct::v1::*;
use std::{
    io::{Read, Write},
    sync::{Arc, OnceLock},
};

pub struct ApiClient {
    pub model_provider: ModelProviderServiceClient,
    pub model: ModelServiceClient,
    pub agent: AgentServiceClient,
    pub task: TaskServiceClient,
    pub message: MessageServiceClient,
    pub auth: AuthServiceClient,
    pub skill: SkillServiceClient,
    pub event: EventServiceClient,
}
impl ApiClient {
    pub fn new(context: &EndpointContext, manager: &ContextManager) -> Result<Self> {
        let endpoint = if context.kind == "unix" {
            Endpoint::Unix {
                path: context.address.clone().into(),
                prefix: "api".into(),
            }
        } else {
            Endpoint::Http(format!("{}/api", context.address.trim_end_matches('/')))
        };
        let mut config = ClientConfig::new(endpoint);
        if let Some(auth) = context.auth.as_ref().filter(|a| a.is_configured()) {
            let token = manager.resolve_token(auth).map_err(|e| {
                format!("failed to configure client: failed to resolve auth token: {e}")
            })?;
            let mut header: construct_connect::http::HeaderValue =
                format!("Bearer {token}").parse()?;
            header.set_sensitive(true);
            config.default_headers.insert("authorization", header);
        }
        let channel =
            Channel::new(config).map_err(|e| format!("failed to create api client: {e}"))?;
        Ok(Self {
            model_provider: ModelProviderServiceClient::new(channel.clone()),
            model: ModelServiceClient::new(channel.clone()),
            agent: AgentServiceClient::new(channel.clone()),
            task: TaskServiceClient::new(channel.clone()),
            message: MessageServiceClient::new(channel.clone()),
            auth: AuthServiceClient::new(channel.clone()),
            skill: SkillServiceClient::new(channel.clone()),
            event: EventServiceClient::new(channel),
        })
    }
}
pub const NO_CONTEXT: &str = "no context configured\n\nTo get started:\n  • Run 'construct daemon run' to start the daemon (first-time setup)\n  • Or run 'construct context add' to configure a remote context";
pub struct App {
    pub user: UserInfo,
    pub config: ConfigStore,
    pub contexts: ContextManager,
    pub renderer: crate::render::Renderer,
    pub stdout: Box<dyn Write + Send>,
    pub stdin: Box<dyn Read + Send>,
    pub cancellation: tokio_util::sync::CancellationToken,
    client: OnceLock<ApiClient>,
    context_override: String,
}
impl App {
    pub fn new(user: UserInfo, context_override: String) -> Result<Self> {
        let contexts = ContextManager::new(&user, Arc::new(SystemProvider));
        let config = ConfigStore::load(&user)?;
        Ok(Self {
            user,
            config,
            contexts,
            renderer: crate::render::Renderer,
            stdout: Box::new(std::io::stdout()),
            stdin: Box::new(std::io::stdin()),
            cancellation: tokio_util::sync::CancellationToken::new(),
            client: OnceLock::new(),
            context_override,
        })
    }
    pub fn client(&self) -> Result<&ApiClient> {
        if let Some(client) = self.client.get() {
            return Ok(client);
        }
        let contexts = self.contexts.load()?;
        contexts.validate()?;
        let name = resolve_context_name(
            &self.context_override,
            std::env::var("CONSTRUCT_CONTEXT").ok().as_deref(),
            &contexts.current,
        );
        if name.is_empty() {
            return Err(NO_CONTEXT.into());
        }
        let context = contexts
            .contexts
            .get(&name)
            .ok_or_else(|| format!("context {name:?} not found"))?;
        let client = ApiClient::new(context, &self.contexts)?;
        let _ = self.client.set(client);
        Ok(self.client.get().unwrap())
    }
}
pub fn requires_context(top_level: &str) -> bool {
    !matches!(
        top_level,
        "info" | "help" | "update" | "daemon" | "config" | "context"
    )
}
