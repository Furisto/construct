//! Internal event routing values and Go-compatible pattern matching.
use crate::Timestamp;
use uuid::Uuid;

/// Domain event, not a serialized wire envelope.
#[derive(Clone, Debug)]
pub struct StreamEvent<P = serde_json::Value> {
    /// Dotted event name, such as `task.created`.
    pub r#type: String,
    /// CRUD action, or empty for non-CRUD events.
    pub action: String,
    /// Time of the change.
    pub timestamp: Timestamp,
    /// Optional task scope.
    pub task_id: Option<Uuid>,
    /// Domain payload; routers may choose a concrete enum or generic value.
    pub payload: P,
}

/// Options corresponding to Go's `event.SubscribeOptions` (not serialized).
#[derive(Clone, Debug, Default)]
pub struct SubscribeOptions {
    /// Empty means all external events; patterns are two-part Go globs.
    pub event_types: Vec<String>,
    /// Optional task UUID string; empty means no task filter.
    pub task_id: String,
    /// Replay message.created events after this message, when task-scoped.
    pub replay_after_message_id: String,
    /// Permit explicit internal patterns (never enabled for external consumers).
    pub internal: bool,
}

/// Whether a name begins with the literal `internal.` prefix.
pub fn is_internal_event(event_type: &str) -> bool {
    event_type.starts_with("internal.")
}

/// Match exactly as Go's `matchPattern`, splitting only at the first dot.
/// `*.*` is not a general wildcard; internal events require `internal.` patterns.
pub fn match_pattern(pattern: &str, event_type: &str) -> bool {
    if is_internal_event(event_type) && !is_internal_event(pattern) {
        return false;
    }
    if pattern == "*" || pattern == event_type {
        return true;
    }
    let (Some((entity, action)), Some((event_entity, event_action))) =
        (pattern.split_once('.'), event_type.split_once('.'))
    else {
        return false;
    };
    (action == "*" && entity == event_entity) || (entity == "*" && action == event_action)
}
