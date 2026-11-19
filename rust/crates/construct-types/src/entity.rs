//! Database scalar records, without Ent edges or wire serialization.
//! JSON columns: Task.tool_uses, Message.content/usage, Model.capabilities.
//! Nullable foreign keys and optional JSON/time fields use Option; optional
//! scalar columns retain Ent's zero-value representation. These are data records,
//! not constructors: Default does not apply database defaults such as enabled=true.
use crate::{memory::*, Timestamp};
use std::collections::BTreeMap;
use uuid::Uuid;

/// Scalar columns of Go's memory `Agent`, including the Time mixin.
#[derive(Clone, Debug, Default)]
pub struct Agent {
    /// Database `id` column.
    pub id: Uuid,
    /// Database `name` column.
    pub name: String,
    /// Database `description` column.
    pub description: String,
    /// Database `instructions` column.
    pub instructions: String,
    /// Database `builtin` column.
    pub builtin: bool,
    /// Database `model_id` column.
    pub model_id: Option<Uuid>,
    /// Database `create_time` column.
    pub create_time: Timestamp,
    /// Database `update_time` column.
    pub update_time: Timestamp,
}

/// Scalar columns of Go's memory `Task`, including the Time mixin.
#[derive(Clone, Debug, Default)]
pub struct Task {
    /// Database `id` column.
    pub id: Uuid,
    /// Database `project_directory` column.
    pub project_directory: String,
    /// Database `input_tokens` column.
    pub input_tokens: i64,
    /// Database `output_tokens` column.
    pub output_tokens: i64,
    /// Database `cache_write_tokens` column.
    pub cache_write_tokens: i64,
    /// Database `cache_read_tokens` column.
    pub cache_read_tokens: i64,
    /// Database `cost` column.
    pub cost: f64,
    /// Database `turns` column.
    pub turns: i64,
    /// Database `tool_uses` column.
    pub tool_uses: Option<BTreeMap<String, i64>>,
    /// Database `desired_phase` column.
    pub desired_phase: TaskPhase,
    /// Database `phase` column.
    pub phase: TaskPhase,
    /// Database `description` column.
    pub description: String,
    /// Database `agent_id` column.
    pub agent_id: Option<Uuid>,
    /// Database `create_time` column.
    pub create_time: Timestamp,
    /// Database `update_time` column.
    pub update_time: Timestamp,
}

/// Scalar columns of Go's memory `Message`, including the Time mixin.
#[derive(Clone, Debug, Default)]
pub struct Message {
    /// Database `id` column.
    pub id: Uuid,
    /// Database `source` column.
    pub source: MessageSource,
    /// Database `content` column.
    pub content: Option<MessageContent>,
    /// Database `usage` column.
    pub usage: Option<MessageUsage>,
    /// Database `processed_time` column.
    pub processed_time: Option<Timestamp>,
    /// Database `task_id` column.
    pub task_id: Uuid,
    /// Database `agent_id` column.
    pub agent_id: Option<Uuid>,
    /// Database `model_id` column.
    pub model_id: Option<Uuid>,
    /// Database `create_time` column.
    pub create_time: Timestamp,
    /// Database `update_time` column.
    pub update_time: Timestamp,
}

/// Scalar columns of Go's memory `Model`, including the Time mixin.
#[derive(Clone, Debug, Default)]
pub struct Model {
    /// Database `id` column.
    pub id: Uuid,
    /// Database `name` column.
    pub name: String,
    /// Database `context_window` column.
    pub context_window: i64,
    /// Database `capabilities` column.
    pub capabilities: Option<Vec<ModelCapability>>,
    /// Database `input_cost` column.
    pub input_cost: f64,
    /// Database `output_cost` column.
    pub output_cost: f64,
    /// Database `cache_write_cost` column.
    pub cache_write_cost: f64,
    /// Database `cache_read_cost` column.
    pub cache_read_cost: f64,
    /// Database `enabled` column.
    pub enabled: bool,
    /// Database `alias` column.
    pub alias: String,
    /// Database `model_provider_id` column.
    pub model_provider_id: Uuid,
    /// Database `create_time` column.
    pub create_time: Timestamp,
    /// Database `update_time` column.
    pub update_time: Timestamp,
}

/// Scalar columns of Go's memory `ModelProvider`, including the Time mixin.
#[derive(Clone, Debug, Default)]
pub struct ModelProvider {
    /// Database `id` column.
    pub id: Uuid,
    /// Database `name` column.
    pub name: String,
    /// Database `provider_type` column.
    pub provider_type: ModelProviderType,
    /// Database `url` column.
    pub url: String,
    /// Database `secret` column.
    pub secret: Vec<u8>,
    /// Database `enabled` column.
    pub enabled: bool,
    /// Database `create_time` column.
    pub create_time: Timestamp,
    /// Database `update_time` column.
    pub update_time: Timestamp,
}

/// Scalar columns of Go's memory `Token`, including the Time mixin.
#[derive(Clone, Debug, Default)]
pub struct Token {
    /// Database `id` column.
    pub id: Uuid,
    /// Database `name` column.
    pub name: String,
    /// Database `type` column.
    pub r#type: TokenType,
    /// Database `token_hash` column.
    pub token_hash: Vec<u8>,
    /// Database `description` column.
    pub description: String,
    /// Database `expires_at` column.
    pub expires_at: Timestamp,
    /// Database `create_time` column.
    pub create_time: Timestamp,
    /// Database `update_time` column.
    pub update_time: Timestamp,
}
