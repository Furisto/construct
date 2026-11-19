use std::path::PathBuf;

/// The user directories skills are discovered in and installed to (Go `shared.UserInfo`).
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct UserDirs {
    /// Home directory; `None` skips `~/` discovery paths.
    pub home: Option<PathBuf>,
    /// Construct config directory (`$XDG_CONFIG_HOME/construct`).
    pub config_dir: PathBuf,
}

impl UserDirs {
    /// Resolve like Go `DefaultUserInfo` with `adrg/xdg`.
    pub fn from_env() -> Option<Self> {
        let home = std::env::var_os("HOME")
            .filter(|h| !h.is_empty())
            .map(PathBuf::from);
        let config_home = std::env::var_os("XDG_CONFIG_HOME")
            .filter(|v| !v.is_empty())
            .map(PathBuf::from)
            .or_else(|| {
                home.as_ref().map(|h| {
                    if cfg!(target_os = "macos") {
                        h.join("Library/Application Support")
                    } else {
                        h.join(".config")
                    }
                })
            })?;
        Some(Self {
            home,
            config_dir: config_home.join("construct"),
        })
    }

    /// Construct config dir, created with mode 0700 like Go `ConstructConfigDir`.
    pub(crate) fn ensure_config_dir(&self) -> std::io::Result<PathBuf> {
        let mut builder = std::fs::DirBuilder::new();
        builder.recursive(true);
        #[cfg(unix)]
        std::os::unix::fs::DirBuilderExt::mode(&mut builder, 0o700);
        builder.create(&self.config_dir)?;
        Ok(self.config_dir.clone())
    }
}
