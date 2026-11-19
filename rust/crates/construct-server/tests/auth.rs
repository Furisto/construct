use chrono::{Duration, Utc};
use construct_connect::{
    http::{self, HeaderMap},
    Code, RequestInterceptor, Transport,
};
use construct_core::{
    store::{ListOptions, StoreError, TokenFilter, TokenStore},
    testing::MemoryStore,
};
use construct_proto::construct::v1 as pb;
use construct_server::auth::*;
use construct_types::{entity::Token, Timestamp};
use std::sync::Arc;
use uuid::Uuid;

#[test]
fn generated_exchange_path_and_constants_match_go() {
    assert_eq!(
        EXCHANGE_SETUP_CODE_PROCEDURE,
        pb::auth_service::METHODS
            .iter()
            .find(|m| m.name == "ExchangeSetupCode")
            .unwrap()
            .path
    );
    assert_eq!(DEFAULT_SETUP_EXPIRY.num_seconds(), 1200);
    assert_eq!(MAX_SETUP_EXPIRY.num_seconds(), 259200);
    assert_eq!(DEFAULT_TOKEN_EXPIRY.num_seconds(), 7776000);
    assert_eq!(MAX_TOKEN_EXPIRY.num_seconds(), 31536000);
    assert_eq!(AuthMethod::default().to_string(), "unspecified");
    assert_eq!(AuthMethod::UnixSocket.to_string(), "unix_socket");
    assert_eq!(AuthMethod::Token.to_string(), "token");
}

#[test]
fn token_generation_hash_and_validation() {
    let p = TokenProvider::new();
    let (a, hash_a) = p.generate_token().unwrap();
    let (b, hash_b) = p.generate_token().unwrap();
    assert!(a.starts_with(TOKEN_PREFIX));
    assert_eq!(a.len(), 46);
    assert_eq!(hash_a.len(), 32);
    assert_ne!(a, b);
    assert_ne!(hash_a, hash_b);
    assert_eq!(p.hash_token(&a), hash_a);
    assert_eq!(
        p.hash_token("abc"),
        [
            0xba, 0x78, 0x16, 0xbf, 0x8f, 0x01, 0xcf, 0xea, 0x41, 0x41, 0x40, 0xde, 0x5d, 0xae,
            0x22, 0x23, 0xb0, 0x03, 0x61, 0xa3, 0x96, 0x17, 0x7a, 0x9c, 0xb4, 0x10, 0xff, 0x61,
            0xf2, 0x00, 0x15, 0xad
        ]
    );
    assert!(p.validate_token_format(&a));
    assert!(p.validate_token_format(&format!("ct_{}\n", &a[3..])));
    assert!(p.validate_token_format("ct_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAB"));
    for value in [
        "",
        "abc123",
        "tk_abc123",
        "ct_!!!invalid",
        "ct_abc",
        "ct_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
        "ct_///////////////////////////////////////////",
    ] {
        assert!(!p.validate_token_format(value), "{value}");
    }
}

#[test]
fn setup_code_format_single_use_and_case_insensitivity() {
    let p = TokenProvider::new();
    let code = p
        .create_setup_code("test-token", DEFAULT_SETUP_EXPIRY, DEFAULT_TOKEN_EXPIRY)
        .unwrap();
    assert_eq!(code.code.len(), 9);
    assert_eq!(code.code.as_bytes()[4], b'-');
    assert!(code
        .code
        .chars()
        .filter(|c| *c != '-')
        .all(|c| SETUP_CODE_ALPHABET.contains(c)));
    let consumed = p.consume_setup_code(&code.code.to_lowercase()).unwrap();
    assert_eq!(consumed.token_name, "test-token");
    assert_eq!(consumed.token_expiry, DEFAULT_TOKEN_EXPIRY);
    assert_eq!(consumed.expires_at, code.expires_at);
    assert!(p.consume_setup_code(&code.code).is_none());
    assert!(p.consume_setup_code("ABCD-1234").is_none());
    assert!(p.consume_setup_code("").is_none());
}

#[test]
fn setup_code_expiry_limits_and_expired_consumption() {
    let p = TokenProvider::new();
    assert_eq!(
        p.create_setup_code(
            "test",
            MAX_SETUP_EXPIRY + Duration::seconds(1),
            DEFAULT_TOKEN_EXPIRY
        )
        .unwrap_err()
        .to_string(),
        "code expiry exceeds maximum of 72h0m0s"
    );
    assert_eq!(
        p.create_setup_code(
            "test",
            DEFAULT_SETUP_EXPIRY,
            MAX_TOKEN_EXPIRY + Duration::seconds(1)
        )
        .unwrap_err()
        .to_string(),
        "token expiry exceeds maximum of 8760h0m0s"
    );
    assert!(p
        .create_setup_code("test", MAX_SETUP_EXPIRY, MAX_TOKEN_EXPIRY)
        .is_ok());
    let code = p
        .create_setup_code("test", Duration::seconds(-1), DEFAULT_TOKEN_EXPIRY)
        .unwrap();
    assert!(p.consume_setup_code(&code.code).is_none());
    assert!(p.consume_setup_code(&code.code).is_none());
}

#[test]
fn setup_code_concurrent_consumption_has_one_winner() {
    let p = Arc::new(TokenProvider::new());
    let code = p
        .create_setup_code("test", DEFAULT_SETUP_EXPIRY, DEFAULT_TOKEN_EXPIRY)
        .unwrap();
    let threads: Vec<_> = (0..8)
        .map(|_| {
            let p = p.clone();
            let code = code.code.clone();
            std::thread::spawn(move || p.consume_setup_code(&code).is_some())
        })
        .collect();
    assert_eq!(
        threads
            .into_iter()
            .filter_map(|t| t.join().unwrap().then_some(()))
            .count(),
        1
    );
}

#[tokio::test]
async fn unix_is_admin_and_exchange_is_unauthenticated() {
    let a = AuthInterceptor::new(
        Arc::new(MemoryStore::default()),
        Arc::new(TokenProvider::new()),
    );
    let id = a
        .authenticate("/private", &HeaderMap::new(), Some(Transport::Unix))
        .await
        .unwrap()
        .unwrap();
    assert_eq!(id.subject, "local-admin");
    assert_eq!(id.auth_method, AuthMethod::UnixSocket);
    assert!(id.is_admin);
    assert_eq!(id.expires_at, Timestamp::default());
    for transport in [None, Some(Transport::Tcp), Some(Transport::Unix)] {
        assert!(a
            .authenticate(EXCHANGE_SETUP_CODE_PROCEDURE, &HeaderMap::new(), transport)
            .await
            .unwrap()
            .is_none());
    }
}

#[tokio::test]
async fn tcp_requires_exact_bearer_scheme_and_reports_go_errors() {
    let a = AuthInterceptor::new(
        Arc::new(MemoryStore::default()),
        Arc::new(TokenProvider::new()),
    );
    for (value, message) in [
        ("", "missing authorization header"),
        ("Bearer", "invalid authorization format"),
        ("bearer ct_abc", "invalid authorization format"),
        ("Basic ct_abc", "invalid authorization format"),
        ("Bearer abc", "invalid token format"),
        ("Bearer ct_abc", "invalid or expired token"),
        ("Bearer  ct_abc", "invalid token format"),
    ] {
        let mut headers = HeaderMap::new();
        if !value.is_empty() {
            headers.insert("authorization", value.parse().unwrap());
        }
        for transport in [None, Some(Transport::Tcp)] {
            let error = a
                .authenticate("/private", &headers, transport)
                .await
                .unwrap_err();
            assert_eq!(error.code, Code::Unauthenticated);
            assert_eq!(error.message, message);
        }
        let (mut parts, ()) = http::Request::builder()
            .uri("/private")
            .body(())
            .unwrap()
            .into_parts();
        parts.headers = headers;
        parts.extensions.insert(Transport::Tcp);
        let error = a.intercept(&mut parts).await.unwrap_err();
        assert_eq!(error.code, Code::Unauthenticated);
        assert_eq!(error.message, format!("unauthenticated: {message}"));
    }
}

#[tokio::test]
async fn tcp_looks_up_hash_and_checks_expiry_and_inserts_identity() {
    let store = Arc::new(MemoryStore::default());
    let p = Arc::new(TokenProvider::new());
    let (token, hash) = p.generate_token().unwrap();
    let expires_at = Timestamp(Utc::now() + Duration::hours(1));
    let mut record = Token {
        id: Uuid::from_u128(1),
        name: "client".into(),
        token_hash: hash,
        expires_at: expires_at.clone(),
        r#type: "api_token".into(),
        ..Default::default()
    };
    store.create_token(record.clone()).await.unwrap();
    let a = AuthInterceptor::new(store.clone(), p);
    let (mut parts, ()) = http::Request::builder()
        .uri("/private")
        .header("authorization", format!("Bearer {token}"))
        .body(())
        .unwrap()
        .into_parts();
    parts.extensions.insert(Transport::Tcp);
    a.intercept(&mut parts).await.unwrap();
    let identity = parts.extensions.get::<Identity>().unwrap();
    assert_eq!(identity.subject, "client");
    assert_eq!(identity.auth_method, AuthMethod::Token);
    assert!(!identity.is_admin);
    assert_eq!(identity.expires_at, expires_at);
    record.expires_at = Timestamp(Utc::now() - Duration::seconds(1));
    store.update_token(record).await.unwrap();
    assert_eq!(
        a.authenticate("/private", &parts.headers, Some(Transport::Tcp))
            .await
            .unwrap_err()
            .message,
        "invalid or expired token"
    );
}

#[tokio::test]
async fn storage_failures_are_classified_without_hiding_internal_errors() {
    struct FailingStore(StoreError);
    #[construct_core::async_trait]
    impl TokenStore for FailingStore {
        async fn create_token(&self, _: Token) -> Result<Token, StoreError> {
            unreachable!()
        }
        async fn get_token(&self, _: Uuid) -> Result<Token, StoreError> {
            unreachable!()
        }
        async fn update_token(&self, _: Token) -> Result<Token, StoreError> {
            unreachable!()
        }
        async fn delete_token(&self, _: Uuid) -> Result<(), StoreError> {
            unreachable!()
        }
        async fn list_tokens(
            &self,
            filter: TokenFilter,
            options: ListOptions,
        ) -> Result<Vec<Token>, StoreError> {
            assert!(filter.token_hash.is_some());
            assert!(filter.expires_after.is_some());
            assert!(filter.token_type.is_none());
            assert_eq!(options.limit, Some(1));
            Err(self.0.clone())
        }
    }
    let mut headers = HeaderMap::new();
    headers.insert("authorization", "Bearer ct_test".parse().unwrap());
    for (error, code, message) in [
        (
            StoreError::Internal("unavailable".into()),
            Code::Internal,
            "failed to validate token: internal store error: unavailable",
        ),
        (
            StoreError::NotFound("token".into()),
            Code::Unauthenticated,
            "invalid or expired token",
        ),
    ] {
        let a = AuthInterceptor::new(
            Arc::new(FailingStore(error)),
            Arc::new(TokenProvider::new()),
        );
        let error = a
            .authenticate("/private", &headers, Some(Transport::Tcp))
            .await
            .unwrap_err();
        assert_eq!(error.code, code);
        assert_eq!(error.message, message);
    }
}
