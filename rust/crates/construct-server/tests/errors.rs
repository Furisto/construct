use construct_connect::{Code, ConnectError};
use construct_core::store::StoreError;
use construct_server::error::*;

#[test]
fn classifications_and_sanitization_match_go() {
    assert_eq!(
        sanitize_error("memory: agent memory: not found"),
        "agent not found"
    );
    for (error, code) in [
        (StoreError::NotFound("memory: agent".into()), Code::NotFound),
        (
            StoreError::ConstraintViolation("duplicate".into()),
            Code::Internal,
        ),
        (StoreError::Invalid("bad".into()), Code::Internal),
        (
            StoreError::Internal("memory: broken".into()),
            Code::Internal,
        ),
    ] {
        let result = api_error(&error);
        assert_eq!(result.code, code);
        assert!(!result.message.contains("memory: "));
    }
    assert_eq!(
        api_error(&std::io::Error::other("memory: IO")).message,
        "IO"
    );
}

#[test]
fn coded_errors_retain_message_details_and_metadata() {
    let mut error = ConnectError::new(Code::PermissionDenied, "memory: keep this coded message");
    error.details.push(serde_json::json!({"reason": "denied"}));
    error.metadata.insert("x-error", "value".parse().unwrap());
    let result = api_error(&error);
    assert_eq!(result.code, error.code);
    assert_eq!(result.message, error.message);
    assert_eq!(result.details, error.details);
    assert_eq!(result.metadata, error.metadata);
    assert_eq!(
        api_error(&ConnectError::new(Code::Unknown, "unknown")).code,
        Code::Internal
    );
}

#[test]
fn wrapped_coded_and_not_found_errors_keep_their_classification() {
    #[derive(Debug, thiserror::Error)]
    #[error("context: {0}")]
    struct Context(#[source] Box<dyn std::error::Error>);
    let error = Context(Box::new(ConnectError::new(Code::NotFound, "missing")));
    let result = api_error(&error);
    assert_eq!(result.code, Code::NotFound);
    assert_eq!(result.message, "missing");
    let error = Context(Box::new(StoreError::NotFound("memory: agent".into())));
    let result = api_error(&error);
    assert_eq!(result.code, Code::NotFound);
    assert_eq!(result.message, "context: not found: agent");
}
