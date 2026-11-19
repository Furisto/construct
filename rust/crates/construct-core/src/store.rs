//! Driver-independent repositories. All writes take complete records (not patches).
//! Callers assign UUIDs and timestamps, including update_time. Updates preserve
//! id/create_time. Implementations enforce schema constraints and atomically write
//! JSON columns using construct-types' Go-compatible encoding. Delete of a missing
//! row is NotFound. Multi-record changes must use begin/commit, not read-modify-write
//! on the root store. No Ent edges are returned: resolve references explicitly.
use async_trait::async_trait;
use construct_types::{entity::*, memory::*, Timestamp};
use uuid::Uuid;

/// Portable failures; backend diagnostics must not contain secrets.
#[derive(Clone, Debug, PartialEq, Eq, thiserror::Error)]
pub enum StoreError {
    /// Missing entity or reference lookup.
    #[error("not found: {0}")]
    NotFound(String),
    /// Unique/FK/check constraint or optimistic transaction conflict.
    #[error("constraint/conflict: {0}")]
    ConstraintViolation(String),
    /// Malformed record/filter or unsupported nested transaction.
    #[error("invalid: {0}")]
    Invalid(String),
    /// Other database failures.
    #[error("internal store error: {0}")]
    Internal(String),
}

/// Supported ordering across repositories; UUID is the deterministic tiebreaker.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum SortField {
    /// Time mixin create_time.
    #[default]
    CreatedAt,
    /// Time mixin update_time.
    UpdatedAt,
    /// Name column (Invalid for tasks/messages without a name).
    Name,
}
/// Ordering direction.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum SortOrder {
    /// Ascending.
    Ascending,
    /// Descending, Go's task listing default.
    #[default]
    Descending,
}
/// Pagination is applied after filtering and ordering. API adapters translate
/// opaque page tokens to offsets/cursors; these contracts do not store RPC tokens.
#[derive(Clone, Debug, Default)]
pub struct ListOptions {
    /// None means unbounded; Some(0) returns an empty list.
    pub limit: Option<usize>,
    /// Number of matching rows to skip.
    pub offset: usize,
    /// Column ordering.
    pub sort_field: SortField,
    /// Ordering direction.
    pub sort_order: SortOrder,
}

/// Agent API and built-in seeding predicates, combined with AND.
#[derive(Clone, Debug, Default)]
pub struct AgentFilter {
    /// Exact names (OR); empty means unrestricted.
    pub names: Vec<String>,
    /// Optional model reference.
    pub model_id: Option<Uuid>,
    /// Optional built-in marker.
    pub builtin: Option<bool>,
}
/// Task API/reconciler predicates, combined with AND.
#[derive(Clone, Debug, Default)]
pub struct TaskFilter {
    /// Assigned agent.
    pub agent_id: Option<Uuid>,
    /// Go SQLite LIKE prefix of canonical UUID text (case-insensitive ASCII,
    /// including hyphens; `%`/`_` retain LIKE wildcard semantics).
    pub id_prefix: Option<String>,
    /// Whether at least one message references the task.
    pub has_messages: Option<bool>,
    /// Current lifecycle phase.
    pub phase: Option<TaskPhase>,
    /// Desired lifecycle phase.
    pub desired_phase: Option<TaskPhase>,
}
/// Message API, reconciliation, and replay predicates, combined with AND.
#[derive(Clone, Debug, Default)]
pub struct MessageFilter {
    /// Task scope.
    pub task_id: Option<Uuid>,
    /// Agent scope.
    pub agent_id: Option<Uuid>,
    /// Source roles (OR); empty means unrestricted.
    pub sources: Vec<MessageSource>,
    /// Whether processed_time is set (Go's zero time counts as unset).
    pub processed: Option<bool>,
    /// Strict create_time bound for replay; resolve replay-after ID separately.
    pub created_after: Option<Timestamp>,
}
/// Model API predicates, combined with AND.
#[derive(Clone, Debug, Default)]
pub struct ModelFilter {
    /// Exact names (OR); empty means unrestricted.
    pub names: Vec<String>,
    /// Owning provider.
    pub model_provider_id: Option<Uuid>,
    /// Availability.
    pub enabled: Option<bool>,
}
/// Provider API predicates, combined with AND.
#[derive(Clone, Debug, Default)]
pub struct ModelProviderFilter {
    /// Availability.
    pub enabled: Option<bool>,
    /// Provider kinds (OR); empty means unrestricted.
    pub provider_types: Vec<ModelProviderType>,
}
/// Token listing, setup exchange, and authentication predicates.
#[derive(Clone, Debug, Default)]
pub struct TokenFilter {
    /// Literal name prefix (ASCII case-insensitive, matching Go's SQLite LIKE).
    pub name_prefix: String,
    /// API listing sets ApiToken; None includes setup codes too.
    pub token_type: Option<TokenType>,
    /// Strict expires_at bound. API include_expired=false supplies now here;
    /// None includes expired tokens. Explicit time keeps backends/fakes deterministic.
    pub expires_after: Option<Timestamp>,
    /// Exact hash lookup for authentication (never plaintext).
    pub token_hash: Option<Vec<u8>>,
}

/// Repository for Agent scalar records.
#[async_trait]
pub trait AgentStore: Send + Sync {
    /// Insert a fully initialized record; duplicate IDs/unique keys are ConstraintViolation.
    async fn create_agent(&self, record: Agent) -> Result<Agent, StoreError>;
    /// Get by primary key, or NotFound.
    async fn get_agent(&self, id: Uuid) -> Result<Agent, StoreError>;
    /// Replace mutable fields atomically; immutable create_time must be unchanged.
    async fn update_agent(&self, record: Agent) -> Result<Agent, StoreError>;
    /// Delete by ID; enforce FK constraints.
    async fn delete_agent(&self, id: Uuid) -> Result<(), StoreError>;
    /// Apply AND filters, sort, then paginate; an empty match is an empty Vec.
    async fn list_agents(
        &self,
        filter: AgentFilter,
        options: ListOptions,
    ) -> Result<Vec<Agent>, StoreError>;
}

/// Repository for Task scalar records.
#[async_trait]
pub trait TaskStore: Send + Sync {
    /// Insert a fully initialized record; duplicate IDs/unique keys are ConstraintViolation.
    async fn create_task(&self, record: Task) -> Result<Task, StoreError>;
    /// Get by primary key, or NotFound.
    async fn get_task(&self, id: Uuid) -> Result<Task, StoreError>;
    /// Replace mutable fields atomically; immutable create_time must be unchanged.
    async fn update_task(&self, record: Task) -> Result<Task, StoreError>;
    /// Delete by ID; enforce FK constraints and cascade task messages.
    async fn delete_task(&self, id: Uuid) -> Result<(), StoreError>;
    /// Apply AND filters, sort, then paginate; an empty match is an empty Vec.
    async fn list_tasks(
        &self,
        filter: TaskFilter,
        options: ListOptions,
    ) -> Result<Vec<Task>, StoreError>;
}

/// Repository for Message scalar records.
#[async_trait]
pub trait MessageStore: Send + Sync {
    /// Insert a fully initialized record; duplicate IDs/unique keys are ConstraintViolation.
    async fn create_message(&self, record: Message) -> Result<Message, StoreError>;
    /// Get by primary key, or NotFound.
    async fn get_message(&self, id: Uuid) -> Result<Message, StoreError>;
    /// Replace mutable fields atomically; immutable create_time must be unchanged.
    async fn update_message(&self, record: Message) -> Result<Message, StoreError>;
    /// Delete by ID; enforce FK constraints.
    async fn delete_message(&self, id: Uuid) -> Result<(), StoreError>;
    /// Apply AND filters, sort, then paginate; an empty match is an empty Vec.
    async fn list_messages(
        &self,
        filter: MessageFilter,
        options: ListOptions,
    ) -> Result<Vec<Message>, StoreError>;
}

/// Repository for Model scalar records.
#[async_trait]
pub trait ModelStore: Send + Sync {
    /// Insert a fully initialized record; duplicate IDs/unique keys are ConstraintViolation.
    async fn create_model(&self, record: Model) -> Result<Model, StoreError>;
    /// Get by primary key, or NotFound.
    async fn get_model(&self, id: Uuid) -> Result<Model, StoreError>;
    /// Replace mutable fields atomically; immutable create_time must be unchanged.
    async fn update_model(&self, record: Model) -> Result<Model, StoreError>;
    /// Delete by ID; enforce FK constraints.
    async fn delete_model(&self, id: Uuid) -> Result<(), StoreError>;
    /// Apply AND filters, sort, then paginate; an empty match is an empty Vec.
    async fn list_models(
        &self,
        filter: ModelFilter,
        options: ListOptions,
    ) -> Result<Vec<Model>, StoreError>;
}

/// Repository for ModelProvider scalar records.
#[async_trait]
pub trait ModelProviderStore: Send + Sync {
    /// Insert a fully initialized record; duplicate IDs/unique keys are ConstraintViolation.
    async fn create_model_provider(
        &self,
        record: ModelProvider,
    ) -> Result<ModelProvider, StoreError>;
    /// Get by primary key, or NotFound.
    async fn get_model_provider(&self, id: Uuid) -> Result<ModelProvider, StoreError>;
    /// Replace mutable fields atomically; immutable create_time must be unchanged.
    async fn update_model_provider(
        &self,
        record: ModelProvider,
    ) -> Result<ModelProvider, StoreError>;
    /// Delete by ID; enforce FK constraints.
    async fn delete_model_provider(&self, id: Uuid) -> Result<(), StoreError>;
    /// Apply AND filters, sort, then paginate; an empty match is an empty Vec.
    async fn list_model_providers(
        &self,
        filter: ModelProviderFilter,
        options: ListOptions,
    ) -> Result<Vec<ModelProvider>, StoreError>;
}

/// Repository for Token scalar records.
#[async_trait]
pub trait TokenStore: Send + Sync {
    /// Insert a fully initialized record; duplicate IDs/unique keys are ConstraintViolation.
    async fn create_token(&self, record: Token) -> Result<Token, StoreError>;
    /// Get by primary key, or NotFound.
    async fn get_token(&self, id: Uuid) -> Result<Token, StoreError>;
    /// Replace mutable fields atomically; immutable create_time must be unchanged.
    async fn update_token(&self, record: Token) -> Result<Token, StoreError>;
    /// Delete by ID; enforce FK constraints.
    async fn delete_token(&self, id: Uuid) -> Result<(), StoreError>;
    /// Apply AND filters, sort, then paginate; an empty match is an empty Vec.
    async fn list_tokens(
        &self,
        filter: TokenFilter,
        options: ListOptions,
    ) -> Result<Vec<Token>, StoreError>;
}

/// Unified repository set, usable as Arc<dyn Store>. Transactions must span all
/// repositories on one database connection and isolate writes until commit.
#[async_trait]
pub trait Store:
    AgentStore + TaskStore + MessageStore + ModelStore + ModelProviderStore + TokenStore
{
    /// Begin a transaction. Unsupported nested transactions return Invalid.
    /// Adapters may hold a pool-owned connection or an actor-owned rusqlite connection.
    async fn begin(&self) -> Result<Box<dyn StoreTransaction>, StoreError>;
}

/// Transaction-scoped repositories. Drop without successful commit rolls back;
/// commit failure must not leak partial writes. Implementations must also roll
/// back on cancellation/panic (e.g. sqlx transaction drop or actor cleanup).
#[async_trait]
pub trait StoreTransaction: Store {
    /// Commit atomically; consumes the transaction so it cannot be reused.
    async fn commit(self: Box<Self>) -> Result<(), StoreError>;
    /// Roll back explicitly; consumes the transaction.
    async fn rollback(self: Box<Self>) -> Result<(), StoreError>;
}
