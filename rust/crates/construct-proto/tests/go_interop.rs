//! Opt-in real-binary interop. Run with `rust/scripts/go-interop.sh`.
//! TCP uses the daemon's Linux socket-activation path: --listen-http currently
//! fails before listening because it saves context kind "tcp" but validates "http".
#![cfg(target_os = "linux")]

use construct_connect::{
    async_trait, serve_unix, Channel, ClientConfig, Code, Codec, ConnectError, Endpoint, Request,
    Response, Router, Transport,
};
use construct_proto::construct::v1::*;
use futures_util::StreamExt;
use std::{
    ffi::OsStr,
    fs::{self, File},
    net::TcpListener,
    os::fd::AsRawFd,
    path::{Path, PathBuf},
    sync::{
        atomic::{AtomicUsize, Ordering},
        Arc,
    },
    time::Duration,
};
use tempfile::TempDir;
use tokio::{
    net::UnixListener,
    process::{Child, Command},
    sync::oneshot,
    time::{sleep, timeout, Instant},
};

fn go_binary() -> Option<PathBuf> {
    let Some(path) = std::env::var_os("CONSTRUCT_GO_BIN") else {
        eprintln!(
            "SKIP real Go interop: CONSTRUCT_GO_BIN is not set (run rust/scripts/go-interop.sh)"
        );
        return None;
    };
    Some(fs::canonicalize(path).expect("CONSTRUCT_GO_BIN must point to a built construct binary"))
}

struct Sandbox(TempDir);

impl Sandbox {
    fn new() -> Self {
        let home = tempfile::Builder::new()
            .prefix("construct-interop-")
            .tempdir()
            .unwrap();
        for dir in [
            "config/construct",
            "data",
            "cache",
            "state",
            "runtime",
            "tmp",
        ] {
            fs::create_dir_all(home.path().join(dir)).unwrap();
        }
        fs::write(
            home.path().join("config/construct/config.yaml"),
            "secret:\n  provider: file\n",
        )
        .unwrap();
        Self(home)
    }

    fn path(&self) -> &Path {
        self.0.path()
    }

    fn command(&self, binary: &Path) -> Command {
        let mut command = Command::new(binary);
        command
            .env_clear()
            .env("PATH", "/usr/local/bin:/usr/bin:/bin")
            .env("HOME", self.path())
            .env("TMPDIR", self.path().join("tmp"))
            .env("XDG_CONFIG_HOME", self.path().join("config"))
            .env("XDG_DATA_HOME", self.path().join("data"))
            .env("XDG_CACHE_HOME", self.path().join("cache"))
            .env("XDG_STATE_HOME", self.path().join("state"))
            .env("XDG_RUNTIME_DIR", self.path().join("runtime"))
            // Keep any telemetry from a caller-supplied release binary off the network.
            .env("HTTP_PROXY", "http://127.0.0.1:9")
            .env("HTTPS_PROXY", "http://127.0.0.1:9")
            .env("NO_PROXY", "localhost,127.0.0.1,::1")
            .kill_on_drop(true);
        command
    }
}

struct Daemon {
    child: Child,
    log: PathBuf,
    endpoint: Endpoint,
}

impl Daemon {
    async fn start(binary: &Path, sandbox: &Sandbox, unix: bool) -> Self {
        let socket = sandbox.path().join("construct.sock");
        let log = sandbox.path().join(if unix {
            "daemon-unix.log"
        } else {
            "daemon-tcp.log"
        });
        let stdout = File::create(&log).unwrap();
        let activated = (!unix).then(|| TcpListener::bind("127.0.0.1:0").unwrap());
        let mut command = if let Some(listener) = &activated {
            eprintln!(
                "[go-daemon] TCP via Linux socket activation on {} (--listen-http rejects context kind tcp)",
                listener.local_addr().unwrap()
            );
            socket_activated_command(binary, sandbox, listener)
        } else {
            let mut command = sandbox.command(binary);
            command.args(["daemon", "run", "--log-level", "debug"]);
            command.arg("--listen-unix").arg(&socket);
            command
        };
        let child = command
            .stdout(stdout.try_clone().unwrap())
            .stderr(stdout)
            .spawn()
            .unwrap();
        drop(activated);
        let mut daemon = Self {
            child,
            log,
            endpoint: Endpoint::Unix {
                path: socket.clone(),
                prefix: "/api".into(),
            },
        };
        let until = Instant::now() + Duration::from_secs(30);
        loop {
            if let Some(status) = daemon.child.try_wait().unwrap() {
                panic!(
                    "Go daemon exited during startup: {status}\n{}",
                    daemon.logs()
                );
            }
            // The daemon reports the actual bound address, so TCP can use port zero
            // without reserving and releasing a port in the test process.
            let configured = daemon
                .logs()
                .lines()
                .filter_map(|line| serde_json::from_str::<serde_json::Value>(line).ok())
                .find(|entry| entry["msg"] == "API server configured");
            if let Some(entry) = configured {
                if unix {
                    assert!(socket.exists());
                } else {
                    daemon.endpoint = Endpoint::Http(format!(
                        "http://{}/api",
                        entry["listener_address"].as_str().unwrap()
                    ));
                }
                eprintln!(
                    "[go-daemon] pid={} bound {:?}; isolated HOME={}; secret.provider=file",
                    daemon.child.id().unwrap(),
                    daemon.endpoint,
                    sandbox.path().display()
                );
                eprintln!("[go-daemon] {entry}");
                break;
            }
            assert!(
                Instant::now() < until,
                "Go startup timed out\n{}",
                daemon.logs()
            );
            sleep(Duration::from_millis(25)).await;
        }
        daemon
    }

    fn logs(&self) -> String {
        fs::read_to_string(&self.log).unwrap_or_default()
    }

    async fn stop(mut self) {
        let pid = self.child.id().unwrap().to_string();
        assert!(Command::new("kill")
            .args(["-TERM", &pid])
            .status()
            .await
            .unwrap()
            .success());
        let status = timeout(Duration::from_secs(12), self.child.wait())
            .await
            .expect("Go daemon shutdown timed out")
            .unwrap();
        assert!(
            status.success(),
            "Go daemon shutdown failed: {status}\n{}",
            self.logs()
        );
        eprintln!("[go-daemon] pid={pid} stopped: {status}");
    }
}

fn socket_activated_command(binary: &Path, sandbox: &Sandbox, listener: &TcpListener) -> Command {
    let mut command = sandbox.command(Path::new("/bin/sh"));
    command
        .args([
            "-c",
            "export LISTEN_PID=$$; exec \"$@\"",
            "socket-activation",
        ])
        .arg(binary)
        .args(["daemon", "run", "--log-level", "debug"])
        .env("LISTEN_FDS", "1");
    let fd = listener.as_raw_fd();
    // Only async-signal-safe syscalls run between fork and exec. The parent
    // owns the listener until spawn completes; FD 3 must survive exec.
    unsafe {
        command.pre_exec(move || {
            if libc::dup2(fd, 3) == -1 || libc::fcntl(3, libc::F_SETFD, 0) == -1 {
                return Err(std::io::Error::last_os_error());
            }
            Ok(())
        });
    }
    command
}

impl Drop for Daemon {
    fn drop(&mut self) {
        if std::thread::panicking() {
            eprintln!("[go-daemon] failure diagnostics:\n{}", self.logs());
        }
    }
}

fn channel(endpoint: Endpoint, codec: Codec, token: Option<&str>) -> Channel {
    let mut config = ClientConfig::new(endpoint);
    config.codec = codec;
    config.timeout = Some(Duration::from_secs(10));
    if let Some(token) = token {
        let mut value = format!("Bearer {token}")
            .parse::<construct_connect::http::HeaderValue>()
            .unwrap();
        value.set_sensitive(true);
        config.default_headers.insert("authorization", value);
    }
    Channel::new(config).unwrap()
}

async fn create_agent(client: &AgentServiceClient, model_id: &str, name: String) -> Agent {
    client
        .create_agent(Request::new(CreateAgentRequest {
            name,
            model_id: model_id.into(),
            instructions: "Interop test only. Do not run model calls.".into(),
            description: "Created by Rust against the real Go daemon".into(),
        }))
        .await
        .unwrap()
        .message
        .agent
        .unwrap()
}

async fn exercise(channel: Channel, label: &str) {
    let providers = ModelProviderServiceClient::new(channel.clone())
        .list_model_providers(Request::new(ListModelProvidersRequest::default()))
        .await
        .unwrap()
        .message
        .model_providers;
    assert_eq!(providers.len(), 1);
    eprintln!("[{label}] ListModelProviders OK count={}", providers.len());
    let models = ModelServiceClient::new(channel.clone())
        .list_models(Request::new(ListModelsRequest::default()))
        .await
        .unwrap()
        .message
        .models;
    assert!(!models.is_empty());
    let model_id = &models[0].metadata.as_ref().unwrap().id;
    eprintln!(
        "[{label}] ListModels OK count={} (locally seeded; no model inference)",
        models.len()
    );
    let client = AgentServiceClient::new(channel.clone());
    let agents = client
        .list_agents(Request::new(ListAgentsRequest::default()))
        .await
        .unwrap()
        .message
        .agents;
    assert!(agents
        .iter()
        .any(|agent| agent.spec.as_ref().unwrap().name == "edit"));
    eprintln!(
        "[{label}] ListAgents OK count={} (includes seeded agents)",
        agents.len()
    );
    let tasks = TaskServiceClient::new(channel.clone())
        .list_tasks(Request::new(ListTasksRequest::default()))
        .await
        .unwrap()
        .message
        .tasks;
    assert!(tasks.is_empty());
    eprintln!("[{label}] ListTasks OK count=0");

    let missing = client
        .get_agent(Request::new(GetAgentRequest {
            id: uuid::Uuid::new_v4().to_string(),
        }))
        .await
        .unwrap_err();
    assert_eq!(missing.code, Code::NotFound);
    assert!(!missing.message.is_empty());
    eprintln!(
        "[{label}] GetAgent(random UUID) -> {}: {}",
        missing.code, missing.message
    );

    let name = format!("interop-{label}-crud");
    let agent = create_agent(&client, model_id, name.clone()).await;
    let id = agent.metadata.as_ref().unwrap().id.clone();
    let got = client
        .get_agent(Request::new(GetAgentRequest { id: id.clone() }))
        .await
        .unwrap()
        .message
        .agent
        .unwrap();
    assert_eq!(got.spec.unwrap().name, name);
    assert!(got.metadata.unwrap().created_at.is_some());
    client
        .delete_agent(Request::new(DeleteAgentRequest { id: id.clone() }))
        .await
        .unwrap();
    assert_eq!(
        client
            .get_agent(Request::new(GetAgentRequest { id }))
            .await
            .unwrap_err()
            .code,
        Code::NotFound
    );
    eprintln!("[{label}] CreateAgent/GetAgent/DeleteAgent OK (including Timestamp)");

    let events = EventServiceClient::new(channel);
    let receive = async {
        let mut response = events
            .subscribe(Request::new(EventSubscribeRequest {
                event_types: vec!["agent.created".into()],
                ..Default::default()
            }))
            .await
            .unwrap();
        response
            .message
            .next()
            .await
            .expect("Go stream ended before event")
            .unwrap()
            .event
            .unwrap()
    };
    tokio::pin!(receive);
    let mut created = Vec::new();
    let mut tick = tokio::time::interval(Duration::from_millis(50));
    // Go does not flush streaming response headers until its first message.
    // Publish concurrently, retrying bounded local CRUD until subscription is live.
    let event = timeout(Duration::from_secs(10), async {
        loop {
            tokio::select! {
                event = &mut receive => break event,
                _ = tick.tick(), if created.len() < 40 => {
                    let agent = create_agent(&client, model_id, format!("interop-{label}-event-{}", created.len())).await;
                    created.push(agent.metadata.unwrap().id);
                }
            }
        }
    }).await.expect("no live Go agent.created event within 10 seconds");
    assert_eq!(event.r#type, "agent.created");
    assert_eq!(event.action, EventAction::Created as i32);
    assert!(event.timestamp.is_some());
    let Some(event::Payload::Agent(payload)) = event.payload else {
        panic!("expected Agent payload");
    };
    let observed_id = payload.agent.unwrap().metadata.unwrap().id;
    assert!(created.contains(&observed_id));
    eprintln!("[{label}] EventService.Subscribe -> agent.created id={observed_id} (matches this test's CreateAgent)");
    for id in created {
        client
            .delete_agent(Request::new(DeleteAgentRequest { id }))
            .await
            .unwrap();
    }
}

async fn real_go_daemon(codec: Codec) {
    let Some(binary) = go_binary() else {
        return;
    };
    let sandbox = Sandbox::new();
    let daemon = Daemon::start(&binary, &sandbox, true).await;
    let unix_channel = channel(daemon.endpoint.clone(), codec, None);
    let provider = ModelProviderServiceClient::new(unix_channel.clone())
        .create_model_provider(Request::new(CreateModelProviderRequest {
            name: "interop-offline-provider".into(),
            provider_type: ModelProviderType::Anthropic as i32,
            authentication: Some(create_model_provider_request::Authentication::ApiKey(
                "interop-dummy-not-a-real-key".into(),
            )),
            url: Some("http://127.0.0.1:9".into()),
        }))
        .await
        .unwrap()
        .message
        .model_provider
        .unwrap();
    assert!(provider.metadata.unwrap().created_at.is_some());
    eprintln!(
        "[{codec:?}/unix] CreateModelProvider OK (dummy key, loopback URL, local catalogue only)"
    );
    exercise(unix_channel.clone(), &format!("{codec:?}/unix")).await;
    let response = AuthServiceClient::new(unix_channel)
        .create_token(Request::new(CreateTokenRequest {
            name: "interop-tcp".into(),
            expires_in: Some(pbjson_types::Duration {
                seconds: 300,
                nanos: 0,
            }),
            description: None,
        }))
        .await
        .unwrap()
        .message;
    assert!(!response.token.is_empty());
    assert!(response.expires_at.is_some());
    eprintln!("[{codec:?}/unix] AuthService.CreateToken OK (token redacted; 300s expiry)");
    assert!(
        sandbox.path().join("data/construct/secrets").is_dir(),
        "file secret provider must be active"
    );
    daemon.stop().await;

    let daemon = Daemon::start(&binary, &sandbox, false).await;
    let unauthenticated = AgentServiceClient::new(channel(daemon.endpoint.clone(), codec, None))
        .list_agents(Request::new(ListAgentsRequest::default()))
        .await
        .unwrap_err();
    assert_eq!(unauthenticated.code, Code::Unauthenticated);
    eprintln!("[{codec:?}/tcp] without Bearer -> unauthenticated (expected)");
    exercise(
        channel(daemon.endpoint.clone(), codec, Some(&response.token)),
        &format!("{codec:?}/tcp+Bearer"),
    )
    .await;
    daemon.stop().await;
}

#[tokio::test]
async fn rust_client_to_go_daemon_proto_unix_and_authenticated_tcp() {
    real_go_daemon(Codec::Proto).await;
}

#[tokio::test]
async fn rust_client_to_go_daemon_json_unix_and_authenticated_tcp() {
    real_go_daemon(Codec::Json).await;
}

struct StubAgents(Arc<AtomicUsize>);

#[async_trait]
impl AgentService for StubAgents {
    async fn list_agents(
        &self,
        request: Request<ListAgentsRequest>,
    ) -> Result<Response<ListAgentsResponse>, ConnectError> {
        assert_eq!(request.transport, Some(Transport::Unix));
        self.0.fetch_add(1, Ordering::SeqCst);
        Ok(Response::new(ListAgentsResponse::default()))
    }
}

async fn cli(sandbox: &Sandbox, binary: &Path, args: &[&OsStr]) -> String {
    let output = timeout(
        Duration::from_secs(15),
        sandbox.command(binary).args(args).output(),
    )
    .await
    .expect("Go CLI timed out")
    .unwrap();
    assert!(
        output.status.success(),
        "Go CLI failed: {}",
        String::from_utf8_lossy(&output.stderr)
    );
    String::from_utf8(output.stdout).unwrap()
}

#[tokio::test]
async fn go_cli_agent_list_to_rust_unix_server() {
    let Some(binary) = go_binary() else {
        return;
    };
    let sandbox = Sandbox::new();
    let socket = sandbox.path().join("rust.sock");
    let listener = UnixListener::bind(&socket).unwrap();
    let hits = Arc::new(AtomicUsize::new(0));
    let router = Router::new().nest(
        "/api",
        AgentServiceServer::new(StubAgents(hits.clone())).router(),
    );
    let (stop, shutdown) = oneshot::channel();
    let task = tokio::spawn(serve_unix(listener, router, async {
        let _ = shutdown.await;
    }));
    cli(
        &sandbox,
        &binary,
        &[
            "context".as_ref(),
            "add".as_ref(),
            "rust-stub".as_ref(),
            "--endpoint".as_ref(),
            socket.as_os_str(),
            "--set-current".as_ref(),
        ],
    )
    .await;
    let output = cli(
        &sandbox,
        &binary,
        &[
            "agent".as_ref(),
            "list".as_ref(),
            "--output".as_ref(),
            "json".as_ref(),
        ],
    )
    .await;
    assert_eq!(
        serde_json::from_str::<serde_json::Value>(&output).unwrap(),
        serde_json::json!([])
    );
    assert_eq!(hits.load(Ordering::SeqCst), 1);
    eprintln!("[Go CLI -> Rust/unix] construct agent list --output json -> []; Rust ListAgents handler hit exactly once");
    stop.send(()).unwrap();
    timeout(Duration::from_secs(5), task)
        .await
        .unwrap()
        .unwrap()
        .unwrap();
}
