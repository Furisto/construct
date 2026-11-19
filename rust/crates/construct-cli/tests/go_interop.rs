#![cfg(target_os = "linux")]
use construct::{
    app::ApiClient,
    config::{parse_value, ConfigStore},
    context::{AuthConfig, ContextManager, EndpointContext},
    credentials::{MemoryProvider, Provider, SystemProvider},
    paths::UserInfo,
};
use construct_connect::Request;
use construct_proto::construct::v1::{
    create_model_provider_request, CreateModelProviderRequest, CreateTokenRequest,
    ListAgentsRequest, ModelProviderType,
};
use serde_json::{json, Value};
use std::{
    collections::BTreeMap,
    fs::{self, File},
    net::TcpListener,
    os::fd::AsRawFd,
    path::{Path, PathBuf},
    process::{Child, Command},
    sync::Arc,
    time::{Duration, Instant},
};
use tokio::io::{AsyncReadExt, AsyncWriteExt};

fn binary() -> Option<PathBuf> {
    let Some(path) = std::env::var_os("CONSTRUCT_GO_BIN") else {
        eprintln!("SKIP CLI Go interop: CONSTRUCT_GO_BIN not set");
        return None;
    };
    Some(fs::canonicalize(path).expect("CONSTRUCT_GO_BIN must be a built Go CLI"))
}
struct Sandbox {
    dir: tempfile::TempDir,
    user: UserInfo,
}
impl Sandbox {
    fn new() -> Self {
        let dir = tempfile::tempdir().unwrap();
        let mut env = BTreeMap::new();
        for (var, name) in [
            ("XDG_CONFIG_HOME", "config"),
            ("XDG_DATA_HOME", "data"),
            ("XDG_STATE_HOME", "state"),
            ("XDG_RUNTIME_DIR", "runtime"),
        ] {
            let path = dir.path().join(name);
            fs::create_dir_all(&path).unwrap();
            env.insert(var.into(), path.to_str().unwrap().into());
        }
        let user = UserInfo::resolve(dir.path(), "linux", "1000", &env);
        Self { dir, user }
    }
    fn command(&self, bin: &Path) -> Command {
        let mut cmd = Command::new(bin);
        cmd.env_clear()
            .env("PATH", "/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin")
            .env("HOME", self.dir.path())
            .env("XDG_CONFIG_HOME", self.user.config.parent().unwrap())
            .env("XDG_DATA_HOME", self.user.data.parent().unwrap())
            .env("XDG_STATE_HOME", self.user.log.parent().unwrap())
            .env("XDG_RUNTIME_DIR", self.user.runtime.parent().unwrap())
            .env("HTTP_PROXY", "http://127.0.0.1:9")
            .env("HTTPS_PROXY", "http://127.0.0.1:9")
            .env("NO_PROXY", "localhost,127.0.0.1,::1");
        cmd
    }
    fn go(&self, bin: &Path, args: &[&str]) -> String {
        let output = self.command(bin).args(args).output().unwrap();
        assert!(
            output.status.success(),
            "{}",
            String::from_utf8_lossy(&output.stderr)
        );
        String::from_utf8(output.stdout).unwrap()
    }
    fn manager(&self) -> ContextManager {
        ContextManager::new(&self.user, Arc::new(MemoryProvider::default()))
    }
}
#[test]
fn cli_files_are_readable_in_both_directions() {
    let Some(bin) = binary() else {
        return;
    };
    let sandbox = Sandbox::new();
    sandbox.go(
        &bin,
        &[
            "context",
            "add",
            "go",
            "--endpoint",
            "/tmp/go.sock",
            "--set-current",
        ],
    );
    sandbox.go(&bin, &["config", "set", "cmd.new.agent", "go-agent"]);
    let manager = sandbox.manager();
    assert_eq!(manager.load().unwrap().current, "go");
    assert_eq!(manager.get("go").unwrap().address, "/tmp/go.sock");
    let mut config = ConfigStore::load(&sandbox.user).unwrap();
    assert_eq!(
        config.get("cmd.new.agent").unwrap().as_str(),
        Some("go-agent")
    );
    manager
        .upsert(
            "rust",
            EndpointContext {
                address: "/tmp/rust.sock".into(),
                kind: "unix".into(),
                auth: None,
            },
            false,
        )
        .unwrap();
    manager.set_current("rust").unwrap();
    config
        .set("cmd.new.agent", parse_value("rust-agent"))
        .unwrap();
    config.flush().unwrap();
    assert_eq!(
        sandbox.go(&bin, &["config", "get", "cmd.new.agent"]),
        "rust-agent\n"
    );
    let list = sandbox.go(&bin, &["context", "list", "--output", "json"]);
    let list: serde_json::Value = serde_json::from_str(&list).unwrap();
    assert_eq!(list[1]["name"], "rust");
    assert_eq!(list[1]["current"], true);
    let bytes = fs::read(sandbox.user.config.join("context.yaml")).unwrap();
    sandbox.go(
        &bin,
        &[
            "context",
            "add",
            "rust",
            "--endpoint",
            "/tmp/rust.sock",
            "--set-current",
        ],
    );
    assert_eq!(
        bytes,
        fs::read(sandbox.user.config.join("context.yaml")).unwrap(),
        "context YAML bytes differ from Go"
    );
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        for name in ["context.yaml", "config.yaml"] {
            assert_eq!(
                fs::metadata(sandbox.user.config.join(name))
                    .unwrap()
                    .permissions()
                    .mode()
                    & 0o777,
                0o600
            );
        }
    }
    eprintln!("[cli/files] Go-written context/config read by Rust; Rust-written files read by Go; YAML bytes and 0600 verified");
}

#[derive(serde::Serialize)]
struct ContextDisplay {
    name: String,
    endpoint: String,
    kind: String,
    auth: String,
    current: bool,
}
impl construct::render::DisplayResource for ContextDisplay {
    fn fields(&self) -> Vec<construct::render::Field<'_>> {
        use construct::render::Field;
        vec![
            Field::new("Name", &self.name),
            Field::new("Endpoint", &self.endpoint),
            Field::new("Kind", &self.kind),
            Field::new("Auth", &self.auth),
            Field::new("Current", self.current),
        ]
    }
}
#[test]
fn renderers_match_go_context_displays() {
    use construct::render::{strip_ansi, OutputFormat, RenderOptions, Renderer};
    let Some(bin) = binary() else {
        return;
    };
    let sandbox = Sandbox::new();
    sandbox.go(
        &bin,
        &[
            "context",
            "add",
            "local",
            "--endpoint",
            "/tmp/local.sock",
            "--set-current",
        ],
    );
    let items = [ContextDisplay {
        name: "local".into(),
        endpoint: "/tmp/local.sock".into(),
        kind: "unix".into(),
        auth: "none".into(),
        current: true,
    }];
    for (format, name) in [
        (OutputFormat::Json, "json"),
        (OutputFormat::Yaml, "yaml"),
        (OutputFormat::Table, "table"),
        (OutputFormat::Card, "card"),
        (OutputFormat::Markdown, "markdown"),
    ] {
        let go = sandbox.go(&bin, &["context", "list", "-o", name]);
        let mut rust = Vec::new();
        Renderer
            .render(
                &mut rust,
                &items,
                &RenderOptions {
                    format,
                    ..Default::default()
                },
            )
            .unwrap();
        let rust = String::from_utf8(rust).unwrap();
        if format == OutputFormat::Table {
            assert_eq!(strip_ansi(&rust), strip_ansi(&go), "{name}");
        } else {
            assert_eq!(rust, go, "{name}");
        }
    }
    eprintln!("[cli/render] JSON/YAML/card/markdown bytes match Go; table spacing matches after terminal-dependent ANSI stripping");
}
struct Daemon(Child);
impl Drop for Daemon {
    fn drop(&mut self) {
        let _ = Command::new("kill")
            .args(["-TERM", &self.0.id().to_string()])
            .status();
        let _ = self.0.wait();
    }
}
async fn daemon(bin: &Path, sandbox: &Sandbox, tcp: bool) -> (Daemon, EndpointContext) {
    let socket = sandbox.dir.path().join("daemon.sock");
    let log = sandbox
        .dir
        .path()
        .join(if tcp { "tcp.log" } else { "unix.log" });
    let file = File::create(&log).unwrap();
    let listener = tcp.then(|| TcpListener::bind("127.0.0.1:0").unwrap());
    let address = listener
        .as_ref()
        .map(|l| format!("http://{}", l.local_addr().unwrap()));
    let mut command = if let Some(listener) = &listener {
        let mut c = sandbox.command(Path::new("/bin/sh"));
        c.args(["-c", "export LISTEN_PID=$$; exec \"$@\"", "activation"])
            .arg(bin)
            .args(["daemon", "run"])
            .env("LISTEN_FDS", "1");
        let fd = listener.as_raw_fd();
        unsafe {
            use std::os::unix::process::CommandExt;
            c.pre_exec(move || {
                if libc::dup2(fd, 3) == -1 || libc::fcntl(3, libc::F_SETFD, 0) == -1 {
                    return Err(std::io::Error::last_os_error());
                }
                Ok(())
            });
        }
        c
    } else {
        let mut c = sandbox.command(bin);
        c.args(["daemon", "run", "--listen-unix"]).arg(&socket);
        c
    };
    let mut daemon = Daemon(
        command
            .stdout(file.try_clone().unwrap())
            .stderr(file)
            .spawn()
            .unwrap(),
    );
    drop(listener);
    let deadline = Instant::now() + Duration::from_secs(30);
    loop {
        assert!(
            daemon.0.try_wait().unwrap().is_none(),
            "daemon exited: {}",
            fs::read_to_string(&log).unwrap()
        );
        if fs::read_to_string(&log)
            .unwrap()
            .contains("API server configured")
        {
            break;
        }
        assert!(Instant::now() < deadline, "daemon startup timed out");
        tokio::time::sleep(Duration::from_millis(25)).await;
    }
    (
        daemon,
        EndpointContext {
            address: address.unwrap_or_else(|| socket.to_str().unwrap().into()),
            kind: if tcp { "http" } else { "unix" }.into(),
            auth: None,
        },
    )
}
#[tokio::test]
async fn context_client_reaches_unix_and_authenticated_tcp() {
    let Some(bin) = binary() else {
        return;
    };
    let sandbox = Sandbox::new();
    let mut config = ConfigStore::load(&sandbox.user).unwrap();
    config.set("secret.provider", parse_value("file")).unwrap();
    config.flush().unwrap();
    let manager = sandbox.manager();
    let (unix, endpoint) = daemon(&bin, &sandbox, false).await;
    let client = ApiClient::new(&endpoint, &manager).unwrap();
    client
        .model_provider
        .create_model_provider(Request::new(CreateModelProviderRequest {
            name: "offline-cli-interop".into(),
            provider_type: ModelProviderType::Anthropic as i32,
            authentication: Some(create_model_provider_request::Authentication::ApiKey(
                "not-a-real-api-key".into(),
            )),
            url: Some("http://127.0.0.1:9".into()),
        }))
        .await
        .unwrap();
    let agents = client
        .agent
        .list_agents(Request::new(ListAgentsRequest::default()))
        .await
        .unwrap()
        .message
        .agents;
    assert!(!agents.is_empty());
    let token = client
        .auth
        .create_token(Request::new(CreateTokenRequest {
            name: "cli-interop".into(),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message
        .token;
    drop(unix);
    let (_tcp, mut endpoint) = daemon(&bin, &sandbox, true).await;
    let unauth = ApiClient::new(&endpoint, &manager)
        .unwrap()
        .agent
        .list_agents(Request::new(ListAgentsRequest::default()))
        .await
        .unwrap_err();
    assert_eq!(unauth.code, construct_connect::Code::Unauthenticated);
    manager.store_token("interop", &token).unwrap();
    endpoint.auth = Some(AuthConfig {
        auth_type: "token".into(),
        token_ref: "keyring://construct/interop".into(),
        ..Default::default()
    });
    let agents = ApiClient::new(&endpoint, &manager)
        .unwrap()
        .agent
        .list_agents(Request::new(ListAgentsRequest::default()))
        .await
        .unwrap()
        .message
        .agents;
    assert!(!agents.is_empty());
    eprintln!("[cli/api] context-built ApiClient lists agents over Unix and authenticated TCP; unauthenticated TCP rejected");
}
#[test]
fn real_keyring_is_go_compatible() {
    if std::env::var("CONSTRUCT_KEYRING_INTEROP").as_deref() != Ok("1") {
        eprintln!(
            "SKIP real keyring: set CONSTRUCT_KEYRING_INTEROP=1 with unlocked Secret Service"
        );
        return;
    }
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("../../..");
    let dir = tempfile::tempdir().unwrap();
    let source = dir.path().join("keyring.go");
    fs::write(
        &source,
        include_str!("../../../scripts/keyring-interop.go.txt"),
    )
    .unwrap();
    let helper = dir.path().join("keyring-helper");
    assert!(Command::new("go")
        .current_dir(&root)
        .args(["build", "-o"])
        .arg(&helper)
        .arg(&source)
        .status()
        .unwrap()
        .success());
    let key = format!("rust-cli-interop-{}", std::process::id());
    let provider = SystemProvider;
    let go = |action: &str, value: Option<&str>| {
        let mut cmd = Command::new(&helper);
        cmd.args([action, &key]);
        if let Some(value) = value {
            cmd.arg(value);
        }
        let output = cmd.output().unwrap();
        assert!(
            output.status.success(),
            "{}",
            String::from_utf8_lossy(&output.stderr)
        );
        String::from_utf8(output.stdout).unwrap()
    };
    go("set", Some("go-test-token"));
    assert_eq!(provider.get(&key).unwrap(), "go-test-token");
    provider.set(&key, "rust-test-token").unwrap();
    assert_eq!(go("get", None), "rust-test-token");
    provider.delete(&key).unwrap();
    provider.set(&key, "new-rust-entry").unwrap();
    assert_eq!(go("get", None), "new-rust-entry");
    go("delete", None);
    eprintln!("[cli/keyring] real Secret Service Go -> Rust, Rust -> Go, overwrite and delete verified (no tokens logged)");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn rust_resource_commands_and_skill_layout_with_go_daemon() {
    let Some(go) = binary() else {
        return;
    };
    let rust = Path::new(env!("CARGO_BIN_EXE_construct"));
    let sandbox = Sandbox::new();
    let mut config = ConfigStore::load(&sandbox.user).unwrap();
    config.set("secret.provider", parse_value("file")).unwrap();
    config.flush().unwrap();
    let (_daemon, endpoint) = daemon(&go, &sandbox, false).await;
    sandbox
        .manager()
        .upsert("smoke", endpoint.clone(), true)
        .unwrap();
    let client = ApiClient::new(&endpoint, &sandbox.manager()).unwrap();
    client
        .model_provider
        .create_model_provider(Request::new(CreateModelProviderRequest {
            name: "offline-resource-smoke".into(),
            provider_type: ModelProviderType::Anthropic as i32,
            authentication: Some(create_model_provider_request::Authentication::ApiKey(
                "not-a-real-api-key".into(),
            )),
            url: Some("http://127.0.0.1:9".into()),
        }))
        .await
        .unwrap();
    let agents: Value =
        serde_json::from_str(&sandbox.go(rust, &["agent", "list", "-o", "json"])).unwrap();
    assert!(!agents.as_array().unwrap().is_empty());
    for group in ["model", "modelprovider"] {
        let values: Value =
            serde_json::from_str(&sandbox.go(rust, &[group, "list", "-o", "json"])).unwrap();
        assert!(values.is_array());
    }
    let task = sandbox.go(
        rust,
        &[
            "task",
            "create",
            "--agent",
            agents[0]["id"].as_str().unwrap(),
        ],
    );
    let task = task.trim();
    assert!(uuid::Uuid::parse_str(task).is_ok());
    let tasks: Value =
        serde_json::from_str(&sandbox.go(rust, &["task", "list", "-o", "json"])).unwrap();
    assert!(tasks.as_array().unwrap().iter().any(|t| t["id"] == task));
    let messages: Value =
        serde_json::from_str(&sandbox.go(rust, &["message", "list", "--task", task, "-o", "json"]))
            .unwrap();
    assert!(messages.is_array());

    let fixture = Arc::new(std::sync::Mutex::new(
        "---\nname: offline-smoke\ndescription: Offline fixture\n---\nFirst instructions.\n"
            .to_owned(),
    ));
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let source = format!("http://{}/SKILL.md", listener.local_addr().unwrap());
    let served = fixture.clone();
    let server = tokio::spawn(async move {
        loop {
            let (mut stream, _) = listener.accept().await.unwrap();
            let mut request = [0; 4096];
            let received = stream.read(&mut request).await.unwrap();
            assert!(received > 0);
            let content = served.lock().unwrap().clone();
            let response = format!(
                "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{content}",
                content.len()
            );
            stream.write_all(response.as_bytes()).await.unwrap();
        }
    });
    let installed: Value =
        serde_json::from_str(&sandbox.go(rust, &["skill", "install", &source, "-o", "json"]))
            .unwrap();
    assert_eq!(installed[0]["name"], "offline-smoke");
    assert_eq!(installed[0]["source"], "url");
    let path = sandbox.user.config.join("skills/offline-smoke/SKILL.md");
    assert_eq!(fs::read_to_string(&path).unwrap(), *fixture.lock().unwrap());
    let lock: Value =
        serde_json::from_slice(&fs::read(sandbox.user.config.join("skills.lock.json")).unwrap())
            .unwrap();
    assert_eq!(lock["version"], 1);
    assert_eq!(
        lock["skills"]["offline-smoke"]["url"],
        json!({"url":source})
    );
    *fixture.lock().unwrap() =
        "---\nname: offline-smoke\ndescription: Updated fixture\n---\nUpdated instructions.\n"
            .into();
    let updated: Value = serde_json::from_str(
        &sandbox.go(rust, &["skill", "update", "offline-smoke", "-o", "json"]),
    )
    .unwrap();
    assert_eq!(updated[0]["description"], "Updated fixture");
    assert_eq!(fs::read_to_string(&path).unwrap(), *fixture.lock().unwrap());
    let skills: Value =
        serde_json::from_str(&sandbox.go(&go, &["skill", "list", "-o", "json"])).unwrap();
    assert!(skills
        .as_array()
        .unwrap()
        .iter()
        .any(|s| s["name"] == "offline-smoke"));
    sandbox.go(rust, &["skill", "delete", "offline-smoke", "--force"]);
    assert!(!path.exists());
    server.abort();
    eprintln!("[cli/resources] Rust agent/model/modelprovider list, task create/list, message list against Go daemon; localhost-only skill install/update/list/delete and Go skill layout verified");
}
