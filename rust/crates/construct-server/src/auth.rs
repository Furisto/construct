//! Token generation, single-use setup codes, and transport-aware authentication.
use base64::{
    alphabet,
    engine::{
        general_purpose::{GeneralPurpose, GeneralPurposeConfig},
        DecodePaddingMode,
    },
    Engine,
};
use chrono::{Duration, Utc};
use construct_connect::{http, Code, ConnectError, RequestInterceptor, Transport};
use construct_core::store::{ListOptions, StoreError, TokenFilter, TokenStore};
use construct_types::Timestamp;
use rand::{rngs::OsRng, RngCore};
use sha2::{Digest, Sha256};
use std::{
    collections::HashMap,
    fmt,
    sync::{Arc, Mutex},
};

const TOKEN_ENCODING: GeneralPurpose = GeneralPurpose::new(
    &alphabet::URL_SAFE,
    GeneralPurposeConfig::new()
        .with_encode_padding(false)
        .with_decode_padding_mode(DecodePaddingMode::RequireNone)
        .with_decode_allow_trailing_bits(true),
);

/// Random bytes in a bearer token.
pub const TOKEN_LENGTH: usize = 32;
/// Plaintext bearer token prefix.
pub const TOKEN_PREFIX: &str = "ct_";
/// Setup code characters, excluding the hyphen.
pub const SETUP_CODE_LENGTH: usize = 8;
/// Human-readable setup code alphabet, in Go's order.
pub const SETUP_CODE_ALPHABET: &str = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";
/// Default setup code lifetime: twenty minutes.
pub const DEFAULT_SETUP_EXPIRY: Duration = Duration::seconds(20 * 60);
/// Maximum setup code lifetime: seventy-two hours.
pub const MAX_SETUP_EXPIRY: Duration = Duration::seconds(72 * 60 * 60);
/// Default token lifetime: ninety days.
pub const DEFAULT_TOKEN_EXPIRY: Duration = Duration::seconds(90 * 24 * 60 * 60);
/// Maximum token lifetime: 365 days.
pub const MAX_TOKEN_EXPIRY: Duration = Duration::seconds(365 * 24 * 60 * 60);
/// Unauthenticated exchange procedure, as generated from the API schema.
pub const EXCHANGE_SETUP_CODE_PROCEDURE: &str = "/construct.v1.AuthService/ExchangeSetupCode";

/// Authentication method recorded in the handler's identity extension.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum AuthMethod {
    /// No established authentication method.
    #[default]
    Unspecified,
    /// Trusted local Unix socket.
    UnixSocket,
    /// Stored bearer token.
    Token,
}
impl fmt::Display for AuthMethod {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(match self {
            Self::Unspecified => "unspecified",
            Self::UnixSocket => "unix_socket",
            Self::Token => "token",
        })
    }
}

/// Authenticated principal, available through `Request::extensions.get::<Identity>()`.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Identity {
    /// Token name or `local-admin`.
    pub subject: String,
    /// How the caller authenticated.
    pub auth_method: AuthMethod,
    /// True only for a trusted local transport.
    pub is_admin: bool,
    /// Token expiry, or Go's zero time for local administration.
    pub expires_at: Timestamp,
}

/// Pending single-use setup exchange.
#[derive(Clone, Debug)]
pub struct PendingSetupCode {
    /// Uppercase code in XXXX-XXXX format.
    pub code: String,
    /// Requested token name.
    pub token_name: String,
    /// Lifetime of the token to be issued.
    pub token_expiry: Duration,
    /// Setup exchange expiry instant.
    pub expires_at: Timestamp,
}

/// Setup creation or cryptographic random source failure.
#[derive(Debug, thiserror::Error)]
pub enum TokenError {
    /// Setup code lifetime exceeds Go's maximum.
    #[error("code expiry exceeds maximum of 72h0m0s")]
    CodeExpiry,
    /// Token lifetime exceeds Go's maximum.
    #[error("token expiry exceeds maximum of 8760h0m0s")]
    TokenExpiry,
    /// Operating system randomness failed.
    #[error("failed to generate random bytes: {0}")]
    Random(#[from] rand::Error),
}

/// Cryptographic token generation and process-local setup code storage.
#[derive(Default)]
pub struct TokenProvider {
    pending_codes: Mutex<HashMap<String, PendingSetupCode>>,
}
impl TokenProvider {
    /// Construct an empty setup code provider.
    pub fn new() -> Self {
        Self::default()
    }

    /// Generate a plaintext bearer token and its SHA-256 hash.
    pub fn generate_token(&self) -> Result<(String, Vec<u8>), TokenError> {
        let mut bytes = [0; TOKEN_LENGTH];
        OsRng.try_fill_bytes(&mut bytes)?;
        let token = format!("{TOKEN_PREFIX}{}", TOKEN_ENCODING.encode(bytes));
        let hash = self.hash_token(&token);
        Ok((token, hash))
    }

    /// Hash the entire plaintext token, including its prefix.
    pub fn hash_token(&self, token: &str) -> Vec<u8> {
        Sha256::digest(token.as_bytes()).to_vec()
    }

    /// Check prefix, unpadded URL-safe base64 encoding, and decoded length.
    pub fn validate_token_format(&self, token: &str) -> bool {
        token
            .strip_prefix(TOKEN_PREFIX)
            .and_then(|s| TOKEN_ENCODING.decode(s.replace(['\r', '\n'], "")).ok())
            .is_some_and(|b| b.len() == TOKEN_LENGTH)
    }

    /// Create a code with explicit lifetimes, rejecting lifetimes above the Go maxima.
    pub fn create_setup_code(
        &self,
        token_name: &str,
        code_expiry: Duration,
        token_expiry: Duration,
    ) -> Result<PendingSetupCode, TokenError> {
        if code_expiry > MAX_SETUP_EXPIRY {
            return Err(TokenError::CodeExpiry);
        }
        if token_expiry > MAX_TOKEN_EXPIRY {
            return Err(TokenError::TokenExpiry);
        }
        let mut bytes = [0; SETUP_CODE_LENGTH];
        OsRng.try_fill_bytes(&mut bytes)?;
        let mut code = String::with_capacity(9);
        for (i, b) in bytes.into_iter().enumerate() {
            if i == 4 {
                code.push('-');
            }
            code.push(
                SETUP_CODE_ALPHABET.as_bytes()[usize::from(b) % SETUP_CODE_ALPHABET.len()] as char,
            );
        }
        let pending = PendingSetupCode {
            code,
            token_name: token_name.into(),
            token_expiry,
            expires_at: Timestamp(Utc::now() + code_expiry),
        };
        self.pending_codes
            .lock()
            .unwrap()
            .insert(pending.code.to_uppercase(), pending.clone());
        Ok(pending)
    }

    /// Atomically remove and return an unexpired code, ignoring case.
    pub fn consume_setup_code(&self, code: &str) -> Option<PendingSetupCode> {
        self.pending_codes
            .lock()
            .unwrap()
            .remove(&code.to_uppercase())
            .filter(|p| Utc::now() <= p.expires_at.0)
    }
}

/// Authentication hook for Connect unary and server-streaming calls.
pub struct AuthInterceptor {
    store: Arc<dyn TokenStore>,
    token_provider: Arc<TokenProvider>,
}
impl AuthInterceptor {
    /// Construct authentication backed by a repository and token provider.
    pub fn new(store: Arc<dyn TokenStore>, token_provider: Arc<TokenProvider>) -> Self {
        Self {
            store,
            token_provider,
        }
    }

    /// Authenticate a procedure and transport without mutating handler context.
    pub async fn authenticate(
        &self,
        procedure: &str,
        headers: &http::HeaderMap,
        transport: Option<Transport>,
    ) -> Result<Option<Identity>, ConnectError> {
        if procedure == EXCHANGE_SETUP_CODE_PROCEDURE {
            return Ok(None);
        }
        if transport == Some(Transport::Unix) {
            return Ok(Some(Identity {
                subject: "local-admin".into(),
                auth_method: AuthMethod::UnixSocket,
                is_admin: true,
                expires_at: Timestamp::default(),
            }));
        }
        let fail = |message| ConnectError::new(Code::Unauthenticated, message);
        let header = headers
            .get(http::header::AUTHORIZATION)
            .and_then(|h| h.to_str().ok())
            .unwrap_or_default();
        if header.is_empty() {
            return Err(fail("missing authorization header"));
        }
        let Some(("Bearer", token)) = header.split_once(' ') else {
            return Err(fail("invalid authorization format"));
        };
        if !token.starts_with(TOKEN_PREFIX) {
            return Err(fail("invalid token format"));
        }
        let tokens = self
            .store
            .list_tokens(
                TokenFilter {
                    expires_after: Some(Timestamp(Utc::now())),
                    token_hash: Some(self.token_provider.hash_token(token)),
                    ..Default::default()
                },
                ListOptions {
                    limit: Some(1),
                    ..Default::default()
                },
            )
            .await
            .map_err(|e| match e {
                StoreError::NotFound(_) => fail("invalid or expired token"),
                _ => ConnectError::new(Code::Internal, format!("failed to validate token: {e}")),
            })?;
        let token = tokens
            .into_iter()
            .next()
            .ok_or_else(|| fail("invalid or expired token"))?;
        Ok(Some(Identity {
            subject: token.name,
            auth_method: AuthMethod::Token,
            is_admin: false,
            expires_at: token.expires_at,
        }))
    }
}

#[construct_connect::async_trait]
impl RequestInterceptor for AuthInterceptor {
    async fn intercept(&self, request: &mut http::request::Parts) -> Result<(), ConnectError> {
        if let Some(identity) = self
            .authenticate(
                request.uri.path(),
                &request.headers,
                request.extensions.get::<Transport>().copied(),
            )
            .await
            .map_err(|e| ConnectError::new(Code::Unauthenticated, e.to_string()))?
        {
            request.extensions.insert(identity);
        }
        Ok(())
    }
}
