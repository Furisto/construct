mod agents;
mod auth;
mod events;
mod messages;
mod models;
mod providers;
mod tasks;
#[cfg(test)]
mod tests;

use crate::{auth::TokenProvider, conv, error::api_error, Analytics};
use chrono::Utc;
use construct_connect::{async_trait, Code, ConnectError, Request, Response};
use construct_core::store::*;
use construct_event::{EventPublisher, EventRouter};
use construct_proto::construct::v1 as pb;
use construct_secret::Encryption;
use construct_types::{entity as db, Timestamp};
use std::sync::Arc;
use uuid::Uuid;

#[derive(Clone)]
pub(crate) struct Services {
    pub store: Arc<dyn Store>,
    pub encryption: Arc<Encryption>,
    pub events: Arc<EventRouter>,
    pub analytics: Arc<dyn Analytics>,
    pub tokens: Arc<TokenProvider>,
}

fn now() -> Timestamp {
    Timestamp(Utc::now())
}
fn invalid(message: impl Into<String>) -> ConnectError {
    ConnectError::new(Code::InvalidArgument, message)
}
fn internal(error: impl std::error::Error + 'static) -> ConnectError {
    api_error(&error)
}
fn parse_id(value: &str, field: &str) -> Result<Uuid, ConnectError> {
    let bytes = value.as_bytes();
    let raw = match bytes.len() {
        32 | 36 => bytes,
        38 => &bytes[1..37],
        45 => {
            if !bytes[..9].eq_ignore_ascii_case(b"urn:uuid:") {
                return Err(invalid(format!(
                    "invalid {field} format: invalid urn prefix: {:?}",
                    String::from_utf8_lossy(&bytes[..9])
                )));
            }
            &bytes[9..]
        }
        len => {
            return Err(invalid(format!(
                "invalid {field} format: invalid UUID length: {len}"
            )))
        }
    };
    Uuid::try_parse_ascii(raw)
        .map_err(|_| invalid(format!("invalid {field} format: invalid UUID format")))
}
trait StoreResult<T> {
    fn api(self, entity: &str) -> Result<T, ConnectError>;
}
impl<T> StoreResult<T> for Result<T, StoreError> {
    fn api(self, entity: &str) -> Result<T, ConnectError> {
        self.map_err(|error| {
            let mut rpc = api_error(&error);
            if matches!(error, StoreError::NotFound(_)) {
                rpc.message = format!("{entity} not found");
            }
            rpc
        })
    }
}
