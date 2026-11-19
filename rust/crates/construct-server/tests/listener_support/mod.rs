use construct_connect::{Channel, ClientConfig, Endpoint};
use serde_json::Value;
use std::{
    fs::{self, File},
    os::fd::AsRawFd,
    path::{Path, PathBuf},
    time::Duration,
};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    process::{Child, Command},
    time::{timeout, Instant},
};

#[derive(Clone, Copy, Debug)]
pub enum Launch {
    Unix,
    Tcp,
    SystemdUnix,
    SystemdTcp,
}
impl Launch {
    pub fn unix(self) -> bool {
        matches!(self, Self::Unix | Self::SystemdUnix)
    }
    fn activated(self) -> bool {
        matches!(self, Self::SystemdUnix | Self::SystemdTcp)
    }
}
pub struct Host {
    child: Child,
    pub startup: Value,
    pub endpoint: Endpoint,
    log: PathBuf,
}
impl Host {
    pub async fn start(base: &Path, launch: Launch) -> Self {
        fs::create_dir_all(base).unwrap();
        let socket = base.join("api.sock");
        let log = base.join(format!("{launch:?}.log"));
        let mut command = if launch.activated() {
            let mut c = Command::new("/bin/sh");
            c.args([
                "-c",
                "export LISTEN_PID=$$; exec \"$@\"",
                "activation",
                env!("CARGO_BIN_EXE_construct-server"),
            ]);
            c
        } else {
            Command::new(env!("CARGO_BIN_EXE_construct-server"))
        };
        command
            .env_clear()
            .env("PATH", "/usr/local/bin:/usr/bin:/bin")
            .env("HOME", base)
            .env("XDG_DATA_HOME", base.join("xdg-data"));
        command
            .args(["--data-dir"])
            .arg(base.join("data"))
            .args(["--secret-provider", "file"]);
        let mut inherited_tcp = None;
        let mut inherited_unix = None;
        if launch.activated() {
            let fd = if launch.unix() {
                inherited_unix = Some(std::os::unix::net::UnixListener::bind(&socket).unwrap());
                inherited_unix.as_ref().unwrap().as_raw_fd()
            } else {
                inherited_tcp = Some(std::net::TcpListener::bind("127.0.0.1:0").unwrap());
                inherited_tcp.as_ref().unwrap().as_raw_fd()
            };
            command.env("LISTEN_FDS", "1");
            // The parent owns fd until spawn returns; only async-signal-safe syscalls after fork.
            unsafe {
                command.pre_exec(move || {
                    if libc::dup2(fd, 3) < 0 || libc::fcntl(3, libc::F_SETFD, 0) < 0 {
                        return Err(std::io::Error::last_os_error());
                    }
                    Ok(())
                });
            }
        } else if launch.unix() {
            command.arg("--listen-unix").arg(&socket);
        } else {
            command.args(["--listen-http", "127.0.0.1:0"]);
        }
        let output = File::create(&log).unwrap();
        let mut child = command
            .stdout(output.try_clone().unwrap())
            .stderr(output)
            .kill_on_drop(true)
            .spawn()
            .unwrap();
        drop(inherited_tcp);
        drop(inherited_unix);
        let until = Instant::now() + Duration::from_secs(15);
        let startup = loop {
            let content = fs::read_to_string(&log).unwrap();
            if let Some(value) = content
                .lines()
                .find_map(|line| serde_json::from_str::<Value>(line).ok())
            {
                break value;
            }
            assert!(
                child.try_wait().unwrap().is_none(),
                "server exited: {content}"
            );
            assert!(Instant::now() < until, "startup timed out: {content}");
            tokio::time::sleep(Duration::from_millis(10)).await;
        };
        assert_eq!(
            startup["activation"],
            if launch.activated() {
                "systemd"
            } else if launch.unix() {
                "unix"
            } else {
                "tcp"
            }
        );
        let endpoint = if launch.unix() {
            Endpoint::Unix {
                path: socket,
                prefix: "/api".into(),
            }
        } else {
            Endpoint::Http(format!(
                "http://{}/api",
                startup["address"].as_str().unwrap()
            ))
        };
        let host = Self {
            child,
            startup,
            endpoint,
            log,
        };
        assert_eq!(host.health().await, "{\"status\":\"ok\"}");
        host
    }
    pub fn channel(&self) -> Channel {
        let mut config = ClientConfig::new(self.endpoint.clone());
        config.timeout = Some(Duration::from_secs(5));
        Channel::new(config).unwrap()
    }
    pub async fn health(&self) -> String {
        let request = b"GET /healthz HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n";
        async fn exchange(
            mut io: impl tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin,
            request: &[u8],
        ) -> String {
            io.write_all(request).await.unwrap();
            let mut result = String::new();
            timeout(Duration::from_secs(5), io.read_to_string(&mut result))
                .await
                .unwrap()
                .unwrap();
            assert!(result.starts_with("HTTP/1.1 200"), "{result}");
            result.split_once("\r\n\r\n").unwrap().1.to_owned()
        }
        match &self.endpoint {
            Endpoint::Unix { path, .. } => {
                exchange(
                    tokio::net::UnixStream::connect(path).await.unwrap(),
                    request,
                )
                .await
            }
            Endpoint::Http(_) => {
                exchange(
                    tokio::net::TcpStream::connect(self.startup["address"].as_str().unwrap())
                        .await
                        .unwrap(),
                    request,
                )
                .await
            }
        }
    }
    pub async fn stop(mut self) {
        let pid = self.child.id().unwrap();
        // Signal only the child process owned by this fixture.
        assert_eq!(unsafe { libc::kill(pid as i32, libc::SIGTERM) }, 0);
        let status = timeout(Duration::from_secs(10), self.child.wait())
            .await
            .expect("graceful shutdown timed out")
            .unwrap();
        assert!(
            status.success(),
            "server failed: {status}: {}",
            fs::read_to_string(&self.log).unwrap()
        );
    }
}
