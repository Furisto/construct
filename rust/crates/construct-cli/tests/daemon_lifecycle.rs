#![cfg(target_os = "linux")]

use construct_connect::{async_trait, serve_tcp, ConnectError, Request, Response, Router};
use construct_proto::construct::v1::{
    ListModelProvidersRequest, ListModelProvidersResponse, ModelProviderService,
    ModelProviderServiceServer,
};
use std::{fs, os::unix::fs::PermissionsExt};
use tokio::process::Command;

struct Daemon;

#[async_trait]
impl ModelProviderService for Daemon {
    async fn list_model_providers(
        &self,
        _: Request<ListModelProvidersRequest>,
    ) -> Result<Response<ListModelProvidersResponse>, ConnectError> {
        Ok(Response::new(ListModelProvidersResponse::default()))
    }
}

#[tokio::test]
async fn install_stop_and_uninstall_user_units() {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = listener.local_addr().unwrap().to_string();
    let server = tokio::spawn(serve_tcp(
        listener,
        Router::new().nest("/api", ModelProviderServiceServer::new(Daemon).router()),
        std::future::pending(),
    ));
    let home = tempfile::tempdir().unwrap();
    let script = home.path().join("systemctl");
    let calls = home.path().join("calls");
    fs::write(
        &script,
        "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$SYSTEMCTL_CALLS\"\n",
    )
    .unwrap();
    fs::set_permissions(&script, fs::Permissions::from_mode(0o700)).unwrap();
    let command = || {
        let mut cmd = Command::new(env!("CARGO_BIN_EXE_construct"));
        cmd.env_clear()
            .env("PATH", home.path())
            .env("HOME", home.path())
            .env("SYSTEMCTL_CALLS", &calls)
            .arg("daemon");
        cmd
    };
    let install = ["install", "--name", "test", "--listen-http", &address];
    let output = command().args(install).output().await.unwrap();
    assert!(
        output.status.success(),
        "{}",
        String::from_utf8_lossy(&output.stderr)
    );
    assert!(String::from_utf8_lossy(&output.stdout).contains("Next: Create a model provider"));
    let units = home.path().join(".config/systemd/user");
    let service = units.join("construct.service");
    let socket = units.join("construct.socket");
    assert!(fs::read_to_string(&socket)
        .unwrap()
        .contains(&format!("ListenStream={address}")));
    assert!(!fs::read_to_string(&service).unwrap().contains("[Install]"));
    let config = home.path().join(".config/construct/context.yaml");
    let config: serde_yaml::Value = serde_yaml::from_slice(&fs::read(config).unwrap()).unwrap();
    assert_eq!(config["current"], "test");
    assert_eq!(
        config["contexts"]["test"]["address"],
        format!("http://{address}")
    );
    let output = command().args(install).output().await.unwrap();
    assert!(!output.status.success());
    assert!(String::from_utf8_lossy(&output.stderr).contains("already installed"));
    let output = command()
        .args(install)
        .args(["-f", "-q", "--always-running"])
        .output()
        .await
        .unwrap();
    assert!(
        output.status.success(),
        "{}",
        String::from_utf8_lossy(&output.stderr)
    );
    assert!(output.stdout.is_empty());
    assert!(fs::read_to_string(&service)
        .unwrap()
        .contains("WantedBy=default.target"));

    let output = command().arg("stop").output().await.unwrap();
    assert!(output.status.success());
    assert!(service.is_file() && socket.is_file());
    let output = command().arg("uninstall").output().await.unwrap();
    assert!(output.status.success());
    assert!(
        service.is_file() && socket.is_file(),
        "EOF must cancel uninstall"
    );
    let output = command()
        .args(["uninstall", "-y", "-q"])
        .output()
        .await
        .unwrap();
    assert!(
        output.status.success(),
        "{}",
        String::from_utf8_lossy(&output.stderr)
    );
    assert!(output.stdout.is_empty());
    assert!(!service.exists() && !socket.exists());
    assert_eq!(
        fs::read_to_string(&calls)
            .unwrap()
            .lines()
            .collect::<Vec<_>>(),
        [
            "--user daemon-reload",
            "--user enable --now construct.socket",
            "--user stop construct.socket construct.service",
            "--user disable construct.socket construct.service",
            "--user daemon-reload",
            "--user daemon-reload",
            "--user enable --now construct.socket",
            "--user enable --now construct.service",
            "--user stop construct.socket construct.service",
            "--user stop construct.socket construct.service",
            "--user disable construct.socket construct.service",
            "--user daemon-reload",
        ]
    );
    server.abort();
}
