#![cfg(target_os = "linux")]
mod listener_support;
use construct_connect::{Code, Request};
use construct_proto::construct::v1 as pb;
use futures_util::StreamExt;
use listener_support::{Host, Launch};
use std::{fs, os::unix::fs::PermissionsExt};
use tokio::process::Command;

#[tokio::test]
async fn servers_health_prefix_trust_activation_and_graceful_shutdown() {
    for mode in [
        Launch::Unix,
        Launch::Tcp,
        Launch::SystemdUnix,
        Launch::SystemdTcp,
    ] {
        let dir = tempfile::tempdir().unwrap();
        let host = Host::start(dir.path(), mode).await;
        let c = pb::AuthServiceClient::new(host.channel());
        let response = c
            .create_setup_code(Request::new(pb::CreateSetupCodeRequest {
                token_name: "listener-test".into(),
                ..Default::default()
            }))
            .await;
        if mode.unix() {
            assert_eq!(response.unwrap().message.setup_code.len(), 9);
        } else {
            assert_eq!(response.unwrap_err().code, Code::Unauthenticated);
            assert_eq!(
                c.exchange_setup_code(Request::new(pb::ExchangeSetupCodeRequest::default()))
                    .await
                    .unwrap_err()
                    .code,
                Code::InvalidArgument
            );
            assert_eq!(
                c.exchange_setup_code(Request::new(pb::ExchangeSetupCodeRequest {
                    setup_code: "invalid".into()
                }))
                .await
                .unwrap_err()
                .code,
                Code::PermissionDenied
            );
        }
        if let Launch::Tcp = mode {
            assert_eq!(host.startup["context"]["kind"], "http");
            assert!(host.startup["context"]["address"]
                .as_str()
                .unwrap()
                .starts_with("http://127.0.0.1:"));
            assert!(host.startup["context"]["name"]
                .as_str()
                .unwrap()
                .starts_with("http-"));
        }
        let key = dir.path().join("data/secrets/encryption_key");
        assert_eq!(
            fs::metadata(key).unwrap().permissions().mode() & 0o777,
            0o600
        );
        let stream = if mode.unix() {
            Some(
                pb::EventServiceClient::new(host.channel())
                    .subscribe(Request::new(pb::EventSubscribeRequest::default()))
                    .await
                    .unwrap()
                    .message,
            )
        } else {
            None
        };
        host.stop().await;
        if let Some(mut stream) = stream {
            assert!(stream.next().await.is_none());
        }
        if matches!(mode, Launch::Unix) {
            assert!(!dir.path().join("api.sock").exists());
        }
    }
}

#[tokio::test]
async fn isolated_activation_detection_and_creation_errors() {
    let cases = [
        (None, None, "no valid listener has been detected. Specify either a unix socket, tcp address or use systemd socket activation"),
        (Some("0"), Some("self"), "no valid listener has been detected. Specify either a unix socket, tcp address or use systemd socket activation"),
        (Some("abc"), Some("self"), "no valid listener has been detected. Specify either a unix socket, tcp address or use systemd socket activation"),
        (Some("1"), Some("0"), "no valid listener has been detected. Specify either a unix socket, tcp address or use systemd socket activation"),
        (Some("1"), Some("self"), "file descriptor 3 is not a socket"),
    ];
    for (fds, pid, expected) in cases {
        let dir = tempfile::tempdir().unwrap();
        let mut c = Command::new("/bin/sh");
        c.env_clear()
            .env("PATH", "/usr/bin:/bin")
            .env("HOME", dir.path());
        c.args(["-c", "if [ \"$LISTEN_PID\" = self ]; then export LISTEN_PID=$$; fi; exec 3</dev/null; exec \"$@\"", "activation-errors", env!("CARGO_BIN_EXE_construct-server")]);
        c.arg("--data-dir").arg(dir.path().join("data"));
        if let Some(fds) = fds {
            c.env("LISTEN_FDS", fds);
        }
        if let Some(pid) = pid {
            c.env("LISTEN_PID", pid);
        }
        let out = c.output().await.unwrap();
        assert!(!out.status.success());
        assert!(
            String::from_utf8_lossy(&out.stderr).contains(expected),
            "{}",
            String::from_utf8_lossy(&out.stderr)
        );
    }
    let out = Command::new(env!("CARGO_BIN_EXE_construct-server"))
        .arg("--help")
        .output()
        .await
        .unwrap();
    assert!(out.status.success());
    assert!(String::from_utf8_lossy(&out.stdout).contains("--listen-http"));
}
