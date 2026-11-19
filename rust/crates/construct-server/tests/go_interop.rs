//! Opt-in real Go CLI against the Rust top-level server and minimal binary.
#![cfg(target_os = "linux")]
mod listener_support;
use construct_connect::Request;
use construct_proto::construct::v1 as pb;
use listener_support::{Host, Launch};
use serde_json::{json, Value};
use std::{
    fs,
    os::unix::fs::PermissionsExt,
    path::{Path, PathBuf},
    process::Output,
    time::Duration,
};
use tokio::{process::Command, time::timeout};

struct Go {
    binary: PathBuf,
    home: PathBuf,
}
impl Go {
    fn command(&self, args: &[&str]) -> Command {
        let mut c = Command::new(&self.binary);
        c.env_clear()
            .env("PATH", "/usr/local/bin:/usr/bin:/bin")
            .env("HOME", &self.home)
            .env("XDG_CONFIG_HOME", self.home.join("config"))
            .env("XDG_DATA_HOME", self.home.join("data"))
            .env("XDG_CACHE_HOME", self.home.join("cache"))
            .env("XDG_STATE_HOME", self.home.join("state"))
            .env("XDG_RUNTIME_DIR", self.home.join("runtime"))
            .env("HTTP_PROXY", "http://127.0.0.1:9")
            .env("HTTPS_PROXY", "http://127.0.0.1:9")
            .env("NO_PROXY", "localhost,127.0.0.1,::1")
            .args(args)
            .kill_on_drop(true);
        c
    }
    async fn output(&self, args: &[&str]) -> Output {
        timeout(Duration::from_secs(20), self.command(args).output())
            .await
            .expect("Go CLI timed out")
            .unwrap()
    }
    async fn run(&self, args: &[&str]) -> String {
        let out = self.output(args).await;
        assert!(
            out.status.success(),
            "Go {args:?} failed: {}",
            String::from_utf8_lossy(&out.stderr)
        );
        let stdout = String::from_utf8(out.stdout).unwrap();
        if stdout.trim().is_empty() {
            String::from_utf8(out.stderr).unwrap().trim().to_owned()
        } else {
            stdout.trim().to_owned()
        }
    }
    async fn value(&self, args: &[&str]) -> Value {
        let out = self.run(args).await;
        serde_json::from_str(&out).unwrap_or_else(|_| panic!("non-JSON CLI output: {out}"))
    }
    async fn context(&self, name: &str, address: &str) {
        self.run(&[
            "context",
            "add",
            name,
            "--endpoint",
            address,
            "--set-current",
        ])
        .await;
    }
    fn http_context(&self, address: &str, token: &str) {
        let path = self.home.join("config/construct/context.yaml");
        fs::write(&path, serde_json::to_vec(&json!({"current":"remote", "contexts":{"remote":{"kind":"http", "address":address,"auth":{"type":"token","token":token}}}})).unwrap()).unwrap();
        fs::set_permissions(path, fs::Permissions::from_mode(0o600)).unwrap();
    }
}
fn address(host: &Host, unix: bool) -> String {
    if unix {
        host.startup["address"].as_str().unwrap().into()
    } else {
        format!("http://{}", host.startup["address"].as_str().unwrap())
    }
}
fn has_id(list: &Value, id: &str) -> bool {
    list.as_array().unwrap().iter().any(|v| v["id"] == id)
}

#[tokio::test]
async fn real_go_cli_rust_server_unix_http_and_systemd() {
    let Some(binary) = std::env::var_os("CONSTRUCT_GO_BIN") else {
        eprintln!("SKIP real Go CLI/Rust server: CONSTRUCT_GO_BIN is not set (run rust/scripts/go-interop.sh)");
        return;
    };
    let dir = tempfile::tempdir().unwrap();
    let home = dir.path().join("go");
    for folder in ["config/construct", "data", "cache", "state", "runtime"] {
        fs::create_dir_all(home.join(folder)).unwrap();
    }
    fs::write(
        home.join("config/construct/config.yaml"),
        "secret:\n  provider: file\n",
    )
    .unwrap();
    let go = Go {
        binary: fs::canonicalize(Path::new(&binary))
            .expect("CONSTRUCT_GO_BIN must point to a built Go CLI"),
        home,
    };
    let server_dir = dir.path().join("rust");
    let host = Host::start(&server_dir, Launch::Unix).await;
    go.context("local", &address(&host, true)).await;
    assert!(go
        .value(&["agent", "list", "--output", "json"])
        .await
        .as_array()
        .unwrap()
        .is_empty());
    go.run(&[
        "modelprovider",
        "create",
        "interop-provider",
        "--type",
        "anthropic",
        "--api-key",
        "interop-placeholder-not-a-real-key",
    ])
    .await;
    let providers = go
        .value(&["modelprovider", "list", "--output", "json"])
        .await;
    assert_eq!(providers.as_array().unwrap().len(), 1);
    let model = go
        .run(&[
            "model",
            "create",
            "interop-model",
            "--provider",
            "interop-provider",
            "--context-window",
            "4096",
        ])
        .await;
    assert!(uuid::Uuid::parse_str(&model).is_ok());
    assert!(has_id(
        &go.value(&["model", "list", "--all", "--output", "json"])
            .await,
        &model
    ));
    let agent = go
        .run(&[
            "agent",
            "create",
            "interop-agent",
            "--model",
            "claude-sonnet-4-20250514",
            "--prompt",
            "No model calls in this test.",
        ])
        .await;
    assert!(uuid::Uuid::parse_str(&agent).is_ok());
    assert!(has_id(
        &go.value(&["agent", "list", "--output", "json"]).await,
        &agent
    ));
    let task = go
        .run(&["task", "create", "--agent", "interop-agent"])
        .await;
    assert!(uuid::Uuid::parse_str(&task).is_ok());
    let message = go
        .run(&["message", "create", &task, "hello from the real Go CLI"])
        .await;
    assert!(has_id(
        &go.value(&["task", "list", "--limit", "10", "--output", "json"])
            .await,
        &task
    ));
    let messages = go
        .value(&["message", "list", "--task", &task, "--output", "json"])
        .await;
    assert!(has_id(&messages, &message));
    assert_eq!(messages[0]["content"], "hello from the real Go CLI");
    let setup = go
        .value(&[
            "daemon",
            "token",
            "create-setup",
            "go-remote",
            "--output",
            "json",
        ])
        .await;
    let help = go.run(&["context", "add", "--help"]).await;
    // Go advertises --setup-code in create-setup's help but does not implement the flag.
    assert!(
        !help.contains("--setup-code"),
        "Go CLI now implements setup exchange; extend this test to use it"
    );
    let exchange = pb::AuthServiceClient::new(host.channel())
        .exchange_setup_code(Request::new(pb::ExchangeSetupCodeRequest {
            setup_code: setup["setup_code"].as_str().unwrap().into(),
        }))
        .await
        .unwrap()
        .message;
    eprintln!("PASS real Go CLI -> Rust Unix: context, provider/model/agent create+list, task/message create+list, setup code");
    eprintln!("NOTE Go context add has no --setup-code flag; generated Rust client exchanges the Go-created setup code");
    host.stop().await;

    let host = Host::start(&server_dir, Launch::Tcp).await;
    let url = address(&host, false);
    go.context("unauthenticated", &url).await;
    let denied = go.output(&["agent", "list", "--output", "json"]).await;
    assert!(!denied.status.success());
    assert!(String::from_utf8_lossy(&denied.stderr).contains("unauthenticated"));
    go.http_context(&url, &exchange.token);
    assert!(has_id(
        &go.value(&["agent", "list", "--output", "json"]).await,
        &agent
    ));
    assert!(has_id(
        &go.value(&["task", "list", "--limit", "10", "--output", "json"])
            .await,
        &task
    ));
    assert!(has_id(
        &go.value(&["message", "list", "--task", &task, "--output", "json"])
            .await,
        &message
    ));
    let denied = go
        .output(&["daemon", "token", "create", "forbidden", "--output", "json"])
        .await;
    assert!(!denied.status.success());
    assert!(String::from_utf8_lossy(&denied.stderr).contains("permission_denied"));
    eprintln!("PASS real Go CLI -> Rust --listen-http: setup-derived token, persisted data, unauthenticated/admin rejection");
    host.stop().await;

    for launch in [Launch::SystemdTcp, Launch::SystemdUnix] {
        let host = Host::start(&server_dir, launch).await;
        if launch.unix() {
            go.context("activated", &address(&host, true)).await;
        } else {
            go.http_context(&address(&host, false), &exchange.token);
        }
        assert!(has_id(
            &go.value(&["agent", "list", "--output", "json"]).await,
            &agent
        ));
        if launch.unix() {
            go.value(&["daemon", "token", "list", "--output", "json"])
                .await;
        } else {
            let denied = go
                .output(&["daemon", "token", "list", "--output", "json"])
                .await;
            assert!(!denied.status.success());
            assert!(String::from_utf8_lossy(&denied.stderr).contains("permission_denied"));
        }
        eprintln!("PASS real Go CLI -> Rust FD-3 activation: {launch:?}, transport trust and authenticated agent list");
        host.stop().await;
    }
}
