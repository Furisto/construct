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
    create_model_provider_request, CreateModelProviderRequest, CreateSetupCodeRequest,
    CreateTokenRequest, ListAgentsRequest, ModelProviderType,
};
use serde_json::{json, Value};
use std::{
    collections::BTreeMap,
    fs::{self, File},
    io::Write,
    net::TcpListener,
    os::fd::AsRawFd,
    path::{Path, PathBuf},
    process::{Child, Command, Stdio},
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
        if let Some(address) = std::env::var_os("DBUS_SESSION_BUS_ADDRESS") {
            cmd.env("DBUS_SESSION_BUS_ADDRESS", address);
        }
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

fn pty_command(
    sandbox: &Sandbox,
    bin: &Path,
    args: &[&str],
    prompt: &str,
    input: &str,
    interrupt: bool,
) -> Value {
    let mut command = vec![bin.to_str().unwrap()];
    command.extend_from_slice(args);
    let mut child = sandbox
        .command(Path::new("/usr/bin/python3"))
        .arg("-c")
        .arg(include_str!("../../../scripts/cli-pty.py"))
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    serde_json::to_writer(
        child.stdin.take().unwrap(),
        &json!({"command":command,"prompt":prompt,"input":input,"interrupt":interrupt}),
    )
    .unwrap();
    let output = child.wait_with_output().unwrap();
    assert!(
        output.status.success(),
        "{}",
        String::from_utf8_lossy(&output.stderr)
    );
    let mut result: Value = serde_json::from_slice(&output.stdout).unwrap();
    result["transcript"] = json!(prompt_transcript(result["transcript"].as_str().unwrap()));
    assert_eq!(result["hidden"], true);
    assert_eq!(result["echoed"], false);
    eprintln!("[cli/pty] {} {:?}: echo disabled={}, input echoed={}, restored={}, Enter after Ctrl-C={}, exit={}\n{}", bin.file_name().unwrap().to_string_lossy(),args,result["hidden"],result["echoed"],result["restored"],result["needed_enter_after_interrupt"],result["status"],result["transcript"].as_str().unwrap());
    result
}
fn prompt_transcript(value: &str) -> String {
    let ansi =
        regex::Regex::new(r"\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x1b\x07]*(?:\x07|\x1b\\))").unwrap();
    let plain = ansi.replace_all(value, "");
    plain
        .split("\nUsage:")
        .next()
        .unwrap()
        .trim_end_matches('\n')
        .to_owned()
}

fn verify_provider_secrets(sandbox: &Sandbox) {
    let source = sandbox.dir.path().join("provider-secret.go");
    fs::write(
        &source,
        include_str!("../../../scripts/provider-secret-interop.go.txt"),
    )
    .unwrap();
    let helper = sandbox.dir.path().join("provider-secret-helper");
    assert!(Command::new("go")
        .current_dir(Path::new(env!("CARGO_MANIFEST_DIR")).join("../../.."))
        .env("CGO_ENABLED", "0")
        .args(["build", "-o"])
        .arg(&helper)
        .arg(&source)
        .status()
        .unwrap()
        .success());
    let mut child = Command::new(helper)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    serde_json::to_writer(child.stdin.take().unwrap(), &json!({"data":sandbox.user.data,"keys":{"pty-go-enter":" offline-pty-synthetic-key ","pty-rust-enter":" offline-pty-synthetic-key "}})).unwrap();
    let output = child.wait_with_output().unwrap();
    assert!(
        output.status.success(),
        "{}",
        String::from_utf8_lossy(&output.stderr)
    );
    eprint!("{}", String::from_utf8_lossy(&output.stdout));
}

struct KeyringEntries(Vec<String>);
impl Drop for KeyringEntries {
    fn drop(&mut self) {
        for key in &self.0 {
            let _ = SystemProvider.delete(key);
        }
    }
}
fn inspect_keyring(key: &str, expected: Option<&str>) {
    let service =
        dbus_secret_service::SecretService::connect(dbus_secret_service::EncryptionType::Dh)
            .unwrap();
    let found = service
        .search_items(std::collections::HashMap::from([
            ("service", "construct"),
            ("username", key),
        ]))
        .unwrap();
    assert!(found.locked.is_empty());
    if let Some(expected) = expected {
        assert_eq!(found.unlocked.len(), 1);
        let item = &found.unlocked[0];
        let attributes = item.get_attributes().unwrap();
        assert_eq!(attributes.get("service").unwrap(), "construct");
        assert_eq!(attributes.get("username").unwrap(), key);
        assert!(
            item.get_secret().unwrap() == expected.as_bytes(),
            "keyring secret bytes differ"
        );
        eprintln!("[cli/keyring/attributes] service=construct username={key}; unlocked; secret bytes match exactly (redacted)");
    } else {
        assert!(found.unlocked.is_empty());
        assert!(SystemProvider.get(key).is_err());
        eprintln!("[cli/keyring/attributes] service=construct username={key}; absent after opposite CLI removal");
    }
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn real_keyring_context_tokens_cross_cli_and_removal() {
    if std::env::var("CONSTRUCT_KEYRING_INTEROP").as_deref() != Ok("1") {
        eprintln!("SKIP real context keyring: CONSTRUCT_KEYRING_INTEROP not set");
        return;
    }
    let Some(go) = binary() else {
        return;
    };
    let rust = Path::new(env!("CARGO_BIN_EXE_construct"));
    let sandbox = Sandbox::new();
    let mut config = ConfigStore::load(&sandbox.user).unwrap();
    config.set("secret.provider", parse_value("file")).unwrap();
    config.flush().unwrap();
    let (unix, endpoint) = daemon(&go, &sandbox, false).await;
    let client = ApiClient::new(&endpoint, &sandbox.manager()).unwrap();
    let token = client
        .auth
        .create_token(Request::new(CreateTokenRequest {
            name: "real-keyring-context".into(),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message
        .token;
    drop(unix);
    let (_tcp, endpoint) = daemon(&go, &sandbox, true).await;
    let keys = KeyringEntries(vec![
        format!("rust-cli-context-{}-go", std::process::id()),
        format!("rust-cli-context-{}-rust", std::process::id()),
    ]);
    for (index, creator, reader) in [(0, go.as_path(), rust), (1, rust, go.as_path())] {
        let key = &keys.0[index];
        let result = pty_command(
            &sandbox,
            creator,
            &[
                "context",
                "add",
                key,
                "--endpoint",
                &endpoint.address,
                "--auth-token",
                "--set-current",
            ],
            "Enter authentication token: ",
            &token,
            false,
        );
        assert_eq!(result["status"], 0);
        assert_eq!(result["restored"], true);
        inspect_keyring(key, Some(&token));
        assert_eq!(
            sandbox.go(reader, &["context", "current"]),
            format!("{key}\n")
        );
        let agents: Value =
            serde_json::from_str(&sandbox.go(reader, &["agent", "list", "-o", "json"])).unwrap();
        assert!(agents.is_array());
        sandbox.go(reader, &["context", "remove", key, "--force"]);
        inspect_keyring(key, None);
    }
    eprintln!("[cli/keyring/context] Go -> Rust and Rust -> Go: real context add/current, authenticated TCP agent list, opposite CLI removal verified");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn setup_code_enrolls_rust_context_with_real_keyring_and_go_daemon() {
    if std::env::var("CONSTRUCT_KEYRING_INTEROP").as_deref() != Ok("1") {
        eprintln!("SKIP setup code keyring: CONSTRUCT_KEYRING_INTEROP not set");
        return;
    }
    let Some(go) = binary() else {
        return;
    };
    let rust = Path::new(env!("CARGO_BIN_EXE_construct"));
    let sandbox = Sandbox::new();
    let mut config = ConfigStore::load(&sandbox.user).unwrap();
    config.set("secret.provider", parse_value("file")).unwrap();
    config.flush().unwrap();
    let (unix, local) = daemon(&go, &sandbox, false).await;
    let admin = ApiClient::new(&local, &sandbox.manager()).unwrap();
    let setup_code = admin
        .auth
        .create_setup_code(Request::new(CreateSetupCodeRequest {
            token_name: "setup-code-interop".into(),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message
        .setup_code;
    // Pending codes are in memory, and the daemon exposes only one transport.
    // Relay enrollment to its Unix listener, then hand the same TCP port to the daemon.
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    listener.set_nonblocking(true).unwrap();
    let endpoint = EndpointContext {
        address: format!("http://{}", listener.local_addr().unwrap()),
        kind: "http".into(),
        auth: None,
    };
    let relay_listener = tokio::net::TcpListener::from_std(listener.try_clone().unwrap()).unwrap();
    let socket = local.address.clone();
    let relay = tokio::spawn(async move {
        while let Ok((mut tcp, _)) = relay_listener.accept().await {
            let mut unix = tokio::net::UnixStream::connect(&socket).await.unwrap();
            tokio::spawn(async move {
                tokio::io::copy_bidirectional(&mut tcp, &mut unix)
                    .await
                    .unwrap();
            });
        }
    });
    let key = format!("rust-cli-setup-{}", std::process::id());
    let _cleanup = KeyringEntries(vec![key.clone()]);
    let args = [
        "context",
        "add",
        &key,
        "--endpoint",
        &endpoint.address,
        "--setup-code",
        &setup_code,
        "--set-current",
    ];
    let output = sandbox.go(rust, &args);
    assert_eq!(
        output,
        format!("Context {key:?} created and set as current\n")
    );
    let manager = ContextManager::new(&sandbox.user, Arc::new(SystemProvider));
    let contexts = manager.load().unwrap();
    let auth = contexts.current().unwrap().auth.as_ref().unwrap();
    assert_eq!(auth.token_ref, format!("keyring://construct/{key}"));
    assert!(auth.token.is_empty());
    let token = manager.resolve_token(auth).unwrap();
    inspect_keyring(&key, Some(&token));
    let yaml = fs::read_to_string(sandbox.user.config.join("context.yaml")).unwrap();
    assert!(!yaml.contains(&token));
    assert!(!yaml.contains(&setup_code));
    let before = fs::read(sandbox.user.config.join("context.yaml")).unwrap();
    let reused = sandbox.command(rust).args(args).output().unwrap();
    assert!(!reused.status.success());
    assert!(String::from_utf8_lossy(&reused.stderr).contains("invalid or expired setup code"));
    assert_eq!(
        fs::read(sandbox.user.config.join("context.yaml")).unwrap(),
        before
    );
    inspect_keyring(&key, Some(&token));
    relay.abort();
    let _ = relay.await;
    drop(unix);
    let (_tcp, _) = daemon_with_listener(&go, &sandbox, Some(listener)).await;
    let unauthenticated = ApiClient::new(&endpoint, &sandbox.manager())
        .unwrap()
        .agent
        .list_agents(Request::new(ListAgentsRequest::default()))
        .await
        .unwrap_err();
    assert_eq!(
        unauthenticated.code,
        construct_connect::Code::Unauthenticated
    );
    for bin in [rust, go.as_path()] {
        assert_eq!(sandbox.go(bin, &["context", "current"]), format!("{key}\n"));
        let agents: Value =
            serde_json::from_str(&sandbox.go(bin, &["agent", "list", "-o", "json"])).unwrap();
        assert_eq!(agents, json!([]));
    }
    eprintln!("[cli/setup/go] HTTP enrollment through TCP-to-Unix relay, then real socket-activated TCP on the same address:\nRust context add {key} --endpoint {} --setup-code [REDACTED] --set-current\n{}\nUnauthenticated TCP agent list -> unauthenticated; Rust agent list -o json -> []; Go agent list -o json -> []; reusing code -> invalid or expired setup code; context and keyring unchanged", endpoint.address, output.trim_end());
    sandbox.go(&go, &["context", "remove", &key, "--force"]);
    inspect_keyring(&key, None);
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn hidden_provider_key_prompt_go_rust_pty_and_pipe() {
    let Some(go) = binary() else {
        return;
    };
    let rust = Path::new(env!("CARGO_BIN_EXE_construct"));
    let sandbox = Sandbox::new();
    let mut config = ConfigStore::load(&sandbox.user).unwrap();
    config.set("secret.provider", parse_value("file")).unwrap();
    config.flush().unwrap();
    let (_daemon, endpoint) = daemon(&go, &sandbox, false).await;
    sandbox.manager().upsert("pty", endpoint, true).unwrap();
    let mut results = Vec::new();
    let mut pipe_results = Vec::new();
    for (label, bin) in [("go", go.as_path()), ("rust", rust)] {
        let mut cases = Vec::new();
        for case in ["enter", "empty", "interrupt"] {
            let name = format!("pty-{label}-{case}");
            let prompt = format!("Enter Anthropic API key for {name}: ");
            let input = if case == "empty" {
                ""
            } else {
                " offline-pty-synthetic-key "
            };
            let result = pty_command(
                &sandbox,
                bin,
                &["modelprovider", "create", &name, "--type", "anthropic"],
                &prompt,
                input,
                case == "interrupt",
            );
            assert_eq!(result["restored"], true);
            if case == "enter" {
                assert_eq!(result["status"], 0);
                let text = result["transcript"].as_str().unwrap();
                let id = text
                    .strip_prefix(&format!("{prompt}\n"))
                    .expect("prompt must be followed by exactly one newline");
                assert!(uuid::Uuid::parse_str(id).is_ok());
            } else {
                assert_eq!(result["status"], 1);
                assert_eq!(result["transcript"], format!("{prompt}\nError: API key cannot be empty\n\nTip: You can also set the ANTHROPIC_API_KEY environment variable or use the --api-key flag"));
            }
            cases.push(result);
        }
        let mut child = sandbox
            .command(bin)
            .args(["modelprovider", "create", "pty-pipe", "--type", "anthropic"])
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .unwrap();
        child
            .stdin
            .take()
            .unwrap()
            .write_all(b"offline-piped-key\n")
            .unwrap();
        let output = child.wait_with_output().unwrap();
        assert!(!output.status.success());
        let transcript = format!(
            "{}{}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
        assert!(!transcript.contains("offline-piped-key"));
        let transcript = prompt_transcript(&transcript);
        assert_eq!(output.status.code(), Some(1));
        eprintln!("[cli/pipe] {label}: exit=1\n{transcript}");
        pipe_results.push(transcript);
        results.push(cases);
    }
    assert_eq!(
        results[0][2]["needed_enter_after_interrupt"],
        results[1][2]["needed_enter_after_interrupt"]
    );
    assert_eq!(results[0][2]["needed_enter_after_interrupt"], true);
    assert_eq!(pipe_results[0], pipe_results[1]);
    verify_provider_secrets(&sandbox);
    let providers: Value =
        serde_json::from_str(&sandbox.go(rust, &["modelprovider", "list", "-o", "json"])).unwrap();
    let names: Vec<_> = providers
        .as_array()
        .unwrap()
        .iter()
        .map(|p| p["name"].as_str().unwrap())
        .collect();
    assert_eq!(names.len(), 2);
    assert!(names.contains(&"pty-go-enter"));
    assert!(names.contains(&"pty-rust-enter"));
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
    daemon_with_listener(
        bin,
        sandbox,
        tcp.then(|| TcpListener::bind("127.0.0.1:0").unwrap()),
    )
    .await
}
async fn daemon_with_listener(
    bin: &Path,
    sandbox: &Sandbox,
    listener: Option<TcpListener>,
) -> (Daemon, EndpointContext) {
    let tcp = listener.is_some();
    let socket = sandbox.dir.path().join("daemon.sock");
    let log = sandbox
        .dir
        .path()
        .join(if tcp { "tcp.log" } else { "unix.log" });
    let file = File::create(&log).unwrap();
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
fn info_output_matches_go() {
    let Some(bin) = binary() else {
        return;
    };
    let sandbox = Sandbox::new();
    let rust = Path::new(env!("CARGO_BIN_EXE_construct"));
    if [
        option_env!("CONSTRUCT_VERSION"),
        option_env!("CONSTRUCT_GIT_COMMIT"),
        option_env!("CONSTRUCT_BUILD_DATE"),
    ]
    .into_iter()
    .flatten()
    .any(|v| !v.is_empty() && v != "unknown")
    {
        return;
    }
    sandbox
        .manager()
        .upsert(
            "local",
            EndpointContext {
                kind: "unix".into(),
                address: "/tmp/unused.sock".into(),
                auth: None,
            },
            true,
        )
        .unwrap();
    for format in ["json", "yaml", "card", "markdown"] {
        let args = ["info", "--output", format];
        assert_eq!(sandbox.go(rust, &args), sandbox.go(&bin, &args), "{format}");
    }
}

#[tokio::test]
async fn rust_token_commands_interoperate_with_go_daemon() {
    let Some(bin) = binary() else {
        return;
    };
    use construct_proto::construct::v1::ExchangeSetupCodeRequest;
    let sandbox = Sandbox::new();
    let rust = Path::new(env!("CARGO_BIN_EXE_construct"));
    let mut config = ConfigStore::load(&sandbox.user).unwrap();
    config.set("secret.provider", parse_value("file")).unwrap();
    config.flush().unwrap();
    let (_daemon, endpoint) = daemon(&bin, &sandbox, false).await;
    let manager = sandbox.manager();
    manager.upsert("ops", endpoint.clone(), true).unwrap();
    let json = |cli: &Path, args: &[&str]| -> serde_json::Value {
        serde_json::from_str(&sandbox.go(cli, args)).unwrap()
    };
    let created = json(
        rust,
        &[
            "daemon",
            "token",
            "create",
            "ops-token",
            "--description",
            "interop",
            "--expires",
            "1.5d",
            "-o",
            "json",
        ],
    );
    assert_eq!(created["name"], "ops-token");
    assert!(created["token"].as_str().unwrap().starts_with("ct_"));
    assert!(chrono::DateTime::parse_from_rfc3339(created["expires_at"].as_str().unwrap()).is_ok());
    let list_args = [
        "daemon",
        "token",
        "ls",
        "--name-prefix",
        "ops-",
        "--include-expired",
        "-o",
        "json",
    ];
    let list = json(rust, &list_args);
    assert_eq!(list, json(&bin, &list_args));
    assert_eq!(list[0]["name"], "ops-token");
    assert_eq!(list[0]["status"], "Active");
    assert!(list[0].get("token").is_none());
    let id = list[0]["id"].as_str().unwrap();
    sandbox.go(rust, &["daemon", "token", "revoke", id]);
    assert_eq!(
        json(rust, &list_args)[0]["id"],
        id,
        "EOF must cancel revocation"
    );
    sandbox.go(rust, &["daemon", "token", "revoke", id, "-f"]);
    assert_eq!(json(&bin, &list_args), serde_json::json!([]));
    let setup = json(
        rust,
        &[
            "daemon",
            "token",
            "create-setup",
            "new-client",
            "--code-expires",
            "1m",
            "--token-expires",
            "2d",
            "-o",
            "json",
        ],
    );
    assert_eq!(setup["token_name"], "new-client");
    let exchange = ApiClient::new(&endpoint, &manager)
        .unwrap()
        .auth
        .exchange_setup_code(Request::new(ExchangeSetupCodeRequest {
            setup_code: setup["setup_code"].as_str().unwrap().into(),
        }))
        .await
        .unwrap()
        .message;
    assert_eq!(exchange.name, "new-client");
    assert!(exchange.token.starts_with("ct_"));
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
    let output = sandbox
        .command(rust)
        .args([
            "task",
            "create",
            "--agent",
            agents[0]["id"].as_str().unwrap(),
        ])
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "{}",
        String::from_utf8_lossy(&output.stderr)
    );
    assert!(output.stdout.is_empty());
    let task = String::from_utf8(output.stderr).unwrap();
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
