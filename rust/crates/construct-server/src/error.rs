//! Go API error classification and message sanitization.
use construct_connect::{Code, ConnectError};
use construct_core::store::StoreError;
use std::error::Error;

/// Remove the Go storage implementation prefix from a diagnostic.
pub fn sanitize_error(error: impl std::fmt::Display) -> String {
    error.to_string().replace("memory: ", "")
}

/// Preserve coded RPC errors; classify missing rows and otherwise report internal failures.
pub fn api_error(error: &(dyn Error + 'static)) -> ConnectError {
    let mut current = Some(error);
    let mut not_found = false;
    while let Some(cause) = current {
        if let Some(error) = cause.downcast_ref::<ConnectError>() {
            if error.code != Code::Unknown {
                return error.clone();
            }
        }
        not_found |= matches!(
            cause.downcast_ref::<StoreError>(),
            Some(StoreError::NotFound(_))
        );
        current = cause.source();
    }
    let code = if not_found {
        Code::NotFound
    } else {
        Code::Internal
    };
    ConnectError::new(code, sanitize_error(error))
}
