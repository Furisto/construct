//! Explicit and platform-activated listeners, independent of the API router.
use construct_connect::{async_trait, Transport};
use sha2::{Digest, Sha256};
#[cfg(unix)]
use std::path::PathBuf;
use std::{fmt, io};
use tokio::net::TcpListener;
#[cfg(unix)]
use tokio::net::UnixListener;

/// How a listener was acquired, not the transport of its accepted connections.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ActivationType {
    /// Explicit Unix socket.
    Unix,
    /// Explicit TCP address.
    Tcp,
    /// Linux FD-3 activation (Unix or TCP).
    Systemd,
    /// macOS launchd activation (Unix or TCP).
    Launchd,
}
impl ActivationType {
    /// Go's provider activation type string.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Unix => "unix",
            Self::Tcp => "tcp",
            Self::Systemd => "systemd",
            Self::Launchd => "launchd",
        }
    }
    /// Explicit-launch context kind accepted by Go's EndpointContext.Validate.
    pub fn context_kind(self) -> Option<&'static str> {
        match self {
            Self::Unix => Some("unix"),
            Self::Tcp => Some("http"),
            _ => None,
        }
    }
    /// Build a context from the bound listener address, hashing its raw Go spelling.
    /// HTTP clients require the URL scheme even though TCP listener addresses lack it.
    pub fn endpoint_context(self, address: &str) -> Option<EndpointContext> {
        let kind = self.context_kind()?;
        let hash = Sha256::digest(address.as_bytes());
        Some(EndpointContext {
            name: format!("{kind}-{:02x}{:02x}{:02x}", hash[0], hash[1], hash[2]),
            kind,
            address: if self == Self::Tcp {
                format!("http://{address}")
            } else {
                address.into()
            },
        })
    }
}
impl fmt::Display for ActivationType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}
/// Context metadata; no credentials are persisted by the listener layer.
#[derive(Debug, PartialEq, Eq)]
pub struct EndpointContext {
    /// `<kind>-<first three bytes of SHA-256(raw listener address), hex>`.
    pub name: String,
    /// `unix` or `http`, never `tcp`.
    pub kind: &'static str,
    /// Absolute Unix path or HTTP URL including its scheme.
    pub address: String,
}
/// An owned, nonblocking listener; transport is determined by the actual socket family.
#[derive(Debug)]
pub enum Listener {
    /// Unix-domain listener, including activated sockets.
    #[cfg(unix)]
    Unix(UnixListener),
    /// IPv4/IPv6 listener, including activated sockets.
    Tcp(TcpListener),
}
impl Listener {
    /// Connection trust classification, independent of activation mechanism.
    pub fn transport(&self) -> Transport {
        match self {
            #[cfg(unix)]
            Self::Unix(_) => Transport::Unix,
            Self::Tcp(_) => Transport::Tcp,
        }
    }
    /// Bound address in Go net.Listener.Addr().String() form.
    pub fn address(&self) -> io::Result<String> {
        match self {
            Self::Tcp(l) => Ok(l.local_addr()?.to_string()),
            #[cfg(unix)]
            Self::Unix(l) => {
                let address = l.local_addr()?;
                if let Some(path) = address.as_pathname() {
                    return Ok(path.to_string_lossy().into_owned());
                }
                #[cfg(target_os = "linux")]
                {
                    if let Some(name) = address.as_abstract_name() {
                        return Ok(format!("@{}", String::from_utf8_lossy(name)));
                    }
                }
                Ok("@".into())
            }
        }
    }
}
/// Listener creation/detection error, retaining Go's diagnostic prefixes.
#[derive(Debug, thiserror::Error)]
#[error("{0}")]
pub struct Error(String);
fn error(message: impl Into<String>) -> Error {
    Error(message.into())
}
fn context(prefix: &str, source: impl fmt::Display) -> Error {
    error(format!("{prefix}: {source}"))
}

/// Listener acquisition and explicit socket-path cleanup.
#[async_trait]
pub trait Provider: Send + Sync {
    /// Acquire a listener; each returned value owns its socket.
    async fn create(&self) -> Result<Listener, Error>;
    /// How the listener is acquired (not a trust classification).
    fn activation_type(&self) -> ActivationType;
    /// Cleanup after the owned listener has been closed.
    fn close(&self) -> Result<(), Error> {
        Ok(())
    }
}
/// Explicit Unix socket, with Go's replacement and 0666 permission behavior.
#[cfg(unix)]
pub struct UnixSocketProvider {
    path: PathBuf,
}
#[cfg(unix)]
impl UnixSocketProvider {
    /// Select a Unix socket path; its parent directory must already exist.
    pub fn new(path: impl Into<PathBuf>) -> Self {
        Self { path: path.into() }
    }
}
#[async_trait]
#[cfg(unix)]
impl Provider for UnixSocketProvider {
    async fn create(&self) -> Result<Listener, Error> {
        use std::os::unix::fs::PermissionsExt;
        let _ = std::fs::remove_file(&self.path);
        let listener = UnixListener::bind(&self.path)
            .map_err(|e| context("failed to listen on unix socket", e))?;
        std::fs::set_permissions(&self.path, std::fs::Permissions::from_mode(0o666))
            .map_err(|e| context("failed to set socket permissions", e))?;
        Ok(Listener::Unix(listener))
    }
    fn activation_type(&self) -> ActivationType {
        ActivationType::Unix
    }
    fn close(&self) -> Result<(), Error> {
        std::fs::remove_file(&self.path).map_err(|e| context("remove unix socket", e))
    }
}
/// Explicit TCP listener, supporting DNS, IPv4, and IPv6 addresses.
pub struct TcpProvider {
    address: String,
}
impl TcpProvider {
    /// Select an address in Go's `host:port` form (not a URL).
    pub fn new(address: impl Into<String>) -> Self {
        Self {
            address: address.into(),
        }
    }
}
#[async_trait]
impl Provider for TcpProvider {
    async fn create(&self) -> Result<Listener, Error> {
        let address = if self.address.starts_with(':') {
            format!("0.0.0.0{}", self.address)
        } else {
            self.address.clone()
        };
        TcpListener::bind(address)
            .await
            .map(Listener::Tcp)
            .map_err(|e| context("failed to listen on tcp", e))
    }
    fn activation_type(&self) -> ActivationType {
        ActivationType::Tcp
    }
}

#[cfg(any(target_os = "linux", target_os = "macos"))]
fn inherited_listener(fd: std::os::fd::OwnedFd) -> io::Result<Listener> {
    use std::{mem, os::fd::AsRawFd};
    let mut socket_type: libc::c_int = 0;
    let mut type_len = mem::size_of_val(&socket_type) as libc::socklen_t;
    // SO_TYPE must describe a connection-oriented socket, as net.FileListener requires.
    if unsafe {
        libc::getsockopt(
            fd.as_raw_fd(),
            libc::SOL_SOCKET,
            libc::SO_TYPE,
            (&mut socket_type as *mut libc::c_int).cast(),
            &mut type_len,
        )
    } != 0
    {
        return Err(io::Error::last_os_error());
    }
    if socket_type != libc::SOCK_STREAM && socket_type != libc::SOCK_SEQPACKET {
        return Err(io::Error::from_raw_os_error(libc::EINVAL));
    }
    let mut address = mem::MaybeUninit::<libc::sockaddr_storage>::zeroed();
    let mut len = mem::size_of::<libc::sockaddr_storage>() as libc::socklen_t;
    // getsockname initializes the family and at most `len` bytes of the supplied storage.
    if unsafe { libc::getsockname(fd.as_raw_fd(), address.as_mut_ptr().cast(), &mut len) } != 0 {
        return Err(io::Error::last_os_error());
    }
    let family = unsafe { address.assume_init() }.ss_family as i32;
    match family {
        libc::AF_UNIX => {
            let listener = std::os::unix::net::UnixListener::from(fd);
            listener.set_nonblocking(true)?;
            UnixListener::from_std(listener).map(Listener::Unix)
        }
        libc::AF_INET | libc::AF_INET6 => {
            let listener = std::net::TcpListener::from(fd);
            listener.set_nonblocking(true)?;
            TcpListener::from_std(listener).map(Listener::Tcp)
        }
        _ => Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            "unsupported socket family",
        )),
    }
}

/// Linux systemd socket activation provider, using inherited FD 3.
#[cfg(target_os = "linux")]
pub struct SystemdSocketProvider;
#[cfg(target_os = "linux")]
fn listen_fds(value: &str) -> Result<i64, Error> {
    if value.is_empty() {
        return Err(error("no LISTEN_FDS environment variable from systemd"));
    }
    let count = value.parse::<i64>().map_err(|e| {
        use std::num::IntErrorKind;
        let reason = if matches!(
            e.kind(),
            IntErrorKind::PosOverflow | IntErrorKind::NegOverflow
        ) {
            "value out of range"
        } else {
            "invalid syntax"
        };
        error(format!(
            "invalid LISTEN_FDS value: strconv.Atoi: parsing {value:?}: {reason}"
        ))
    })?;
    if count < 1 {
        return Err(error("no sockets passed from systemd"));
    }
    Ok(count)
}
#[cfg(target_os = "linux")]
#[async_trait]
impl Provider for SystemdSocketProvider {
    async fn create(&self) -> Result<Listener, Error> {
        use std::{
            mem::MaybeUninit,
            os::fd::{BorrowedFd, OwnedFd},
        };
        listen_fds(&std::env::var("LISTEN_FDS").unwrap_or_default())?;
        let mut stat = MaybeUninit::<libc::stat>::uninit();
        // fstat only writes to initialized storage on success; no ownership of FD 3 is taken.
        if unsafe { libc::fstat(3, stat.as_mut_ptr()) } != 0
            || unsafe { stat.assume_init() }.st_mode & libc::S_IFMT != libc::S_IFSOCK
        {
            return Err(error("file descriptor 3 is not a socket"));
        }
        // Duplicate rather than stealing a descriptor owned by the process activation environment.
        let fd: OwnedFd = unsafe { BorrowedFd::borrow_raw(3) }
            .try_clone_to_owned()
            .map_err(|e| context("failed to create listener from systemd socket", e))?;
        inherited_listener(fd)
            .map_err(|e| context("failed to create listener from systemd socket", e))
    }
    fn activation_type(&self) -> ActivationType {
        ActivationType::Systemd
    }
}
/// Whether Linux activation variables identify this process and at least one socket.
#[cfg(target_os = "linux")]
pub fn is_systemd_socket_activation() -> bool {
    listen_fds(&std::env::var("LISTEN_FDS").unwrap_or_default()).is_ok()
        && std::env::var("LISTEN_PID").is_ok_and(|pid| pid == std::process::id().to_string())
}

/// macOS launchd provider for the `Listeners` socket group.
#[cfg(target_os = "macos")]
pub struct LaunchdSocketProvider;
/// Whether this process was launched by a Construct launchd service.
#[cfg(target_os = "macos")]
pub fn is_launchd_socket_activation() -> bool {
    std::env::var("XPC_SERVICE_NAME").is_ok_and(|name| name.starts_with("sh.construct.daemon."))
}
#[cfg(target_os = "macos")]
#[async_trait]
impl Provider for LaunchdSocketProvider {
    async fn create(&self) -> Result<Listener, Error> {
        use std::{
            os::fd::{FromRawFd, OwnedFd},
            ptr,
        };
        #[link(name = "System")]
        unsafe extern "C" {
            fn launch_activate_socket(
                name: *const libc::c_char,
                fds: *mut *mut libc::c_int,
                count: *mut libc::size_t,
            ) -> libc::c_int;
        }
        let mut fds = ptr::null_mut();
        let mut count = 0;
        // libSystem allocates an array and transfers ownership of all returned descriptors.
        let status = unsafe { launch_activate_socket(c"Listeners".as_ptr(), &mut fds, &mut count) };
        if status != 0 {
            return Err(context(
                "failed to activate launchd socket",
                io::Error::from_raw_os_error(status),
            ));
        }
        if count == 0 || fds.is_null() {
            unsafe {
                libc::free(fds.cast());
            }
            return Err(error(
                "failed to activate launchd socket: no sockets passed from launchd",
            ));
        }
        let descriptors: Vec<OwnedFd> = unsafe { std::slice::from_raw_parts(fds, count) }
            .iter()
            .map(|&fd| unsafe { OwnedFd::from_raw_fd(fd) })
            .collect();
        unsafe {
            libc::free(fds.cast());
        }
        let first = descriptors
            .into_iter()
            .next()
            .expect("nonempty launchd descriptor array");
        inherited_listener(first).map_err(|e| context("failed to activate launchd socket", e))
    }
    fn activation_type(&self) -> ActivationType {
        ActivationType::Launchd
    }
}

/// Select Unix flag, HTTP flag, then platform activation, in Go's precedence order.
pub fn detect_provider(http_address: &str, unix_socket: &str) -> Result<Box<dyn Provider>, Error> {
    if !unix_socket.is_empty() {
        #[cfg(unix)]
        return Ok(Box::new(UnixSocketProvider::new(unix_socket)));
        #[cfg(not(unix))]
        return Err(error(
            "Unix sockets are unavailable on this platform; use --listen-http",
        ));
    }
    if !http_address.is_empty() {
        return Ok(Box::new(TcpProvider::new(http_address)));
    }
    #[cfg(target_os = "linux")]
    if is_systemd_socket_activation() {
        return Ok(Box::new(SystemdSocketProvider));
    }
    #[cfg(target_os = "macos")]
    if is_launchd_socket_activation() {
        return Ok(Box::new(LaunchdSocketProvider));
    }
    #[cfg(not(unix))]
    return Err(error(
        "no valid listener has been detected; specify --listen-http HOST:PORT",
    ));
    #[cfg(unix)]
    {
        let activation = if cfg!(target_os = "macos") {
            "launchd"
        } else {
            "systemd"
        };
        Err(error(format!("no valid listener has been detected. Specify either a unix socket, tcp address or use {activation} socket activation")))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[cfg(any(target_os = "linux", target_os = "macos"))]
    #[tokio::test]
    async fn inherited_socket_families_and_datagram_rejection() {
        let udp = std::net::UdpSocket::bind("127.0.0.1:0").unwrap();
        assert_eq!(
            inherited_listener(udp.into()).unwrap_err().raw_os_error(),
            Some(libc::EINVAL)
        );
        let tcp = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        assert_eq!(
            inherited_listener(tcp.into()).unwrap().transport(),
            Transport::Tcp
        );
        if let Ok(tcp6) = std::net::TcpListener::bind("[::1]:0") {
            let listener = inherited_listener(tcp6.into()).unwrap();
            assert_eq!(listener.transport(), Transport::Tcp);
            assert!(listener.address().unwrap().starts_with("[::1]:"));
        }
    }
    #[test]
    fn explicit_precedence_and_contexts() {
        #[cfg(unix)]
        assert_eq!(
            detect_provider("bad:123", "/example.sock")
                .unwrap()
                .activation_type(),
            ActivationType::Unix
        );
        #[cfg(not(unix))]
        {
            assert!(detect_provider("127.0.0.1:0", "/example.sock").is_err());
            assert!(detect_provider("", "").is_err());
        }
        assert_eq!(
            detect_provider("127.0.0.1:0", "")
                .unwrap()
                .activation_type(),
            ActivationType::Tcp
        );
        let context = ActivationType::Tcp
            .endpoint_context("127.0.0.1:1234")
            .unwrap();
        assert_eq!(context.kind, "http");
        assert_eq!(context.address, "http://127.0.0.1:1234");
        assert_eq!(
            context.name,
            format!(
                "http-{}",
                Sha256::digest(b"127.0.0.1:1234")[..3]
                    .iter()
                    .map(|b| format!("{b:02x}"))
                    .collect::<String>()
            )
        );
        let context = ActivationType::Unix.endpoint_context("/a.sock").unwrap();
        assert_eq!(context.kind, "unix");
        assert_eq!(context.address, "/a.sock");
        assert!(ActivationType::Systemd
            .endpoint_context("127.0.0.1:1234")
            .is_none());
        assert_eq!(ActivationType::Launchd.to_string(), "launchd");
    }
    #[cfg(target_os = "linux")]
    #[test]
    fn systemd_count_errors_match_go() {
        for (value, message) in [("", "no LISTEN_FDS environment variable from systemd"), ("abc", "invalid LISTEN_FDS value: strconv.Atoi: parsing \"abc\": invalid syntax"), ("0", "no sockets passed from systemd"), ("-1", "no sockets passed from systemd"), ("999999999999999999999999", "invalid LISTEN_FDS value: strconv.Atoi: parsing \"999999999999999999999999\": value out of range")] {
            assert_eq!(listen_fds(value).unwrap_err().to_string(), message);
        }
        assert_eq!(listen_fds("+2").unwrap(), 2);
    }
    #[cfg(unix)]
    #[tokio::test]
    async fn explicit_unix_creation_errors_permissions_and_close() {
        use std::os::unix::fs::PermissionsExt;
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("api.sock");
        std::fs::write(&path, "stale").unwrap();
        let provider = UnixSocketProvider::new(&path);
        let listener = provider.create().await.unwrap();
        assert_eq!(listener.transport(), Transport::Unix);
        assert_eq!(listener.address().unwrap(), path.to_str().unwrap());
        assert_eq!(
            std::fs::metadata(&path).unwrap().permissions().mode() & 0o777,
            0o666
        );
        drop(listener);
        provider.close().unwrap();
        assert!(!path.exists());
        assert!(UnixSocketProvider::new(dir.path().join("missing/api.sock"))
            .create()
            .await
            .unwrap_err()
            .to_string()
            .starts_with("failed to listen on unix socket: "));
    }
    #[tokio::test]
    async fn explicit_tcp_creation_and_errors() {
        assert!(TcpProvider::new("http://invalid")
            .create()
            .await
            .unwrap_err()
            .to_string()
            .starts_with("failed to listen on tcp: "));
        let listener = TcpProvider::new("127.0.0.1:0").create().await.unwrap();
        assert_eq!(listener.transport(), Transport::Tcp);
        assert!(listener.address().unwrap().starts_with("127.0.0.1:"));
    }
}
