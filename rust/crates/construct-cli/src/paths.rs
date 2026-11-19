use std::{
    collections::BTreeMap,
    fs, io,
    path::{Path, PathBuf},
};

#[derive(Clone, Debug)]
pub struct UserInfo {
    pub home: PathBuf,
    pub config: PathBuf,
    pub data: PathBuf,
    pub log: PathBuf,
    pub runtime: PathBuf,
}

impl UserInfo {
    pub fn config_dir(&self) -> io::Result<&Path> {
        Self::ensure(&self.config).map_err(|e| {
            io::Error::new(e.kind(), format!("failed to create config directory: {e}"))
        })?;
        Ok(&self.config)
    }
    pub fn data_dir(&self) -> io::Result<&Path> {
        Self::ensure(&self.data).map_err(|e| {
            io::Error::new(e.kind(), format!("failed to create data directory: {e}"))
        })?;
        Ok(&self.data)
    }
    pub fn log_dir(&self) -> io::Result<&Path> {
        Self::ensure(&self.log).map_err(|e| {
            io::Error::new(e.kind(), format!("failed to create log directory: {e}"))
        })?;
        Ok(&self.log)
    }
    pub fn runtime_dir(&self) -> io::Result<&Path> {
        Self::ensure(&self.runtime).map_err(|e| {
            io::Error::new(e.kind(), format!("failed to create runtime directory: {e}"))
        })?;
        Ok(&self.runtime)
    }
    pub fn from_env() -> io::Result<Self> {
        let env: BTreeMap<_, _> = std::env::vars().collect();
        let home = env
            .get(if cfg!(windows) { "USERPROFILE" } else { "HOME" })
            .ok_or_else(|| io::Error::other("home directory is not defined"))?;
        #[cfg(unix)]
        let uid = unsafe { libc::getuid() }.to_string();
        #[cfg(not(unix))]
        let uid = String::new();
        Ok(Self::resolve(home, std::env::consts::OS, &uid, &env))
    }

    pub fn resolve(
        home: impl AsRef<Path>,
        os: &str,
        uid: &str,
        env: &BTreeMap<String, String>,
    ) -> Self {
        let home = home.as_ref().to_path_buf();
        let base = match os {
            "macos" | "darwin" => home.join("Library/Application Support"),
            "windows" => env
                .get("LOCALAPPDATA")
                .map(PathBuf::from)
                .unwrap_or_else(|| home.join("AppData/Local")),
            _ => home.join(".local/share"),
        };
        let mac = matches!(os, "macos" | "darwin");
        let unix = !mac && os != "windows";
        let override_path = |key: &str, default: PathBuf| {
            env.get(key)
                .map(|s| {
                    if let Some(rest) = s.strip_prefix('~').or_else(|| s.strip_prefix("$HOME")) {
                        home.join(rest.trim_start_matches('/'))
                    } else {
                        PathBuf::from(s)
                    }
                })
                .filter(|p| p.is_absolute())
                .unwrap_or(default)
                .join("construct")
        };
        Self {
            config: override_path(
                "XDG_CONFIG_HOME",
                if unix {
                    home.join(".config")
                } else {
                    base.clone()
                },
            ),
            data: override_path("XDG_DATA_HOME", base.clone()),
            log: if mac {
                home.join("Library/Logs/construct")
            } else {
                override_path(
                    "XDG_STATE_HOME",
                    if unix {
                        home.join(".local/state")
                    } else {
                        base.clone()
                    },
                )
            },
            runtime: if mac {
                PathBuf::from("/tmp/construct")
            } else {
                override_path(
                    "XDG_RUNTIME_DIR",
                    if unix {
                        PathBuf::from(format!("/run/user/{uid}"))
                    } else {
                        base
                    },
                )
            },
            home,
        }
    }

    pub fn ensure(path: &Path) -> io::Result<()> {
        let mut builder = fs::DirBuilder::new();
        builder.recursive(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::DirBuilderExt;
            builder.mode(0o700);
        }
        builder.create(path)
    }
}

pub(crate) fn write_private(path: &Path, bytes: &[u8]) -> io::Result<()> {
    use std::io::Write;
    if let Some(parent) = path.parent() {
        UserInfo::ensure(parent)?;
    }
    let mut options = fs::OpenOptions::new();
    options.write(true).create(true).truncate(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o600);
    }
    let mut file = options.open(path)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        file.set_permissions(fs::Permissions::from_mode(0o600))?;
    }
    file.write_all(bytes)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn platform_defaults_and_overrides() {
        let mut env = BTreeMap::new();
        let linux = UserInfo::resolve("/home/a", "linux", "1000", &env);
        assert_eq!(linux.config, Path::new("/home/a/.config/construct"));
        assert_eq!(linux.runtime, Path::new("/run/user/1000/construct"));
        let dir = tempfile::tempdir().unwrap();
        let config = dir.path().to_string_lossy();
        env.insert("XDG_CONFIG_HOME".into(), config.into_owned());
        let mac = UserInfo::resolve("/Users/a", "macos", "501", &env);
        assert_eq!(mac.config, dir.path().join("construct"));
        assert_eq!(
            mac.data,
            Path::new("/Users/a/Library/Application Support/construct")
        );
        assert_eq!(mac.runtime, Path::new("/tmp/construct"));
    }

    #[test]
    fn windows_defaults_to_local_app_data() {
        let dir = tempfile::tempdir().unwrap();
        let home = dir.path().join("home");
        let app_data = dir.path().join("local");
        let env = BTreeMap::from([("LOCALAPPDATA".into(), app_data.display().to_string())]);
        let windows = UserInfo::resolve(&home, "windows", "", &env);
        let expected = app_data.join("construct");
        for path in [windows.config, windows.data, windows.log, windows.runtime] {
            assert_eq!(path, expected);
        }
    }
}
