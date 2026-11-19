//! Go-compatible Construct secret storage and cleartext Tink AES-GCM keysets.
//! Serialized keysets contain secret key material; store them only in a provider.
mod encryption;
mod provider;
pub use encryption::*;
pub use provider::*;

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("key {0} not found")]
    NotFound(String),
    #[error("secret {0} is too large")]
    TooLarge(String),
    #[error("invalid keyset: {0}")]
    InvalidKeyset(String),
    #[error("encryption or decryption failed")]
    Crypto,
    #[error(transparent)]
    Io(#[from] std::io::Error),
    #[error("secret provider: {0}")]
    Provider(String),
}
pub type Result<T> = std::result::Result<T, Error>;

pub fn encryption_key_secret() -> &'static str {
    "encryption_key"
}
pub fn model_provider_associated(id: uuid::Uuid) -> Vec<u8> {
    format!("model_provider:{id}").into_bytes()
}

pub fn load_or_generate(provider: &dyn Provider) -> Result<Encryption> {
    let keyset = match provider.get(encryption_key_secret()) {
        Ok(json) => Keyset::from_json(&json)?,
        Err(Error::NotFound(_)) => {
            let keyset = Keyset::generate();
            provider.set(encryption_key_secret(), &keyset.to_json()?)?;
            keyset
        }
        Err(err) => return Err(err),
    };
    Encryption::new(keyset)
}
