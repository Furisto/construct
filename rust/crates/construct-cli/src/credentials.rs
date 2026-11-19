use std::{collections::BTreeMap, sync::Mutex};

pub const SERVICE: &str = "construct";
#[derive(Debug, thiserror::Error)]
pub enum CredentialError {
    #[error("secret {key:?} not found: {message}")]
    NotFound { key: String, message: String },
    #[error("secret {key:?} is too large: {message}")]
    TooLarge { key: String, message: String },
    #[error("{0}")]
    Other(String),
}
pub trait Provider: Send + Sync {
    fn get(&self, key: &str) -> Result<String, CredentialError>;
    fn set(&self, key: &str, value: &str) -> Result<(), CredentialError>;
    fn delete(&self, key: &str) -> Result<(), CredentialError>;
}
#[derive(Default)]
pub struct MemoryProvider(Mutex<BTreeMap<String, String>>);
fn not_found(key: &str) -> CredentialError {
    CredentialError::NotFound {
        key: key.into(),
        message: "secret not found in keyring".into(),
    }
}
impl Provider for MemoryProvider {
    fn get(&self, key: &str) -> Result<String, CredentialError> {
        self.0
            .lock()
            .unwrap()
            .get(key)
            .cloned()
            .ok_or_else(|| not_found(key))
    }
    fn set(&self, key: &str, value: &str) -> Result<(), CredentialError> {
        self.0.lock().unwrap().insert(key.into(), value.into());
        Ok(())
    }
    fn delete(&self, key: &str) -> Result<(), CredentialError> {
        self.0
            .lock()
            .unwrap()
            .remove(key)
            .map(|_| ())
            .ok_or_else(|| not_found(key))
    }
}
#[derive(Default)]
pub struct SystemProvider;
impl SystemProvider {
    #[cfg(not(target_os = "linux"))]
    fn entry(key: &str) -> Result<keyring::Entry, CredentialError> {
        #[cfg(target_os = "windows")]
        let entry = keyring::Entry::new_with_target(&format!("{SERVICE}:{key}"), SERVICE, key);
        #[cfg(not(target_os = "windows"))]
        let entry = keyring::Entry::new(SERVICE, key);
        entry.map_err(|e| Self::error(key, e))
    }
    #[cfg(not(target_os = "linux"))]
    fn error(key: &str, error: keyring::Error) -> CredentialError {
        match error {
            keyring::Error::NoEntry => not_found(key),
            keyring::Error::TooLong(_, _) => CredentialError::TooLarge {
                key: key.into(),
                message: error.to_string(),
            },
            _ => CredentialError::Other(error.to_string()),
        }
    }
}
#[cfg(not(target_os = "linux"))]
impl Provider for SystemProvider {
    fn get(&self, key: &str) -> Result<String, CredentialError> {
        let entry = Self::entry(key)?;
        let bytes = entry.get_secret().map_err(|e| Self::error(key, e))?;
        let value = String::from_utf8(bytes).map_err(|e| CredentialError::Other(e.to_string()))?;
        #[cfg(target_os = "macos")]
        {
            use base64::Engine;
            if let Some(encoded) = value.strip_prefix("go-keyring-base64:") {
                return String::from_utf8(
                    base64::engine::general_purpose::STANDARD
                        .decode(encoded)
                        .map_err(|e| CredentialError::Other(e.to_string()))?,
                )
                .map_err(|e| CredentialError::Other(e.to_string()));
            }
            if let Some(encoded) = value.strip_prefix("go-keyring-encoded:") {
                return String::from_utf8(
                    hex::decode(encoded).map_err(|e| CredentialError::Other(e.to_string()))?,
                )
                .map_err(|e| CredentialError::Other(e.to_string()));
            }
        }
        Ok(value)
    }
    fn set(&self, key: &str, value: &str) -> Result<(), CredentialError> {
        #[cfg(target_os = "windows")]
        if value.len() > 2560 {
            return Err(CredentialError::TooLarge {
                key: key.into(),
                message: "SetData failed: data too big".into(),
            });
        }
        #[cfg(target_os = "macos")]
        let value = {
            use base64::Engine;
            format!(
                "go-keyring-base64:{}",
                base64::engine::general_purpose::STANDARD.encode(value)
            )
        };
        Self::entry(key)?
            .set_secret(value.as_bytes())
            .map_err(|e| Self::error(key, e))
    }
    fn delete(&self, key: &str) -> Result<(), CredentialError> {
        Self::entry(key)?
            .delete_credential()
            .map_err(|e| Self::error(key, e))
    }
}
#[cfg(target_os = "linux")]
impl SystemProvider {
    fn with_login<T>(
        &self,
        operation: impl FnOnce(dbus_secret_service::Collection<'_>) -> Result<T, CredentialError>,
    ) -> Result<T, CredentialError> {
        use dbus_secret_service::{EncryptionType, SecretService};
        let service = SecretService::connect(EncryptionType::Dh)
            .map_err(|e| CredentialError::Other(e.to_string()))?;
        let collection = service
            .get_all_collections()
            .map_err(|e| CredentialError::Other(e.to_string()))?
            .into_iter()
            .find(|c| &*c.path == "/org/freedesktop/secrets/collection/login")
            .map(Ok)
            .unwrap_or_else(|| service.get_default_collection())
            .map_err(|e| CredentialError::Other(e.to_string()))?;
        collection
            .ensure_unlocked()
            .map_err(|e| CredentialError::Other(e.to_string()))?;
        operation(collection)
    }
}
#[cfg(target_os = "linux")]
impl Provider for SystemProvider {
    fn get(&self, key: &str) -> Result<String, CredentialError> {
        self.with_login(|collection| {
            let items = collection
                .search_items(std::collections::HashMap::from([
                    ("service", SERVICE),
                    ("username", key),
                ]))
                .map_err(|e| CredentialError::Other(e.to_string()))?;
            let item = items.first().ok_or_else(|| not_found(key))?;
            let bytes = item
                .get_secret()
                .map_err(|e| CredentialError::Other(e.to_string()))?;
            String::from_utf8(bytes).map_err(|e| CredentialError::Other(e.to_string()))
        })
    }
    fn set(&self, key: &str, value: &str) -> Result<(), CredentialError> {
        self.with_login(|collection| {
            collection
                .create_item(
                    &format!("Password for '{key}' on '{SERVICE}'"),
                    std::collections::HashMap::from([("service", SERVICE), ("username", key)]),
                    value.as_bytes(),
                    true,
                    "text/plain",
                )
                .map_err(|e| CredentialError::Other(e.to_string()))?;
            Ok(())
        })
    }
    fn delete(&self, key: &str) -> Result<(), CredentialError> {
        self.with_login(|collection| {
            let items = collection
                .search_items(std::collections::HashMap::from([
                    ("service", SERVICE),
                    ("username", key),
                ]))
                .map_err(|e| CredentialError::Other(e.to_string()))?;
            items
                .first()
                .ok_or_else(|| not_found(key))?
                .delete()
                .map_err(|e| CredentialError::Other(e.to_string()))
        })
    }
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn memory() {
        let p = MemoryProvider::default();
        assert!(matches!(p.get("x"), Err(CredentialError::NotFound { .. })));
        p.set("x", "token").unwrap();
        assert_eq!(p.get("x").unwrap(), "token");
        p.delete("x").unwrap();
        assert!(p.delete("x").is_err());
    }
}
