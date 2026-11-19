//! Host environment discovery in Go's display order.
use crate::Result;
use std::{
    collections::BTreeMap,
    ffi::OsStr,
    path::{Path, PathBuf},
};

pub fn project_structure(root: impl AsRef<Path>) -> std::io::Result<String> {
    let mut names = std::fs::read_dir(root)?
        .map(|e| e.map(|e| e.file_name()))
        .collect::<std::io::Result<Vec<_>>>()?;
    names.sort();
    Ok(names
        .iter()
        .map(|name| format!("|_{}\n", name.to_string_lossy()))
        .collect())
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Shell {
    pub path: PathBuf,
    pub name: String,
}

pub fn default_shell() -> Result<Shell> {
    let passwd = std::fs::read_to_string("/etc/passwd").unwrap_or_default();
    #[cfg(unix)]
    let username = {
        // getuid has no memory or pointer preconditions.
        let uid = unsafe { libc::getuid() }.to_string();
        passwd
            .lines()
            .find_map(|line| {
                let fields: Vec<_> = line.split(':').collect();
                (fields.len() >= 7 && fields[2] == uid).then(|| fields[0].to_owned())
            })
            .unwrap_or_default()
    };
    #[cfg(not(unix))]
    let username = String::new();
    shell_with(
        std::env::var_os("SHELL").as_deref(),
        &passwd,
        &username,
        std::env::consts::OS,
        |p| p.exists(),
    )
}

pub fn shell_with(
    shell: Option<&OsStr>,
    passwd: &str,
    username: &str,
    os: &str,
    exists: impl Fn(&Path) -> bool,
) -> Result<Shell> {
    let mut candidates = Vec::new();
    if let Some(shell) = shell.filter(|s| !s.is_empty()) {
        candidates.push(PathBuf::from(shell));
    }
    if os != "windows" {
        for line in passwd.lines() {
            let f: Vec<_> = line.split(':').collect();
            if f.len() >= 7
                && f[0] == username
                && !["", "/sbin/nologin", "/bin/false"].contains(&f[6])
            {
                candidates.push(f[6].into());
                break;
            }
        }
    }
    match os {
        "windows" => {
            let root = std::env::var_os("SYSTEMROOT").unwrap_or_default();
            candidates
                .push(PathBuf::from(&root).join("System32/WindowsPowerShell/v1.0/powershell.exe"));
            if let Some(pwsh) = lookup_path("pwsh.exe", std::env::var_os("PATH").as_deref()) {
                candidates.push(pwsh);
            }
            candidates.push(PathBuf::from(root).join("System32/cmd.exe"));
        }
        "macos" | "darwin" => {
            candidates.extend(["/bin/zsh", "/bin/bash", "/bin/sh"].map(PathBuf::from))
        }
        _ => candidates
            .extend(["/bin/bash", "/bin/sh", "/usr/bin/bash", "/usr/bin/sh"].map(PathBuf::from)),
    }
    let path = candidates
        .into_iter()
        .find(|p| exists(p))
        .ok_or("unable to determine user shell using any available method")?;
    let name = if os == "windows" {
        path.file_stem()
    } else {
        path.file_name()
    }
    .unwrap_or_default()
    .to_string_lossy()
    .into_owned();
    Ok(Shell { path, name })
}

pub fn lookup_path(tool: &str, path: Option<&OsStr>) -> Option<PathBuf> {
    std::env::split_paths(path?)
        .flat_map(|dir| {
            #[cfg(windows)]
            let names: Vec<_> = std::env::var("PATHEXT")
                .unwrap_or_else(|_| ".COM;.EXE;.BAT;.CMD".into())
                .split(';')
                .map(|ext| dir.join(format!("{tool}{ext}")))
                .collect();
            #[cfg(not(windows))]
            let names = vec![dir.join(tool)];
            names
        })
        .find(|p| {
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                p.metadata()
                    .is_ok_and(|m| m.is_file() && m.permissions().mode() & 0o111 != 0)
            }
            #[cfg(not(unix))]
            {
                p.is_file()
            }
        })
        .filter(|p| p.is_absolute())
}

/// Go field names map to ordered available executables; missing categories are empty.
pub type DevTools = BTreeMap<String, Vec<String>>;

pub fn available_dev_tools() -> DevTools {
    let path = std::env::var_os("PATH");
    available_dev_tools_with(|tool| lookup_path(tool, path.as_deref()).is_some())
}

pub fn available_dev_tools_with(lookup: impl Fn(&str) -> bool) -> DevTools {
    TOOL_CATEGORIES
        .iter()
        .map(|(name, tools)| {
            (
                name.to_string(),
                tools
                    .iter()
                    .filter(|tool| lookup(tool))
                    .map(|s| s.to_string())
                    .collect(),
            )
        })
        .collect()
}

pub const TOOL_CATEGORIES: &[(&str, &[&str])] = &[
    ("VersionControl", &["git", "gh"]),
    (
        "PackageManagers",
        &[
            "npm", "yarn", "pnpm", "pip", "pip3", "pipenv", "poetry", "cargo", "go", "composer",
            "gem", "bundle", "maven", "gradle", "brew",
        ],
    ),
    (
        "LanguageRuntimes",
        &[
            "node", "python", "python3", "java", "go", "php", "ruby", "swift", "kotlin", "scala",
            "dotnet",
        ],
    ),
    (
        "BuildTools",
        &[
            "make", "cmake", "ninja", "webpack", "vite", "rollup", "parcel", "gulp", "bazel",
        ],
    ),
    ("Testing", &["jest", "mocha", "pytest", "phpunit", "rspec"]),
    (
        "Database",
        &["mysql", "psql", "sqlite3", "mongo", "redis-cli"],
    ),
    (
        "ContainerOrchestration",
        &[
            "docker",
            "docker-compose",
            "podman",
            "kubectl",
            "helm",
            "minikube",
            "nerdctl",
        ],
    ),
    (
        "CloudInfrastructure",
        &["aws", "gcloud", "az", "terraform", "ansible", "pulumi"],
    ),
    (
        "TextProcessing",
        &["grep", "rg", "ag", "ack", "sed", "awk", "jq", "yq"],
    ),
    (
        "FileOperations",
        &[
            "find", "fd", "locate", "rsync", "scp", "tar", "zip", "unzip", "gzip",
        ],
    ),
    (
        "NetworkHTTP",
        &["curl", "wget", "nc", "ping", "dig", "nslookup"],
    ),
    (
        "SystemMonitoring",
        &["ps", "top", "htop", "lsof", "netstat", "ss", "df", "du"],
    ),
];

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn directory_sorted_empty_and_missing() {
        let dir = tempfile::tempdir().unwrap();
        assert_eq!(project_structure(dir.path()).unwrap(), "");
        std::fs::write(dir.path().join("z"), "").unwrap();
        std::fs::create_dir(dir.path().join("a")).unwrap();
        assert_eq!(project_structure(dir.path()).unwrap(), "|_a\n|_z\n");
        assert!(project_structure(dir.path().join("missing")).is_err());
    }
    #[test]
    fn shell_precedence_and_failure() {
        let passwd = "alice:x:1:1::/home/alice:/bin/zsh\n";
        assert_eq!(
            shell_with(
                Some(OsStr::new("/bin/custom")),
                passwd,
                "alice",
                "linux",
                |_| true
            )
            .unwrap()
            .name,
            "custom"
        );
        assert_eq!(
            shell_with(None, passwd, "alice", "linux", |_| true)
                .unwrap()
                .name,
            "zsh"
        );
        assert_eq!(
            shell_with(None, "alice:x:1:1::/:/bin/false", "alice", "linux", |_| {
                true
            })
            .unwrap()
            .name,
            "bash"
        );
        assert!(shell_with(None, "", "alice", "linux", |_| false).is_err());
    }
    #[test]
    fn tools_order_empty_and_lookup() {
        let tools = available_dev_tools_with(|s| ["gh", "git", "cargo"].contains(&s));
        assert_eq!(tools["VersionControl"], ["git", "gh"]);
        assert!(available_dev_tools_with(|_| false)
            .values()
            .all(Vec::is_empty));
        assert!(lookup_path("missing", None).is_none());
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let dir = tempfile::tempdir().unwrap();
            let bin = dir.path().join("git");
            std::fs::write(&bin, "").unwrap();
            assert!(lookup_path("git", Some(dir.path().as_os_str())).is_none());
            std::fs::set_permissions(&bin, std::fs::Permissions::from_mode(0o755)).unwrap();
            assert_eq!(lookup_path("git", Some(dir.path().as_os_str())), Some(bin));
        }
    }
}
