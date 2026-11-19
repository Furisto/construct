use crate::{
    app::{ApiClient, App},
    context::EndpointContext,
    helpers::confirm,
    paths::UserInfo,
    Result,
};
use clap::Args;
use construct_connect::{Code, Request};
use construct_proto::construct::v1::ListModelProvidersRequest;
use std::{
    fs,
    io::{self, Write},
    net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr},
    path::{Path, PathBuf},
    process::Output,
    time::Duration,
};
use tokio::process::Command;

#[derive(Args)]
pub(super) struct Install {
    #[arg(short, long)]
    force: bool,
    #[arg(long)]
    always_running: bool,
    #[arg(long, value_name = "ADDRESS")]
    listen_http: Option<String>,
    #[arg(short, long)]
    quiet: bool,
    #[arg(short, long, default_value = "default")]
    name: String,
    #[command(flatten)]
    scope: Scope,
}

#[derive(Args)]
pub(super) struct Uninstall {
    #[arg(short, long)]
    yes: bool,
    #[arg(short, long)]
    quiet: bool,
    #[command(flatten)]
    scope: Scope,
}

#[derive(Args)]
pub(super) struct Scope {
    /// Use the system service manager instead of the user service manager.
    #[arg(short, long)]
    system: bool,
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum Platform {
    Linux,
    MacOs,
}

impl Platform {
    fn current() -> Result<Self> {
        match std::env::consts::OS {
            "linux" => Ok(Self::Linux),
            "macos" => Ok(Self::MacOs),
            os => Err(format!("unsupported operating system: {os}").into()),
        }
    }
}

const SOCKET_UNIT: &str = "construct.socket";
const SERVICE_UNIT: &str = "construct.service";

struct ServiceManager {
    platform: Platform,
    system: bool,
    dir: PathBuf,
    uid: u32,
}

impl ServiceManager {
    fn new(user: &UserInfo, scope: Scope) -> Result<Self> {
        let platform = Platform::current()?;
        #[cfg(unix)]
        let uid = unsafe { libc::geteuid() };
        #[cfg(not(unix))]
        let uid = u32::MAX;
        if scope.system && uid != 0 {
            return Err("system service management requires root privileges".into());
        }
        let dir = match (platform, scope.system) {
            (Platform::Linux, true) => "/etc/systemd/system".into(),
            (Platform::Linux, false) => user.home.join(".config/systemd/user"),
            (Platform::MacOs, true) => "/Library/LaunchDaemons".into(),
            (Platform::MacOs, false) => user.home.join("Library/LaunchAgents"),
        };
        Ok(Self {
            platform,
            system: scope.system,
            dir,
            uid,
        })
    }

    fn installed(&self) -> Result<Vec<PathBuf>> {
        if self.platform == Platform::Linux {
            let mut files = Vec::new();
            for name in [SOCKET_UNIT, SERVICE_UNIT] {
                let path = self.dir.join(name);
                if exists(&path)? {
                    files.push(path);
                }
            }
            return Ok(files);
        }
        let entries = match fs::read_dir(&self.dir) {
            Ok(entries) => entries,
            Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(Vec::new()),
            Err(e) => return Err(e.into()),
        };
        let mut files = Vec::new();
        for entry in entries {
            let entry = entry?;
            if entry
                .path()
                .file_name()
                .and_then(|f| f.to_str())
                .and_then(plist_name)
                .is_some()
                && entry.file_type()?.is_file()
            {
                files.push(entry.path());
            }
        }
        files.sort();
        Ok(files)
    }

    fn for_removal(user: &UserInfo, scope: Scope) -> Result<(Self, Vec<PathBuf>)> {
        let mut manager = Self::new(user, scope)?;
        let mut paths = manager.installed()?;
        if paths.is_empty() && !manager.system && manager.uid == 0 {
            manager = Self::new(user, Scope { system: true })?;
            paths = manager.installed()?;
        }
        Ok((manager, paths))
    }

    async fn systemctl(&self, args: &[&str]) -> Result<()> {
        let mut command = Command::new("systemctl");
        if !self.system {
            command.arg("--user");
        }
        checked(command.args(args)).await
    }

    fn launchd_domain(&self) -> String {
        if self.system {
            "system".into()
        } else {
            format!("gui/{}", self.uid)
        }
    }

    async fn bootstrap(&self, path: &Path) -> Result<()> {
        checked(
            Command::new("launchctl")
                .arg("bootstrap")
                .arg(self.launchd_domain())
                .arg(path),
        )
        .await
    }

    async fn stop(&self, paths: &[PathBuf]) -> Result<()> {
        if self.platform == Platform::Linux {
            let names = unit_names(paths);
            let mut args = vec!["stop"];
            args.extend(names);
            return self.systemctl(&args).await;
        }
        for path in paths {
            let name = plist_name(
                path.file_name()
                    .and_then(|f| f.to_str())
                    .unwrap_or_default(),
            )
            .ok_or("invalid Construct service filename")?;
            let domain = self.launchd_domain();
            let label = format!("{domain}/sh.construct.daemon.{name}");
            let mut command = Command::new("launchctl");
            command.arg("bootout").arg(&domain).arg(path);
            let output = command.kill_on_drop(true).output().await?;
            if !output.status.success() {
                // launchd can report a generic I/O error when a known plist is no longer loaded.
                let status = Command::new("launchctl")
                    .args(["print", &label])
                    .kill_on_drop(true)
                    .output()
                    .await?;
                let missing = String::from_utf8_lossy(&status.stderr)
                    .to_ascii_lowercase()
                    .contains("could not find service");
                if !missing {
                    return command_result(&command, output);
                }
            }
        }
        Ok(())
    }

    async fn remove(&self, paths: &[PathBuf]) -> Result<()> {
        self.stop(paths).await?;
        if self.platform == Platform::Linux {
            let mut args = vec!["disable"];
            args.extend(unit_names(paths));
            self.systemctl(&args).await?;
        }
        for path in paths {
            fs::remove_file(path)
                .map_err(|err| format!("failed to remove {}: {err}", path.display()))?;
        }
        if self.platform == Platform::Linux {
            self.systemctl(&["daemon-reload"]).await?;
        }
        Ok(())
    }
}

fn exists(path: &Path) -> io::Result<bool> {
    match fs::symlink_metadata(path) {
        Ok(_) => Ok(true),
        Err(e) if e.kind() == io::ErrorKind::NotFound => Ok(false),
        Err(e) => Err(e),
    }
}

fn unit_names(paths: &[PathBuf]) -> impl Iterator<Item = &str> {
    paths
        .iter()
        .filter_map(|path| path.file_name().and_then(|name| name.to_str()))
}

async fn checked(command: &mut Command) -> Result<()> {
    let output = command.kill_on_drop(true).output().await?;
    command_result(command, output)
}

fn command_result(command: &Command, output: Output) -> Result<()> {
    if output.status.success() {
        return Ok(());
    }
    let cmd = command.as_std();
    let command_line = std::iter::once(cmd.get_program())
        .chain(cmd.get_args())
        .map(|arg| arg.to_string_lossy())
        .collect::<Vec<_>>()
        .join(" ");
    let detail = if output.stderr.is_empty() {
        &output.stdout
    } else {
        &output.stderr
    };
    Err(format!(
        "{command_line} failed ({}): {}",
        output.status,
        String::from_utf8_lossy(detail).trim()
    )
    .into())
}

fn valid_name(name: &str) -> bool {
    !name.is_empty()
        && name
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"_.-".contains(&b))
}

fn plist_name(filename: &str) -> Option<&str> {
    filename
        .strip_prefix("construct-")?
        .strip_suffix(".plist")
        .filter(|name| valid_name(name))
}

enum Listener {
    Unix(PathBuf),
    Http(SocketAddr),
}

impl Listener {
    fn new(user: &UserInfo, system: bool, http: Option<&str>) -> Result<Self> {
        if let Some(address) = http.filter(|s| !s.is_empty()) {
            let invalid = || {
                format!("invalid HTTP listen address {address:?}: expected an IP address and port (such as 127.0.0.1:8080)")
            };
            let socket = if let Ok(parsed) = address.parse::<SocketAddr>() {
                parsed
            } else if let Some(port) = address.strip_prefix("localhost:") {
                SocketAddr::from((
                    Ipv4Addr::LOCALHOST,
                    port.parse::<u16>().map_err(|_| invalid())?,
                ))
            } else {
                let port = address
                    .strip_prefix(':')
                    .unwrap_or(address)
                    .parse::<u16>()
                    .map_err(|_| invalid())?;
                SocketAddr::from((Ipv4Addr::UNSPECIFIED, port))
            };
            if socket.port() == 0 || matches!(socket, SocketAddr::V6(v6) if v6.scope_id() != 0) {
                return Err(invalid().into());
            }
            return Ok(Self::Http(socket));
        }
        let runtime = if system {
            Path::new("/var/run/construct")
        } else {
            &user.runtime
        };
        Ok(Self::Unix(runtime.join("construct.sock")))
    }

    fn context(&self) -> Result<EndpointContext> {
        let (kind, address) = match self {
            Self::Unix(path) => ("unix", path_text(path)?.to_owned()),
            Self::Http(socket) => {
                let mut endpoint = *socket;
                match socket.ip() {
                    IpAddr::V4(ip) if ip.is_unspecified() => {
                        endpoint.set_ip(Ipv4Addr::LOCALHOST.into())
                    }
                    IpAddr::V6(ip) if ip.is_unspecified() => {
                        endpoint.set_ip(Ipv6Addr::LOCALHOST.into())
                    }
                    _ => {}
                }
                ("http", format!("http://{endpoint}"))
            }
        };
        let context = EndpointContext {
            kind: kind.into(),
            address,
            auth: None,
        };
        context.validate()?;
        Ok(context)
    }
}

fn path_text(path: &Path) -> Result<&str> {
    path.to_str()
        .filter(|s| !s.chars().any(char::is_control))
        .ok_or_else(|| format!("invalid path: {}", path.display()).into())
}

fn linux_units(
    exec: &Path,
    listener: &Listener,
    always_running: bool,
    system: bool,
) -> Result<(String, String)> {
    let (address, permissions) = match listener {
        Listener::Unix(path) => {
            let path = path_text(path)?;
            if path.contains('\\') {
                return Err("systemd socket path must not contain a backslash".into());
            }
            (
                path.replace('%', "%%"),
                "SocketMode=0660\nDirectoryMode=0700\n",
            )
        }
        Listener::Http(address) => (address.to_string(), "ReusePort=true\n"),
    };
    // The socket stays available across service restarts, but is stopped explicitly by the CLI.
    let socket = format!(
        "[Unit]\nDescription=Construct Socket\n\n[Socket]\nListenStream={address}\nAccept=false\nRemoveOnStop=true\n{permissions}\n[Install]\nWantedBy=sockets.target\n"
    );
    let exec = path_text(exec)?
        .replace('\\', "\\\\")
        .replace('"', "\\\"")
        .replace('%', "%%");
    let restart = if always_running {
        "always"
    } else {
        "on-failure"
    };
    let mut service = format!(
        "[Unit]\nDescription=Construct Daemon\nRequires={SOCKET_UNIT}\nAfter={SOCKET_UNIT}\n\n[Service]\nType=simple\nExecStart=:\"{exec}\" daemon run\nRestart={restart}\nRestartSec=2s\nProtectSystem=true\n"
    );
    if always_running {
        let target = if system {
            "multi-user.target"
        } else {
            "default.target"
        };
        service.push_str(&format!("\n[Install]\nWantedBy={target}\n"));
    }
    Ok((socket, service))
}

fn xml(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
}

fn macos_unit(
    exec: &Path,
    listener: &Listener,
    log: &Path,
    name: &str,
    keep_alive: bool,
) -> Result<String> {
    let exec = xml(path_text(exec)?);
    let log = xml(path_text(log)?);
    let socket = match listener {
        Listener::Unix(path) => {
            let path = xml(path_text(path)?);
            format!("<key>SockPathName</key><string>{path}</string><key>SockPathMode</key><integer>432</integer>")
        }
        Listener::Http(address) => {
            let family = if address.is_ipv4() { "IPv4" } else { "IPv6" };
            let node = address.ip();
            let port = address.port();
            format!("<key>SockNodeName</key><string>{node}</string><key>SockServiceName</key><string>{port}</string><key>SockFamily</key><string>{family}</string>")
        }
    };
    Ok(format!(
        r#"<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>sh.construct.daemon.{name}</string>
    <key>ProgramArguments</key><array><string>{exec}</string><string>daemon</string><string>run</string></array>
    <key>Sockets</key><dict><key>Listeners</key><dict>
        <key>SockType</key><string>stream</string>{socket}
    </dict></dict>
    <key>KeepAlive</key><{keep_alive}/>
    <key>ExitTimeOut</key><integer>60</integer>
    <key>StandardOutPath</key><string>{log}/construct-{name}.out</string>
    <key>StandardErrorPath</key><string>{log}/construct-{name}.err</string>
</dict>
</plist>
"#
    ))
}

fn write_unit(path: &Path, content: &str, force: bool) -> Result<()> {
    let mut file = tempfile::NamedTempFile::new_in(path.parent().expect("unit has parent"))?;
    file.write_all(content.as_bytes())?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        file.as_file()
            .set_permissions(fs::Permissions::from_mode(0o644))?;
    }
    if force {
        file.persist(path)?;
    } else {
        file.persist_noclobber(path)?;
    }
    Ok(())
}

pub(super) async fn install(app: &mut App, options: Install) -> Result<()> {
    if !valid_name(&options.name) {
        return Err("invalid daemon name: only ASCII letters, digits, dots, hyphens and underscores are allowed".into());
    }
    let manager = ServiceManager::new(&app.user, options.scope)?;
    let listener = Listener::new(&app.user, manager.system, options.listen_http.as_deref())?;
    let context = listener.context()?;
    let exec = std::env::current_exe()?.canonicalize()?;
    let units = if manager.platform == Platform::Linux {
        let (socket, service) =
            linux_units(&exec, &listener, options.always_running, manager.system)?;
        vec![
            (manager.dir.join(SOCKET_UNIT), socket),
            (manager.dir.join(SERVICE_UNIT), service),
        ]
    } else {
        let log = if manager.system {
            Path::new("/var/log")
        } else {
            app.user.log_dir()?
        };
        if let Listener::Unix(path) = &listener {
            if manager.system {
                fs::create_dir_all(path.parent().expect("socket has parent"))?;
            } else {
                app.user.runtime_dir()?;
            }
        }
        let content = macos_unit(&exec, &listener, log, &options.name, options.always_running)?;
        vec![(
            manager
                .dir
                .join(format!("construct-{}.plist", options.name)),
            content,
        )]
    };
    let mut existing = Vec::new();
    for (path, _) in &units {
        if exists(path)? {
            if !options.force {
                return Err(format!(
                    "daemon is already installed at {}; use --force to reinstall",
                    path.display()
                )
                .into());
            }
            existing.push(path.clone());
        }
    }
    if !existing.is_empty() {
        manager.remove(&existing).await?;
    }
    fs::create_dir_all(&manager.dir)?;
    for (path, content) in &units {
        write_unit(path, content, options.force)?;
        if !options.quiet {
            writeln!(app.stdout, "Service file written to {}", path.display())?;
        }
    }
    if manager.platform == Platform::Linux {
        manager.systemctl(&["daemon-reload"]).await?;
        manager.systemctl(&["enable", "--now", SOCKET_UNIT]).await?;
        if options.always_running {
            manager
                .systemctl(&["enable", "--now", SERVICE_UNIT])
                .await?;
        }
    } else {
        manager.bootstrap(&units[0].0).await?;
    }
    let name = &options.name;
    let updated = app.contexts.upsert(name, context.clone(), true)?;
    if !options.quiet {
        writeln!(
            app.stdout,
            "Context '{name}' {}",
            if updated { "updated" } else { "created" }
        )?;
    }
    let client = ApiClient::new(&context, &app.contexts)?;
    let mut request = Request::new(ListModelProvidersRequest::default());
    request.timeout = Some(Duration::from_secs(5));
    let setup_complete = match client.model_provider.list_model_providers(request).await {
        Ok(response) => Some(!response.message.model_providers.is_empty()),
        Err(err) if context.kind == "http" && matches!(err.code, Code::Unauthenticated | Code::PermissionDenied) => None,
        Err(err) => return Err(format!("connection to daemon at {} failed: {err}; see https://docs.construct.sh/daemon/troubleshooting", context.address).into()),
    };
    if !options.quiet {
        writeln!(app.stdout, "Daemon installed successfully")?;
        writeln!(
            app.stdout,
            "{}",
            match setup_complete {
                Some(true) => "Ready to use! Try 'construct new' to start a conversation",
                Some(false) =>
                    "Next: Create a model provider with 'construct modelprovider create'",
                None => "Next: Configure authentication for this HTTP context",
            }
        )?;
    }
    Ok(())
}

pub(super) async fn stop(app: &mut App, scope: Scope) -> Result<()> {
    let (manager, paths) = ServiceManager::for_removal(&app.user, scope)?;
    if paths.is_empty() {
        writeln!(
            app.stdout,
            "Construct daemon service not found. Nothing to do."
        )?;
    } else {
        manager.stop(&paths).await?;
        writeln!(app.stdout, "Daemon stopped successfully")?;
    }
    Ok(())
}

pub(super) async fn uninstall(app: &mut App, options: Uninstall) -> Result<()> {
    let (manager, paths) = ServiceManager::for_removal(&app.user, options.scope)?;
    if paths.is_empty() {
        if !options.quiet {
            writeln!(
                app.stdout,
                "Construct daemon service not found. Nothing to do."
            )?;
        }
        return Ok(());
    }
    if !options.yes {
        writeln!(
            app.stdout,
            "Stop and disable the daemon and remove these files:"
        )?;
        for path in &paths {
            writeln!(app.stdout, "  - {}", path.display())?;
        }
        if !confirm(
            &mut app.stdin,
            &mut app.stdout,
            "Are you sure you want to continue?",
        ) {
            return Ok(());
        }
    }
    manager.remove(&paths).await?;
    if !options.quiet {
        writeln!(app.stdout, "Daemon uninstalled successfully")?;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn listener_context_and_platform_units() {
        let user = UserInfo::resolve("/home/test", "linux", "1000", &Default::default());
        let unix = Listener::new(&user, false, None).unwrap();
        assert_eq!(
            unix.context().unwrap().address,
            "/run/user/1000/construct/construct.sock"
        );
        let system = Listener::new(&user, true, None).unwrap();
        assert_eq!(
            system.context().unwrap().address,
            "/var/run/construct/construct.sock"
        );
        let http = Listener::new(&user, false, Some("[::]:8080")).unwrap();
        assert_eq!(http.context().unwrap().address, "http://[::1]:8080");
        assert!(Listener::new(&user, false, Some("127.0.0.1:0")).is_err());
        assert!(Listener::new(&user, false, Some("8080\nExecStart=/tmp/bad")).is_err());
        assert!(!valid_name("../bad"));

        let (socket, service) =
            linux_units(Path::new("/opt/100% ready/construct"), &unix, true, false).unwrap();
        assert!(socket.contains("ListenStream=/run/user/1000/construct/construct.sock\n"));
        assert!(service.contains(r#"ExecStart=:"/opt/100%% ready/construct" daemon run"#));
        assert!(service.contains("WantedBy=default.target"));
        let (_, socket_only_service) =
            linux_units(Path::new("/usr/bin/construct"), &system, false, true).unwrap();
        assert!(!socket_only_service.contains("[Install]"));

        let plist = macos_unit(
            Path::new("/opt/A&B/construct"),
            &http,
            Path::new("/Users/<user>/Logs"),
            "default",
            true,
        )
        .unwrap();
        for value in [
            "sh.construct.daemon.default",
            "A&amp;B",
            "&lt;user&gt;",
            "<string>8080</string>",
            "<string>IPv6</string>",
            "<true/>",
        ] {
            assert!(plist.contains(value), "missing {value}");
        }
    }
}
