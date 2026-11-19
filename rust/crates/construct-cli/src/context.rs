use crate::{
    credentials::{CredentialError, Provider},
    paths::{write_private, UserInfo},
    Result,
};
use serde::{Deserialize, Serialize};
use std::{collections::BTreeMap, fs, path::PathBuf, sync::Arc};

pub const KEYRING_REF_PREFIX: &str = "keyring://construct/";
#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct AuthConfig {
    #[serde(rename = "type", skip_serializing_if = "String::is_empty")]
    pub auth_type: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub token: String,
    #[serde(rename = "token-ref", skip_serializing_if = "String::is_empty")]
    pub token_ref: String,
}
impl AuthConfig {
    pub fn validate(&self) -> Result<()> {
        if !self.auth_type.is_empty() && self.auth_type != "token" {
            return Err(format!("invalid auth type: {} (supported: token)", self.auth_type).into());
        }
        if !self.token.is_empty() && !self.token_ref.is_empty() {
            return Err("cannot specify both token and token-ref".into());
        }
        if !self.token_ref.is_empty() && !self.token_ref.starts_with(KEYRING_REF_PREFIX) {
            return Err(
                format!("invalid token-ref format: must start with {KEYRING_REF_PREFIX}").into(),
            );
        }
        if self.auth_type == "token" && !self.is_configured() {
            return Err("auth type 'token' requires either token or token-ref".into());
        }
        Ok(())
    }
    pub fn is_configured(&self) -> bool {
        !self.token.is_empty() || !self.token_ref.is_empty()
    }
    pub fn keyring_key(&self) -> &str {
        self.token_ref
            .strip_prefix(KEYRING_REF_PREFIX)
            .unwrap_or(&self.token_ref)
    }
}
#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct EndpointContext {
    pub address: String,
    pub kind: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub auth: Option<AuthConfig>,
}
impl EndpointContext {
    pub fn validate(&self) -> Result<()> {
        if self.kind != "unix" && self.kind != "http" {
            return Err(format!("invalid kind: {}", self.kind).into());
        }
        if self.kind == "unix" && !self.address.starts_with('/') {
            return Err(format!("unix address must be an absolute path: {}", self.address).into());
        }
        if self.kind == "http"
            && (self.address.contains(['\n', '\r', '\t'])
                || self
                    .address
                    .as_bytes()
                    .windows(1)
                    .enumerate()
                    .any(|(i, b)| {
                        b == b"%"
                            && (i + 2 >= self.address.len()
                                || !self.address.as_bytes()[i + 1..i + 3]
                                    .iter()
                                    .all(u8::is_ascii_hexdigit))
                    }))
        {
            return Err(format!("invalid http address: {}", self.address).into());
        }
        if let Some(auth) = &self.auth {
            auth.validate()
                .map_err(|e| format!("invalid auth config: {e}"))?;
        }
        Ok(())
    }
}
#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct EndpointContexts {
    pub current: String,
    pub previous: String,
    #[serde(deserialize_with = "deserialize_contexts")]
    pub contexts: BTreeMap<String, EndpointContext>,
}
fn deserialize_contexts<'de, D: serde::Deserializer<'de>>(
    deserializer: D,
) -> std::result::Result<BTreeMap<String, EndpointContext>, D::Error> {
    Ok(Option::<BTreeMap<String, EndpointContext>>::deserialize(deserializer)?.unwrap_or_default())
}
impl EndpointContexts {
    pub fn validate(&self) -> Result<()> {
        for (name, context) in &self.contexts {
            context
                .validate()
                .map_err(|e| format!("invalid context '{name}': {e}"))?;
        }
        if !self.current.is_empty() && !self.contexts.contains_key(&self.current) {
            return Err(format!("current context {} not found", self.current).into());
        }
        Ok(())
    }
    pub fn current(&self) -> Option<&EndpointContext> {
        self.contexts.get(&self.current)
    }
    pub fn set_current(&mut self, name: &str) -> Result<()> {
        if name.is_empty() {
            return Err("context name is required".into());
        }
        if !self.contexts.contains_key(name) {
            return Err(format!("context {name} not found").into());
        }
        self.current = name.into();
        Ok(())
    }
    pub fn set_previous(&mut self, name: &str) -> Result<()> {
        if name.is_empty() {
            return Err("context name is required".into());
        }
        self.previous = name.into();
        Ok(())
    }
}
pub struct ContextManager {
    path: PathBuf,
    pub provider: Arc<dyn Provider>,
}
impl ContextManager {
    pub fn new(user: &UserInfo, provider: Arc<dyn Provider>) -> Self {
        Self {
            path: user.config.join("context.yaml"),
            provider,
        }
    }
    pub fn load(&self) -> Result<EndpointContexts> {
        if let Some(parent) = self.path.parent() {
            UserInfo::ensure(parent)?;
        }
        match fs::read(&self.path) {
            Ok(bytes) => {
                Ok(serde_yaml::from_slice::<Option<EndpointContexts>>(&bytes)?.unwrap_or_default())
            }
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(EndpointContexts::default()),
            Err(e) => Err(e.into()),
        }
    }
    pub fn save(&self, contexts: &EndpointContexts) -> Result<()> {
        write_private(&self.path, crate::yaml::to_string(contexts)?.as_bytes())?;
        Ok(())
    }
    pub fn get(&self, name: &str) -> Result<EndpointContext> {
        self.load()?
            .contexts
            .remove(name)
            .ok_or_else(|| format!("context {name:?} not found").into())
    }
    pub fn list(&self) -> Result<Vec<String>> {
        Ok(self.load()?.contexts.into_keys().collect())
    }
    pub fn upsert(&self, name: &str, context: EndpointContext, set_current: bool) -> Result<bool> {
        context.validate()?;
        let mut contexts = self.load()?;
        let exists = contexts.contexts.insert(name.into(), context).is_some();
        if set_current {
            contexts.set_current(name)?;
        }
        self.save(&contexts)?;
        Ok(exists)
    }
    pub fn delete(&self, name: &str) -> Result<()> {
        let mut contexts = self.load()?;
        let context = contexts
            .contexts
            .get(name)
            .ok_or_else(|| format!("context {name:?} not found"))?;
        if let Some(auth) = &context.auth {
            if !auth.keyring_key().is_empty() {
                match self.provider.delete(auth.keyring_key()) {
                    Ok(()) | Err(CredentialError::NotFound { .. }) => {}
                    Err(e) => {
                        return Err(format!("failed to delete token from keyring: {e}").into())
                    }
                }
            }
        }
        contexts.contexts.remove(name);
        if contexts.current == name {
            contexts.current.clear();
        }
        self.save(&contexts)
    }
    pub fn set_current(&self, name: &str) -> Result<()> {
        let mut contexts = self.load()?;
        let previous = contexts.current.clone();
        contexts.set_current(name)?;
        if !previous.is_empty() {
            contexts.set_previous(&previous)?;
        }
        self.save(&contexts)
    }
    pub fn store_token(&self, name: &str, token: &str) -> std::result::Result<(), CredentialError> {
        self.provider.set(name, token)
    }
    pub fn retrieve_token(&self, name: &str) -> std::result::Result<String, CredentialError> {
        self.provider.get(name)
    }
    pub fn delete_token(&self, name: &str) -> std::result::Result<(), CredentialError> {
        self.provider.delete(name)
    }
    pub fn resolve_token(&self, auth: &AuthConfig) -> Result<String> {
        if !auth.token.is_empty() {
            return Ok(auth.token.clone());
        }
        if !auth.token_ref.is_empty() {
            let key = auth.keyring_key();
            if key.is_empty() {
                return Err("invalid token-ref format".into());
            }
            return self.retrieve_token(key).map_err(|e| match e {
                CredentialError::NotFound{..} => format!("token not found in keyring for context {key:?} - try re-authenticating with 'construct context add'").into(),
                _ => Box::new(e) as crate::Error,
            });
        }
        Err("no token configured".into())
    }
}
pub fn resolve_context_name(flag: &str, environment: Option<&str>, current: &str) -> String {
    if !flag.is_empty() {
        flag
    } else {
        environment.filter(|s| !s.is_empty()).unwrap_or(current)
    }
    .into()
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn contexts_roundtrip() {
        let dir = tempfile::tempdir().unwrap();
        let user = UserInfo::resolve(dir.path(), "linux", "1", &BTreeMap::new());
        let manager = ContextManager::new(
            &user,
            Arc::new(crate::credentials::MemoryProvider::default()),
        );
        assert!(manager.list().unwrap().is_empty());
        let endpoint = EndpointContext {
            address: "/tmp/socket".into(),
            kind: "unix".into(),
            auth: None,
        };
        assert!(!manager.upsert("a", endpoint.clone(), true).unwrap());
        manager.upsert("b", endpoint, true).unwrap();
        manager.set_current("a").unwrap();
        assert_eq!(manager.load().unwrap().previous, "b");
        manager.delete("a").unwrap();
        assert_eq!(manager.load().unwrap().current, "");
        assert_eq!(
            manager.get("missing").unwrap_err().to_string(),
            "context \"missing\" not found"
        );
        assert_eq!(resolve_context_name("flag", Some("env"), "current"), "flag");
    }
    #[test]
    fn validation() {
        assert_eq!(
            EndpointContext::default()
                .validate()
                .unwrap_err()
                .to_string(),
            "invalid kind: "
        );
        let auth = AuthConfig {
            auth_type: "token".into(),
            ..Default::default()
        };
        assert_eq!(
            auth.validate().unwrap_err().to_string(),
            "auth type 'token' requires either token or token-ref"
        );
    }
}
