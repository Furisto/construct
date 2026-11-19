use crate::{app::App, context::EndpointContext, Result};
use clap::Args;
use construct_agent::{Runtime, RuntimeOptions};
use construct_codeact::{host_tool, print};
use construct_core::store::Store;
use construct_event::EventRouter;
use construct_secret::{load_or_generate, Encryption, FileProvider, KeyringProvider};
use construct_server::{
    listener::{detect_provider, Provider, TcpProvider, UnixSocketProvider},
    AgentRuntime, HandlerOptions, Server,
};
use construct_skill::{SkillManager, UserDirs};
use construct_tools::{filesystem::*, system::ExecuteCommand, web::Fetch};
use futures_util::{stream::FuturesUnordered, StreamExt};
use sha2::{Digest, Sha256};
use std::sync::Arc;

#[derive(Args)]
pub struct Options {
    #[arg(long, value_name = "PATH")]
    listen_unix: Option<String>,
    #[arg(long, value_name = "HOST:PORT")]
    listen_http: Option<String>,
}

struct RuntimeAdapter(Arc<Runtime>);
impl AgentRuntime for RuntimeAdapter {
    fn store(&self) -> Arc<dyn Store> {
        self.0.store()
    }
    fn encryption(&self) -> Arc<Encryption> {
        self.0.encryption()
    }
    fn event_router(&self) -> Arc<EventRouter> {
        self.0.event_router()
    }
}

pub async fn run(app: &mut App, options: Options) -> Result<()> {
    let mut terminate = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())?;
    let mut interrupt = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::interrupt())?;
    let unix = options.listen_unix.as_deref().unwrap_or_default();
    let http = options.listen_http.as_deref().unwrap_or_default();
    let providers: Vec<Arc<dyn Provider>> = if !unix.is_empty() && !http.is_empty() {
        vec![
            Arc::new(UnixSocketProvider::new(unix)),
            Arc::new(TcpProvider::new(http)),
        ]
    } else {
        vec![Arc::from(detect_provider(http, unix)?)]
    };
    let data = app.user.data_dir()?;
    let encryption = Arc::new(
        if app.config.get("secret.provider").and_then(|v| v.as_str()) == Some("file") {
            load_or_generate(&FileProvider::new(data.join("secrets"))?)?
        } else {
            load_or_generate(&KeyringProvider)?
        },
    );
    let dirs = UserDirs {
        home: Some(app.user.home.clone()),
        config_dir: app.user.config.clone(),
    };
    let runtime = Arc::new(
        Runtime::open_sqlite(
            data.join("construct.db"),
            encryption,
            RuntimeOptions {
                tools: vec![
                    host_tool(Arc::new(CreateFile)),
                    host_tool(Arc::new(ReadFile)),
                    host_tool(Arc::new(EditFile)),
                    host_tool(Arc::new(ListFiles)),
                    host_tool(Arc::new(Grep)),
                    host_tool(Arc::new(FindFile)),
                    host_tool(Arc::new(ExecuteCommand)),
                    host_tool(Arc::new(Fetch::default())),
                    print(),
                ],
                skills: Some(construct_skill::Discoverer::new(dirs.clone())),
                ..Default::default()
            },
        )
        .await?,
    );
    let mut bound = Vec::new();
    let result = async {
        let listener = providers[0].create().await?;
        bound.push(providers[0].clone());
        let server = Server::new(
            HandlerOptions::new(
                Arc::new(RuntimeAdapter(runtime.clone())),
                Arc::new(SkillManager::new(dirs)),
            ),
            listener,
        );
        let address = server.address()?;
        let mut servers = vec![];
        if providers.len() == 2 {
            let listener = providers[1].create().await?;
            bound.push(providers[1].clone());
            servers.push(server.additional_listener(listener));
        }
        servers.push(server);
        let activation = providers[0].activation_type();
        if let Some(context) = activation.endpoint_context(&address) {
            let hash = Sha256::digest(address.as_bytes());
            let name = format!(
                "{}-{:02x}{:02x}{:02x}",
                activation.as_str(),
                hash[0],
                hash[1],
                hash[2]
            );
            app.contexts.upsert(
                &name,
                EndpointContext {
                    address: context.address,
                    kind: context.kind.into(),
                    auth: None,
                },
                true,
            )?;
        }
        writeln!(app.stdout, "🤖 Starting Agent Runtime...")?;
        let cancellation = app.cancellation.clone();
        let running = runtime.clone();
        let token = cancellation.clone();
        let mut agent = tokio::spawn(async move { running.run(token).await });
        runtime.wait_until_running().await;
        let mut serving: FuturesUnordered<_> = servers
            .into_iter()
            .map(|server| {
                let token = cancellation.clone();
                async move { server.serve(token.cancelled()).await }
            })
            .collect();
        let mut agent_result = None;
        let mut result = tokio::select! {
            _ = terminate.recv() => Ok(()),
            _ = interrupt.recv() => Ok(()),
            _ = cancellation.cancelled() => Ok(()),
            result = &mut agent => { agent_result = Some(result); Ok(()) },
            result = serving.next() => result.unwrap_or(Ok(())).map_err(crate::Error::from),
        };
        cancellation.cancel();
        let drain = async {
            while let Some(stopped) = serving.next().await {
                if let Err(error) = stopped {
                    result = Err(error.into());
                }
            }
        };
        tokio::join!(runtime.shutdown(), drain);
        match agent_result {
            Some(result) => result,
            None => agent.await,
        }??;
        result
    }
    .await;
    let mut result = result;
    for provider in bound {
        if let Err(error) = provider.close() {
            if result.is_ok() {
                result = Err(error.into());
            }
        }
    }
    result
}
