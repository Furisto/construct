//! Conversions between stored domain records and the Go-compatible API messages.
mod entities;
mod events;
mod messages;
mod tools;

pub use entities::*;
pub use events::*;
pub use messages::*;
pub use tools::{convert_tool_input_to_proto, convert_tool_output_to_proto};

use construct_types::Timestamp;
use uuid::Uuid;

/// Invalid domain or wire value encountered during API conversion.
#[derive(Debug, thiserror::Error)]
#[error("{0}")]
pub struct ConversionError(pub String);

/// Missing entity required by a conversion.
#[derive(Debug, thiserror::Error)]
#[error("missing related entity: {entity}")]
pub struct MissingRelatedEntityError {
    /// Name of the missing related entity.
    pub entity: String,
}

/// Convert a UUID to text, representing the nil UUID as empty.
pub fn convert_uuid_to_string(id: Uuid) -> String {
    if id.is_nil() {
        String::new()
    } else {
        id.to_string()
    }
}
/// Convert a nil UUID to an absent optional wire string.
pub fn convert_uuid_ptr_to_string_ptr(id: Uuid) -> Option<String> {
    (!id.is_nil()).then(|| id.to_string())
}
/// Convert an optional UUID to text, retaining a present nil UUID's spelling.
pub fn convert_uuid_ptr_to_string(id: Option<Uuid>) -> String {
    id.map(|id| id.to_string()).unwrap_or_default()
}
/// Parse UUID text, treating empty as nil like Go.
pub fn convert_string_to_uuid(id: &str) -> Result<Uuid, ConversionError> {
    if id.is_empty() {
        Ok(Uuid::nil())
    } else {
        Uuid::parse_str(id).map_err(|e| ConversionError(e.to_string()))
    }
}
/// Parse an optional UUID string; absent means nil, present empty is invalid.
pub fn convert_string_ptr_to_uuid(id: Option<&str>) -> Result<Uuid, ConversionError> {
    id.map(Uuid::parse_str)
        .transpose()
        .map(|id| id.unwrap_or_default())
        .map_err(|e| ConversionError(e.to_string()))
}
/// Convert an instant without losing nanoseconds.
pub fn convert_time_to_timestamp(time: &Timestamp) -> pbjson_types::Timestamp {
    pbjson_types::Timestamp {
        seconds: time.0.timestamp(),
        nanos: time.0.timestamp_subsec_nanos() as i32,
    }
}
/// Decode a protobuf instant; absent timestamps have Go protobuf's Unix epoch value.
pub fn convert_timestamp_to_time(
    time: Option<&pbjson_types::Timestamp>,
) -> Result<Timestamp, ConversionError> {
    let (seconds, nanos) = time.map(|t| (t.seconds, t.nanos)).unwrap_or_default();
    let normalized = seconds
        .checked_add(i64::from(nanos).div_euclid(1_000_000_000))
        .and_then(|s| {
            chrono::DateTime::from_timestamp(s, i64::from(nanos).rem_euclid(1_000_000_000) as u32)
        });
    normalized
        .map(Timestamp)
        .ok_or_else(|| ConversionError("timestamp out of range".into()))
}
/// Match Go conv's fixed six-decimal formatting helper.
pub fn float64_to_proto_decimal(value: f64) -> construct_proto::google::r#type::Decimal {
    construct_proto::google::r#type::Decimal {
        value: format!("{value:.6}"),
    }
}
