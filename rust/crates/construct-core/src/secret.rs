//! Secret storage, separate from encryption and model-provider persistence.
use async_trait::async_trait;
use uuid::Uuid;

/// Go's encryption key entry, shared across secret implementations.
pub const ENCRYPTION_KEY_SECRET: &str = "encryption_key";

/// Go's model-provider associated-data/key spelling.
pub fn model_provider_secret_key(id: Uuid) -> String {
    format!("model_provider:{id}")
}

/// Secret failures. Causes must not contain secret values.
#[derive(Clone, Debug, PartialEq, Eq, thiserror::Error)]
pub enum SecretError {
    /// Equivalent to Go's ErrSecretNotFound (match the variant, not its cause).
    #[error("key {key} not found: {cause}")]
    NotFound {
        /// Missing entry name.
        key: String,
        /// Backend diagnostic.
        cause: String,
    },
    /// Equivalent to ErrSecretMarshal.
    #[error("failed to marshal secret {key}: {cause}")]
    Marshal {
        /// Entry name.
        key: String,
        /// Encoding diagnostic.
        cause: String,
    },
    /// Equivalent to ErrSecretTooLarge.
    #[error("secret {key} is too large: {cause}")]
    TooLarge {
        /// Entry name.
        key: String,
        /// Backend limit diagnostic.
        cause: String,
    },
    /// Other backend/I/O failures.
    #[error("{0}")]
    Internal(String),
}

/// Async adaptation of Go's secret.Provider; set overwrites existing values.
#[async_trait]
pub trait SecretProvider: Send + Sync {
    /// Retrieve plaintext or NotFound.
    async fn get(&self, key: &str) -> Result<String, SecretError>;
    /// Store plaintext; implementations own persistence and size limits.
    async fn set(&self, key: &str, value: &str) -> Result<(), SecretError>;
    /// Remove an entry idempotently, matching Go's file provider.
    async fn delete(&self, key: &str) -> Result<(), SecretError>;
}
