#![cfg(unix)]

use construct_connect::{Channel, ClientConfig, Code, Endpoint, Request};
use construct_proto::construct::v1 as pb;
use sha2::{Digest, Sha256};
use std::{fs, os::unix::fs::PermissionsExt, path::Path, process::Stdio, time::Duration};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    process::{Child, Command},
    time::{sleep, timeout},
};

fn command(base: &Path) -> Command {
    let mut cmd = Command::new(env!("CARGO_BIN_EXE_construct"));
    cmd.env_clear()
        .env("PATH", "/usr/local/bin:/usr/bin:/bin")
        .env("HOME", base)
        .env("XDG_CONFIG_HOME", base.join("config"))
        .env("XDG_DATA_HOME", base.join("data"))
        .env("XDG_STATE_HOME", base.join("state"))
        .env("XDG_RUNTIME_DIR", base.join("runtime"))
        .args(["daemon", "run"])
        .kill_on_drop(true);
    cmd
}

fn file_config(base: &Path) {
    fs::create_dir_all(base.join("config/construct")).unwrap();
    fs::write(
        base.join("config/construct/config.yaml"),
        "secret:\n  provider: file\n",
    )
    .unwrap();
}

fn channel(endpoint: Endpoint) -> Channel {
    Channel::new(ClientConfig::new(endpoint)).unwrap()
}

async fn health_unix(path: &Path) -> bool {
    let Ok(mut stream) = tokio::net::UnixStream::connect(path).await else {
        return false;
    };
    stream
        .write_all(b"GET /healthz HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")
        .await
        .unwrap();
    let mut response = String::new();
    stream.read_to_string(&mut response).await.unwrap();
    response.starts_with("HTTP/1.1 200") && response.contains("{\"status\":\"ok\"}")
}

async fn stop(child: &mut Child, signal: i32) {
    let pid = child.id().unwrap();
    assert_eq!(unsafe { libc::kill(pid as i32, signal) }, 0);
    let status = timeout(Duration::from_secs(10), child.wait())
        .await
        .unwrap()
        .unwrap();
    assert!(status.success(), "daemon exited with {status}");
}

#[tokio::test]
async fn foreground_listeners_share_auth_and_shutdown_cleanly() {
    for (unix, tcp) in [(true, true), (true, false), (false, true)] {
        let dir = tempfile::tempdir().unwrap();
        let base = dir.path();
        file_config(base);
        let socket = base.join("daemon.sock");
        let reservation = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let address = reservation.local_addr().unwrap().to_string();
        let mut cmd = command(base);
        if unix {
            cmd.arg("--listen-unix").arg(&socket);
        }
        if tcp {
            cmd.args(["--listen-http", &address]);
        }
        let log = fs::File::create(base.join("daemon.log")).unwrap();
        cmd.stdout(log.try_clone().unwrap()).stderr(log);
        drop(reservation);
        let mut child = cmd.spawn().unwrap();
        let client = reqwest::Client::new();
        timeout(Duration::from_secs(15), async {
            loop {
                assert!(
                    child.try_wait().unwrap().is_none(),
                    "{}",
                    fs::read_to_string(base.join("daemon.log")).unwrap()
                );
                let unix_ready = !unix || health_unix(&socket).await;
                let tcp_ready = !tcp
                    || match client.get(format!("http://{address}/healthz")).send().await {
                        Ok(response) => {
                            response.status().is_success()
                                && response.text().await.unwrap() == "{\"status\":\"ok\"}"
                        }
                        Err(_) => false,
                    };
                if unix_ready && tcp_ready {
                    break;
                }
                sleep(Duration::from_millis(20)).await;
            }
        })
        .await
        .unwrap();
        if unix && tcp {
            let local = pb::AuthServiceClient::new(channel(Endpoint::Unix {
                path: socket.clone(),
                prefix: "/api".into(),
            }));
            let remote = pb::AuthServiceClient::new(channel(Endpoint::Http(format!(
                "http://{address}/api"
            ))));
            assert_eq!(
                remote
                    .create_setup_code(Request::new(pb::CreateSetupCodeRequest::default()))
                    .await
                    .unwrap_err()
                    .code,
                Code::Unauthenticated
            );
            let setup = local
                .create_setup_code(Request::new(pb::CreateSetupCodeRequest {
                    token_name: "daemon-test".into(),
                    ..Default::default()
                }))
                .await
                .unwrap()
                .message;
            let exchange = remote
                .exchange_setup_code(Request::new(pb::ExchangeSetupCodeRequest {
                    setup_code: setup.setup_code.clone(),
                }))
                .await
                .unwrap()
                .message;
            assert!(!exchange.token.is_empty());
            assert_eq!(
                remote
                    .exchange_setup_code(Request::new(pb::ExchangeSetupCodeRequest {
                        setup_code: setup.setup_code
                    }))
                    .await
                    .unwrap_err()
                    .code,
                Code::PermissionDenied
            );
        }
        let contexts: construct::context::EndpointContexts =
            serde_yaml::from_slice(&fs::read(base.join("config/construct/context.yaml")).unwrap())
                .unwrap();
        let raw_address = if unix {
            socket.to_str().unwrap()
        } else {
            &address
        };
        let hash = Sha256::digest(raw_address.as_bytes());
        assert_eq!(
            contexts.current,
            format!(
                "{}-{:02x}{:02x}{:02x}",
                if unix { "unix" } else { "tcp" },
                hash[0],
                hash[1],
                hash[2]
            )
        );
        assert_eq!(
            contexts.current().unwrap().kind,
            if unix { "unix" } else { "http" }
        );
        assert_eq!(
            fs::metadata(base.join("data/construct"))
                .unwrap()
                .permissions()
                .mode()
                & 0o777,
            0o700
        );
        let key_path = base.join("data/construct/secrets/encryption_key");
        let key = fs::read(&key_path).unwrap();
        assert_eq!(
            fs::metadata(&key_path).unwrap().permissions().mode() & 0o777,
            0o600
        );
        assert!(base.join("data/construct/construct.db").exists());
        stop(&mut child, if tcp { libc::SIGTERM } else { libc::SIGINT }).await;
        if unix {
            assert!(!socket.exists());
        }
        assert_eq!(fs::read(key_path).unwrap(), key);
    }
}

#[tokio::test]
async fn startup_errors_and_missing_config_are_reported_without_hanging() {
    let dir = tempfile::tempdir().unwrap();
    let base = dir.path();
    let output = timeout(Duration::from_secs(10), command(base).output())
        .await
        .unwrap()
        .unwrap();
    assert!(!output.status.success());
    assert!(String::from_utf8_lossy(&output.stderr).contains("no valid listener has been detected"));
    file_config(base);
    fs::write(base.join("config/construct/config.yaml"), "secret: [").unwrap();
    let output = command(base).output().await.unwrap();
    assert!(!output.status.success());
    assert!(String::from_utf8_lossy(&output.stderr).contains("failed to parse config file"));
    file_config(base);
    let occupied = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    let socket = base.join("daemon.sock");
    let output = command(base)
        .arg("--listen-unix")
        .arg(&socket)
        .arg("--listen-http")
        .arg(occupied.local_addr().unwrap().to_string())
        .stdout(Stdio::piped())
        .output()
        .await
        .unwrap();
    assert!(!output.status.success());
    assert!(String::from_utf8_lossy(&output.stderr).contains("failed to listen on tcp"));
    assert!(
        !socket.exists(),
        "failed second bind must clean up first listener"
    );
}
