use crate::{Error, Result};
use std::{
    fs,
    io::Write,
    path::{Path, PathBuf},
};

pub trait Provider {
    fn get(&self, key: &str) -> Result<String>;
    fn set(&self, key: &str, value: &str) -> Result<()>;
    fn delete(&self, key: &str) -> Result<()>;
}
pub struct FileProvider {
    base: PathBuf,
}
impl FileProvider {
    pub fn new(base: impl AsRef<Path>) -> Result<Self> {
        let base = base.as_ref().to_owned();
        let mut builder = fs::DirBuilder::new();
        builder.recursive(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::DirBuilderExt;
            builder.mode(0o700);
        }
        builder.create(&base)?;
        Ok(Self { base })
    }
    /// `data_home` is XDG_DATA_HOME (normally ~/.local/share), not the config directory.
    pub fn from_data_home(data_home: impl AsRef<Path>) -> Result<Self> {
        Self::new(data_home.as_ref().join("construct/secrets"))
    }
}
impl Provider for FileProvider {
    fn get(&self, key: &str) -> Result<String> {
        fs::read_to_string(self.base.join(key)).map_err(|e| {
            if e.kind() == std::io::ErrorKind::NotFound {
                Error::NotFound(key.into())
            } else {
                e.into()
            }
        })
    }
    fn set(&self, key: &str, value: &str) -> Result<()> {
        let mut options = fs::OpenOptions::new();
        options.write(true).create(true).truncate(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::OpenOptionsExt;
            options.mode(0o600);
        }
        options
            .open(self.base.join(key))?
            .write_all(value.as_bytes())?;
        Ok(())
    }
    fn delete(&self, key: &str) -> Result<()> {
        match fs::remove_file(self.base.join(key)) {
            Ok(()) => Ok(()),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
            Err(e) => Err(e.into()),
        }
    }
}

/// Uses the Go library's `service=construct`, `username=<key>` attributes on Linux.
pub struct KeyringProvider;
#[cfg(target_os = "linux")]
impl KeyringProvider {
    fn access<T>(
        &self,
        key: &str,
        f: impl FnOnce(secret_service::blocking::Collection<'_>) -> Result<T>,
    ) -> Result<T> {
        use secret_service::{blocking::SecretService, EncryptionType};
        let service = SecretService::connect(EncryptionType::Dh)
            .map_err(|e| Error::Provider(e.to_string()))?;
        let collection = service
            .get_collection_by_alias("login")
            .map_err(|e| Error::Provider(e.to_string()))?;
        collection
            .unlock()
            .map_err(|e| Error::Provider(format!("{key}: {e}")))?;
        f(collection)
    }
}
#[cfg(target_os = "linux")]
impl Provider for KeyringProvider {
    fn get(&self, key: &str) -> Result<String> {
        self.access(key, |c| {
            let items = c
                .search_items(std::collections::HashMap::from([
                    ("service", "construct"),
                    ("username", key),
                ]))
                .map_err(|e| Error::Provider(e.to_string()))?;
            let item = items.first().ok_or_else(|| Error::NotFound(key.into()))?;
            item.unlock().map_err(|e| Error::Provider(e.to_string()))?;
            String::from_utf8(
                item.get_secret()
                    .map_err(|e| Error::Provider(e.to_string()))?,
            )
            .map_err(|e| Error::Provider(e.to_string()))
        })
    }
    fn set(&self, key: &str, value: &str) -> Result<()> {
        self.access(key, |c| {
            c.create_item(
                &format!("Password for '{key}' on 'construct'"),
                std::collections::HashMap::from([("service", "construct"), ("username", key)]),
                value.as_bytes(),
                true,
                "text/plain",
            )
            .map_err(|e| Error::Provider(e.to_string()))?;
            Ok(())
        })
    }
    fn delete(&self, key: &str) -> Result<()> {
        self.access(key, |c| {
            let items = c
                .search_items(std::collections::HashMap::from([
                    ("service", "construct"),
                    ("username", key),
                ]))
                .map_err(|e| Error::Provider(e.to_string()))?;
            items
                .first()
                .ok_or_else(|| Error::NotFound(key.into()))?
                .delete()
                .map_err(|e| Error::Provider(e.to_string()))
        })
    }
}
#[cfg(not(target_os = "linux"))]
fn keyring_error(key: &str, error: keyring::Error) -> Error {
    match error {
        keyring::Error::NoEntry => Error::NotFound(key.into()),
        keyring::Error::TooLong(_, _) => Error::TooLarge(key.into()),
        e => Error::Provider(e.to_string()),
    }
}
#[cfg(not(target_os = "linux"))]
fn entry(key: &str) -> std::result::Result<keyring::Entry, keyring::Error> {
    #[cfg(target_os = "windows")]
    {
        keyring::Entry::new_with_target(&format!("construct:{key}"), "construct", key)
    }
    #[cfg(not(target_os = "windows"))]
    {
        keyring::Entry::new("construct", key)
    }
}
#[cfg(not(target_os = "linux"))]
impl Provider for KeyringProvider {
    fn get(&self, key: &str) -> Result<String> {
        let value = entry(key)
            .and_then(|e| e.get_secret())
            .map_err(|e| keyring_error(key, e))?;
        let value = String::from_utf8(value).map_err(|e| Error::Provider(e.to_string()))?;
        #[cfg(target_os = "macos")]
        {
            use base64::Engine;
            if let Some(encoded) = value.strip_prefix("go-keyring-base64:") {
                let decoded = base64::engine::general_purpose::STANDARD
                    .decode(encoded)
                    .map_err(|e| Error::Provider(e.to_string()))?;
                return String::from_utf8(decoded).map_err(|e| Error::Provider(e.to_string()));
            }
            if let Some(encoded) = value.strip_prefix("go-keyring-encoded:") {
                let decoded = (0..encoded.len())
                    .step_by(2)
                    .map(|i| {
                        encoded
                            .get(i..i + 2)
                            .and_then(|s| u8::from_str_radix(s, 16).ok())
                            .ok_or_else(|| Error::Provider("invalid hex secret".into()))
                    })
                    .collect::<Result<Vec<_>>>()?;
                return String::from_utf8(decoded).map_err(|e| Error::Provider(e.to_string()));
            }
        }
        Ok(value)
    }
    fn set(&self, key: &str, value: &str) -> Result<()> {
        #[cfg(target_os = "windows")]
        if value.len() > 2560 {
            return Err(Error::TooLarge(key.into()));
        }
        #[cfg(target_os = "macos")]
        let value = {
            use base64::Engine;
            format!(
                "go-keyring-base64:{}",
                base64::engine::general_purpose::STANDARD.encode(value)
            )
        };
        entry(key)
            .and_then(|e| e.set_secret(value.as_bytes()))
            .map_err(|e| keyring_error(key, e))
    }
    fn delete(&self, key: &str) -> Result<()> {
        entry(key)
            .and_then(|e| e.delete_credential())
            .map_err(|e| keyring_error(key, e))
    }
}
