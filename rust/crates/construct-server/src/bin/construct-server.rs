//! Minimal API host for interoperability; no agent execution or daemon installation.
use construct_event::{EventHook, EventRouter};
use construct_secret::{load_or_generate, FileProvider};
use construct_server::{
    listener::{detect_provider, Provider},
    HandlerOptions, Server, StaticRuntime,
};
use construct_skill::{SkillManager, UserDirs};
use construct_store::SqliteStore;
use std::{path::PathBuf, sync::Arc};

struct Options {
    unix: String,
    http: String,
    data: PathBuf,
    db: Option<PathBuf>,
}
fn options() -> Result<Option<Options>, Box<dyn std::error::Error>> {
    let data = std::env::var_os("XDG_DATA_HOME")
        .map(PathBuf::from)
        .or_else(|| std::env::var_os("HOME").map(|home| PathBuf::from(home).join(".local/share")))
        .map(|base| base.join("construct"))
        .unwrap_or_default();
    let mut options = Options {
        unix: String::new(),
        http: String::new(),
        data,
        db: None,
    };
    let mut args = std::env::args().skip(1);
    while let Some(flag) = args.next() {
        if flag == "--help" || flag == "-h" {
            println!("construct-server [--listen-unix PATH | --listen-http HOST:PORT] [--data-dir PATH] [--db PATH] [--secret-provider file]\nWithout listener flags, uses systemd/launchd socket activation. No agent execution.");
            return Ok(None);
        }
        if ![
            "--listen-unix",
            "--listen-http",
            "--data-dir",
            "--db",
            "--secret-provider",
        ]
        .contains(&flag.as_str())
        {
            return Err(format!("unknown argument: {flag}").into());
        }
        let value = args
            .next()
            .ok_or_else(|| format!("{flag} requires a value"))?;
        match flag.as_str() {
            "--listen-unix" => options.unix = value,
            "--listen-http" => options.http = value,
            "--data-dir" => options.data = value.into(),
            "--db" => options.db = Some(value.into()),
            "--secret-provider" if value != "file" => {
                return Err("only the file secret provider is supported".into())
            }
            _ => {}
        }
    }
    if options.data.as_os_str().is_empty() {
        return Err("set HOME or XDG_DATA_HOME, or provide --data-dir".into());
    }
    Ok(Some(options))
}
#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let Some(options) = options()? else {
        return Ok(());
    };
    let provider: Arc<dyn Provider> = Arc::from(detect_provider(&options.http, &options.unix)?);
    std::fs::create_dir_all(&options.data)?;
    let encryption = Arc::new(load_or_generate(&FileProvider::new(
        options.data.join("secrets"),
    )?)?);
    let events = Arc::new(EventRouter::default());
    let store = Arc::new(
        SqliteStore::open_with_hooks(
            options
                .db
                .unwrap_or_else(|| options.data.join("construct.db")),
            vec![Arc::new(EventHook::new(events.clone()))],
        )
        .await?,
    );
    let skills = Arc::new(SkillManager::new(UserDirs {
        home: std::env::var_os("HOME").map(PathBuf::from),
        config_dir: options.data.join("config"),
    }));
    let runtime = Arc::new(StaticRuntime::new(store, encryption, events));
    let server =
        Server::from_provider(HandlerOptions::new(runtime, skills), provider.clone()).await?;
    let address = server.address()?;
    let context = provider.activation_type().endpoint_context(&address);
    let context =
        context.map(|c| serde_json::json!({"name": c.name, "kind": c.kind, "address": c.address}));
    println!(
        "{}",
        serde_json::json!({"activation": provider.activation_type().as_str(), "address": address, "context": context})
    );
    #[cfg(unix)]
    let mut terminate = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())?;
    server
        .serve(async {
            #[cfg(unix)]
            tokio::select! { _ = tokio::signal::ctrl_c() => {}, _ = terminate.recv() => {} }
            #[cfg(not(unix))]
            let _ = tokio::signal::ctrl_c().await;
        })
        .await?;
    Ok(())
}
