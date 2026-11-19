//! HashMap store fake with snapshot transactions and basic relational constraints.
use crate::store::*;
use async_trait::async_trait;
use construct_types::{entity::*, Timestamp};
use std::{
    collections::{HashMap, HashSet},
    sync::{Arc, Mutex},
};
use uuid::Uuid;

#[derive(Clone, Default)]
struct Data {
    agents: HashMap<Uuid, Agent>,
    tasks: HashMap<Uuid, Task>,
    messages: HashMap<Uuid, Message>,
    models: HashMap<Uuid, Model>,
    model_providers: HashMap<Uuid, ModelProvider>,
    tokens: HashMap<Uuid, Token>,
}
#[derive(Default)]
struct Versioned {
    data: Data,
    revision: u64,
}

/// HashMap-backed fake enforcing unique keys, references, the agent/model message
/// check, immutable create_time, and task-message cascade. It does not implement
/// all Ent field validators/defaults or emit events. Callers supply initialized
/// records. Transactions use snapshots and reject conflicting root writes.
#[derive(Clone, Default)]
pub struct MemoryStore {
    inner: Arc<Mutex<Versioned>>,
}
struct Transaction {
    local: MemoryStore,
    parent: Arc<Mutex<Versioned>>,
    base_revision: u64,
}

fn constraint(message: &str) -> StoreError {
    StoreError::ConstraintViolation(message.into())
}
fn require(condition: bool, message: &str) -> Result<(), StoreError> {
    if condition {
        Ok(())
    } else {
        Err(constraint(message))
    }
}
impl Data {
    fn validate(&self) -> Result<(), StoreError> {
        let mut names = HashSet::new();
        for a in self.agents.values() {
            require(!a.name.is_empty(), "agent name required")?;
            require(names.insert(&a.name), "agent name must be unique")?;
            require(
                a.model_id.is_none_or(|id| self.models.contains_key(&id)),
                "agent model reference",
            )?;
        }
        let mut models = HashSet::new();
        for m in self.models.values() {
            require(
                self.model_providers.contains_key(&m.model_provider_id),
                "model provider reference",
            )?;
            require(
                models.insert((&m.name, m.model_provider_id)),
                "model/provider name must be unique",
            )?;
        }
        for p in self.model_providers.values() {
            require(!p.name.is_empty(), "provider name required")?;
            require(!p.secret.is_empty(), "provider secret required")?;
        }
        for t in self.tasks.values() {
            require(
                t.agent_id.is_none_or(|id| self.agents.contains_key(&id)),
                "task agent reference",
            )?;
        }
        for m in self.messages.values() {
            require(
                self.tasks.contains_key(&m.task_id),
                "message task reference",
            )?;
            require(
                m.agent_id.is_none_or(|id| self.agents.contains_key(&id)),
                "message agent reference",
            )?;
            require(
                m.model_id.is_none_or(|id| self.models.contains_key(&id)),
                "message model reference",
            )?;
            require(
                m.agent_id.is_none() || m.model_id.is_some(),
                "message agent requires model",
            )?;
        }
        let (mut names, mut hashes) = (HashSet::new(), HashSet::new());
        for t in self.tokens.values() {
            require(!t.name.is_empty(), "token name required")?;
            require(names.insert(&t.name), "token name must be unique")?;
            require(hashes.insert(&t.token_hash), "token hash must be unique")?;
        }
        Ok(())
    }
}

trait Row {
    fn id(&self) -> Uuid;
    fn created(&self) -> &Timestamp;
    fn updated(&self) -> &Timestamp;
    fn name(&self) -> Option<&str>;
}

fn uuid_has_prefix(id: Uuid, prefix: &str) -> bool {
    let text = id.to_string();
    let mut matched = vec![false; text.len() + 1];
    matched[0] = true;
    for c in prefix
        .bytes()
        .map(|c| c.to_ascii_lowercase())
        .chain(std::iter::once(b'%'))
    {
        if c == b'%' {
            for i in 1..matched.len() {
                matched[i] |= matched[i - 1];
            }
        } else {
            for i in (1..matched.len()).rev() {
                matched[i] = matched[i - 1] && (c == b'_' || c == text.as_bytes()[i - 1]);
            }
            matched[0] = false;
        }
    }
    matched[text.len()]
}
fn page<T: Row>(mut rows: Vec<T>, options: ListOptions) -> Result<Vec<T>, StoreError> {
    if options.sort_field == SortField::Name && rows.iter().any(|r| r.name().is_none()) {
        return Err(StoreError::Invalid("entity has no name column".into()));
    }
    rows.sort_by(|a, b| {
        let order = match options.sort_field {
            SortField::CreatedAt => a.created().cmp(b.created()),
            SortField::UpdatedAt => a.updated().cmp(b.updated()),
            SortField::Name => a.name().cmp(&b.name()),
        }
        .then_with(|| a.id().cmp(&b.id()));
        if options.sort_order == SortOrder::Descending {
            order.reverse()
        } else {
            order
        }
    });
    Ok(rows
        .into_iter()
        .skip(options.offset)
        .take(options.limit.unwrap_or(usize::MAX))
        .collect())
}

#[async_trait]
impl Store for MemoryStore {
    async fn begin(&self) -> Result<Box<dyn StoreTransaction>, StoreError> {
        let state = self.inner.lock().expect("store fake poisoned");
        Ok(Box::new(Transaction {
            local: MemoryStore {
                inner: Arc::new(Mutex::new(Versioned {
                    data: state.data.clone(),
                    revision: 0,
                })),
            },
            parent: self.inner.clone(),
            base_revision: state.revision,
        }))
    }
}
#[async_trait]
impl Store for Transaction {
    async fn begin(&self) -> Result<Box<dyn StoreTransaction>, StoreError> {
        Err(StoreError::Invalid(
            "nested transactions unsupported".into(),
        ))
    }
}
#[async_trait]
impl StoreTransaction for Transaction {
    async fn commit(self: Box<Self>) -> Result<(), StoreError> {
        let data = self
            .local
            .inner
            .lock()
            .expect("store fake poisoned")
            .data
            .clone();
        let mut parent = self.parent.lock().expect("store fake poisoned");
        if parent.revision != self.base_revision {
            return Err(constraint("transaction snapshot conflict"));
        }
        parent.data = data;
        parent.revision += 1;
        Ok(())
    }
    async fn rollback(self: Box<Self>) -> Result<(), StoreError> {
        Ok(())
    }
}

impl Row for Agent {
    fn id(&self) -> Uuid {
        self.id
    }
    fn created(&self) -> &Timestamp {
        &self.create_time
    }
    fn updated(&self) -> &Timestamp {
        &self.update_time
    }
    fn name(&self) -> Option<&str> {
        Some(&self.name)
    }
}
#[async_trait]
impl AgentStore for MemoryStore {
    async fn create_agent(&self, record: Agent) -> Result<Agent, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        if state.data.agents.contains_key(&record.id) {
            return Err(constraint("duplicate agent id"));
        }
        let mut data = state.data.clone();
        data.agents.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn get_agent(&self, id: Uuid) -> Result<Agent, StoreError> {
        self.inner
            .lock()
            .expect("store fake poisoned")
            .data
            .agents
            .get(&id)
            .cloned()
            .ok_or_else(|| StoreError::NotFound(id.to_string()))
    }
    async fn update_agent(&self, record: Agent) -> Result<Agent, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let old = state
            .data
            .agents
            .get(&record.id)
            .ok_or_else(|| StoreError::NotFound(record.id.to_string()))?;
        if old.create_time != record.create_time {
            return Err(StoreError::Invalid("create_time is immutable".into()));
        }
        let mut data = state.data.clone();
        data.agents.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn delete_agent(&self, id: Uuid) -> Result<(), StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let mut data = state.data.clone();
        if data.agents.remove(&id).is_none() {
            return Err(StoreError::NotFound(id.to_string()));
        }

        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(())
    }
    async fn list_agents(
        &self,
        filter: AgentFilter,
        options: ListOptions,
    ) -> Result<Vec<Agent>, StoreError> {
        let state = self.inner.lock().expect("store fake poisoned");
        let data = &state.data;
        let rows = data
            .agents
            .values()
            .filter(|r| {
                (filter.names.is_empty() || filter.names.contains(&r.name))
                    && filter.model_id.is_none_or(|v| r.model_id == Some(v))
                    && filter.builtin.is_none_or(|v| r.builtin == v)
            })
            .cloned()
            .collect();
        page(rows, options)
    }
}
#[async_trait]
impl AgentStore for Transaction {
    async fn create_agent(&self, record: Agent) -> Result<Agent, StoreError> {
        self.local.create_agent(record).await
    }
    async fn get_agent(&self, id: Uuid) -> Result<Agent, StoreError> {
        self.local.get_agent(id).await
    }
    async fn update_agent(&self, record: Agent) -> Result<Agent, StoreError> {
        self.local.update_agent(record).await
    }
    async fn delete_agent(&self, id: Uuid) -> Result<(), StoreError> {
        self.local.delete_agent(id).await
    }
    async fn list_agents(
        &self,
        filter: AgentFilter,
        options: ListOptions,
    ) -> Result<Vec<Agent>, StoreError> {
        self.local.list_agents(filter, options).await
    }
}

impl Row for Task {
    fn id(&self) -> Uuid {
        self.id
    }
    fn created(&self) -> &Timestamp {
        &self.create_time
    }
    fn updated(&self) -> &Timestamp {
        &self.update_time
    }
    fn name(&self) -> Option<&str> {
        None
    }
}
#[async_trait]
impl TaskStore for MemoryStore {
    async fn create_task(&self, record: Task) -> Result<Task, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        if state.data.tasks.contains_key(&record.id) {
            return Err(constraint("duplicate task id"));
        }
        let mut data = state.data.clone();
        data.tasks.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn get_task(&self, id: Uuid) -> Result<Task, StoreError> {
        self.inner
            .lock()
            .expect("store fake poisoned")
            .data
            .tasks
            .get(&id)
            .cloned()
            .ok_or_else(|| StoreError::NotFound(id.to_string()))
    }
    async fn update_task(&self, record: Task) -> Result<Task, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let old = state
            .data
            .tasks
            .get(&record.id)
            .ok_or_else(|| StoreError::NotFound(record.id.to_string()))?;
        if old.create_time != record.create_time {
            return Err(StoreError::Invalid("create_time is immutable".into()));
        }
        let mut data = state.data.clone();
        data.tasks.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn delete_task(&self, id: Uuid) -> Result<(), StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let mut data = state.data.clone();
        if data.tasks.remove(&id).is_none() {
            return Err(StoreError::NotFound(id.to_string()));
        }
        data.messages.retain(|_, m| m.task_id != id);
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(())
    }
    async fn list_tasks(
        &self,
        filter: TaskFilter,
        options: ListOptions,
    ) -> Result<Vec<Task>, StoreError> {
        if options.sort_field == SortField::Name {
            return Err(StoreError::Invalid("entity has no name column".into()));
        }
        let state = self.inner.lock().expect("store fake poisoned");
        let data = &state.data;
        let rows = data
            .tasks
            .values()
            .filter(|r| {
                filter.agent_id.is_none_or(|v| r.agent_id == Some(v))
                    && filter
                        .id_prefix
                        .as_ref()
                        .is_none_or(|v| uuid_has_prefix(r.id, v))
                    && filter
                        .has_messages
                        .is_none_or(|v| data.messages.values().any(|m| m.task_id == r.id) == v)
                    && filter.phase.as_ref().is_none_or(|v| &r.phase == v)
                    && filter
                        .desired_phase
                        .as_ref()
                        .is_none_or(|v| &r.desired_phase == v)
            })
            .cloned()
            .collect();
        page(rows, options)
    }
}
#[async_trait]
impl TaskStore for Transaction {
    async fn create_task(&self, record: Task) -> Result<Task, StoreError> {
        self.local.create_task(record).await
    }
    async fn get_task(&self, id: Uuid) -> Result<Task, StoreError> {
        self.local.get_task(id).await
    }
    async fn update_task(&self, record: Task) -> Result<Task, StoreError> {
        self.local.update_task(record).await
    }
    async fn delete_task(&self, id: Uuid) -> Result<(), StoreError> {
        self.local.delete_task(id).await
    }
    async fn list_tasks(
        &self,
        filter: TaskFilter,
        options: ListOptions,
    ) -> Result<Vec<Task>, StoreError> {
        self.local.list_tasks(filter, options).await
    }
}

impl Row for Message {
    fn id(&self) -> Uuid {
        self.id
    }
    fn created(&self) -> &Timestamp {
        &self.create_time
    }
    fn updated(&self) -> &Timestamp {
        &self.update_time
    }
    fn name(&self) -> Option<&str> {
        None
    }
}
#[async_trait]
impl MessageStore for MemoryStore {
    async fn create_message(&self, record: Message) -> Result<Message, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        if state.data.messages.contains_key(&record.id) {
            return Err(constraint("duplicate message id"));
        }
        let mut data = state.data.clone();
        data.messages.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn get_message(&self, id: Uuid) -> Result<Message, StoreError> {
        self.inner
            .lock()
            .expect("store fake poisoned")
            .data
            .messages
            .get(&id)
            .cloned()
            .ok_or_else(|| StoreError::NotFound(id.to_string()))
    }
    async fn update_message(&self, record: Message) -> Result<Message, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let old = state
            .data
            .messages
            .get(&record.id)
            .ok_or_else(|| StoreError::NotFound(record.id.to_string()))?;
        if old.create_time != record.create_time {
            return Err(StoreError::Invalid("create_time is immutable".into()));
        }
        let mut data = state.data.clone();
        data.messages.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn delete_message(&self, id: Uuid) -> Result<(), StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let mut data = state.data.clone();
        if data.messages.remove(&id).is_none() {
            return Err(StoreError::NotFound(id.to_string()));
        }

        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(())
    }
    async fn list_messages(
        &self,
        filter: MessageFilter,
        options: ListOptions,
    ) -> Result<Vec<Message>, StoreError> {
        if options.sort_field == SortField::Name {
            return Err(StoreError::Invalid("entity has no name column".into()));
        }
        let state = self.inner.lock().expect("store fake poisoned");
        let data = &state.data;
        let rows = data
            .messages
            .values()
            .filter(|r| {
                filter.task_id.is_none_or(|v| r.task_id == v)
                    && filter.agent_id.is_none_or(|v| r.agent_id == Some(v))
                    && (filter.sources.is_empty() || filter.sources.contains(&r.source))
                    && filter.processed.is_none_or(|v| {
                        r.processed_time
                            .as_ref()
                            .is_some_and(|t| t != &Timestamp::default())
                            == v
                    })
                    && filter
                        .created_after
                        .as_ref()
                        .is_none_or(|v| &r.create_time > v)
            })
            .cloned()
            .collect();
        page(rows, options)
    }
}
#[async_trait]
impl MessageStore for Transaction {
    async fn create_message(&self, record: Message) -> Result<Message, StoreError> {
        self.local.create_message(record).await
    }
    async fn get_message(&self, id: Uuid) -> Result<Message, StoreError> {
        self.local.get_message(id).await
    }
    async fn update_message(&self, record: Message) -> Result<Message, StoreError> {
        self.local.update_message(record).await
    }
    async fn delete_message(&self, id: Uuid) -> Result<(), StoreError> {
        self.local.delete_message(id).await
    }
    async fn list_messages(
        &self,
        filter: MessageFilter,
        options: ListOptions,
    ) -> Result<Vec<Message>, StoreError> {
        self.local.list_messages(filter, options).await
    }
}

impl Row for Model {
    fn id(&self) -> Uuid {
        self.id
    }
    fn created(&self) -> &Timestamp {
        &self.create_time
    }
    fn updated(&self) -> &Timestamp {
        &self.update_time
    }
    fn name(&self) -> Option<&str> {
        Some(&self.name)
    }
}
#[async_trait]
impl ModelStore for MemoryStore {
    async fn create_model(&self, record: Model) -> Result<Model, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        if state.data.models.contains_key(&record.id) {
            return Err(constraint("duplicate model id"));
        }
        let mut data = state.data.clone();
        data.models.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn get_model(&self, id: Uuid) -> Result<Model, StoreError> {
        self.inner
            .lock()
            .expect("store fake poisoned")
            .data
            .models
            .get(&id)
            .cloned()
            .ok_or_else(|| StoreError::NotFound(id.to_string()))
    }
    async fn update_model(&self, record: Model) -> Result<Model, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let old = state
            .data
            .models
            .get(&record.id)
            .ok_or_else(|| StoreError::NotFound(record.id.to_string()))?;
        if old.create_time != record.create_time {
            return Err(StoreError::Invalid("create_time is immutable".into()));
        }
        let mut data = state.data.clone();
        data.models.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn delete_model(&self, id: Uuid) -> Result<(), StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let mut data = state.data.clone();
        if data.models.remove(&id).is_none() {
            return Err(StoreError::NotFound(id.to_string()));
        }

        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(())
    }
    async fn list_models(
        &self,
        filter: ModelFilter,
        options: ListOptions,
    ) -> Result<Vec<Model>, StoreError> {
        let state = self.inner.lock().expect("store fake poisoned");
        let data = &state.data;
        let rows = data
            .models
            .values()
            .filter(|r| {
                (filter.names.is_empty() || filter.names.contains(&r.name))
                    && filter
                        .model_provider_id
                        .is_none_or(|v| r.model_provider_id == v)
                    && filter.enabled.is_none_or(|v| r.enabled == v)
            })
            .cloned()
            .collect();
        page(rows, options)
    }
}
#[async_trait]
impl ModelStore for Transaction {
    async fn create_model(&self, record: Model) -> Result<Model, StoreError> {
        self.local.create_model(record).await
    }
    async fn get_model(&self, id: Uuid) -> Result<Model, StoreError> {
        self.local.get_model(id).await
    }
    async fn update_model(&self, record: Model) -> Result<Model, StoreError> {
        self.local.update_model(record).await
    }
    async fn delete_model(&self, id: Uuid) -> Result<(), StoreError> {
        self.local.delete_model(id).await
    }
    async fn list_models(
        &self,
        filter: ModelFilter,
        options: ListOptions,
    ) -> Result<Vec<Model>, StoreError> {
        self.local.list_models(filter, options).await
    }
}

impl Row for ModelProvider {
    fn id(&self) -> Uuid {
        self.id
    }
    fn created(&self) -> &Timestamp {
        &self.create_time
    }
    fn updated(&self) -> &Timestamp {
        &self.update_time
    }
    fn name(&self) -> Option<&str> {
        Some(&self.name)
    }
}
#[async_trait]
impl ModelProviderStore for MemoryStore {
    async fn create_model_provider(
        &self,
        record: ModelProvider,
    ) -> Result<ModelProvider, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        if state.data.model_providers.contains_key(&record.id) {
            return Err(constraint("duplicate model_provider id"));
        }
        let mut data = state.data.clone();
        data.model_providers.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn get_model_provider(&self, id: Uuid) -> Result<ModelProvider, StoreError> {
        self.inner
            .lock()
            .expect("store fake poisoned")
            .data
            .model_providers
            .get(&id)
            .cloned()
            .ok_or_else(|| StoreError::NotFound(id.to_string()))
    }
    async fn update_model_provider(
        &self,
        record: ModelProvider,
    ) -> Result<ModelProvider, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let old = state
            .data
            .model_providers
            .get(&record.id)
            .ok_or_else(|| StoreError::NotFound(record.id.to_string()))?;
        if old.create_time != record.create_time {
            return Err(StoreError::Invalid("create_time is immutable".into()));
        }
        let mut data = state.data.clone();
        data.model_providers.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn delete_model_provider(&self, id: Uuid) -> Result<(), StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let mut data = state.data.clone();
        if data.model_providers.remove(&id).is_none() {
            return Err(StoreError::NotFound(id.to_string()));
        }

        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(())
    }
    async fn list_model_providers(
        &self,
        filter: ModelProviderFilter,
        options: ListOptions,
    ) -> Result<Vec<ModelProvider>, StoreError> {
        let state = self.inner.lock().expect("store fake poisoned");
        let data = &state.data;
        let rows = data
            .model_providers
            .values()
            .filter(|r| {
                filter.enabled.is_none_or(|v| r.enabled == v)
                    && (filter.provider_types.is_empty()
                        || filter.provider_types.contains(&r.provider_type))
            })
            .cloned()
            .collect();
        page(rows, options)
    }
}
#[async_trait]
impl ModelProviderStore for Transaction {
    async fn create_model_provider(
        &self,
        record: ModelProvider,
    ) -> Result<ModelProvider, StoreError> {
        self.local.create_model_provider(record).await
    }
    async fn get_model_provider(&self, id: Uuid) -> Result<ModelProvider, StoreError> {
        self.local.get_model_provider(id).await
    }
    async fn update_model_provider(
        &self,
        record: ModelProvider,
    ) -> Result<ModelProvider, StoreError> {
        self.local.update_model_provider(record).await
    }
    async fn delete_model_provider(&self, id: Uuid) -> Result<(), StoreError> {
        self.local.delete_model_provider(id).await
    }
    async fn list_model_providers(
        &self,
        filter: ModelProviderFilter,
        options: ListOptions,
    ) -> Result<Vec<ModelProvider>, StoreError> {
        self.local.list_model_providers(filter, options).await
    }
}

impl Row for Token {
    fn id(&self) -> Uuid {
        self.id
    }
    fn created(&self) -> &Timestamp {
        &self.create_time
    }
    fn updated(&self) -> &Timestamp {
        &self.update_time
    }
    fn name(&self) -> Option<&str> {
        Some(&self.name)
    }
}
#[async_trait]
impl TokenStore for MemoryStore {
    async fn create_token(&self, record: Token) -> Result<Token, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        if state.data.tokens.contains_key(&record.id) {
            return Err(constraint("duplicate token id"));
        }
        let mut data = state.data.clone();
        data.tokens.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn get_token(&self, id: Uuid) -> Result<Token, StoreError> {
        self.inner
            .lock()
            .expect("store fake poisoned")
            .data
            .tokens
            .get(&id)
            .cloned()
            .ok_or_else(|| StoreError::NotFound(id.to_string()))
    }
    async fn update_token(&self, record: Token) -> Result<Token, StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let old = state
            .data
            .tokens
            .get(&record.id)
            .ok_or_else(|| StoreError::NotFound(record.id.to_string()))?;
        if old.create_time != record.create_time {
            return Err(StoreError::Invalid("create_time is immutable".into()));
        }
        let mut data = state.data.clone();
        data.tokens.insert(record.id, record.clone());
        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(record)
    }
    async fn delete_token(&self, id: Uuid) -> Result<(), StoreError> {
        let mut state = self.inner.lock().expect("store fake poisoned");
        let mut data = state.data.clone();
        if data.tokens.remove(&id).is_none() {
            return Err(StoreError::NotFound(id.to_string()));
        }

        data.validate()?;
        state.data = data;
        state.revision += 1;
        Ok(())
    }
    async fn list_tokens(
        &self,
        filter: TokenFilter,
        options: ListOptions,
    ) -> Result<Vec<Token>, StoreError> {
        let state = self.inner.lock().expect("store fake poisoned");
        let data = &state.data;
        let rows = data
            .tokens
            .values()
            .filter(|r| {
                r.name
                    .get(..filter.name_prefix.len())
                    .is_some_and(|prefix| prefix.eq_ignore_ascii_case(&filter.name_prefix))
                    && filter.token_type.as_ref().is_none_or(|v| &r.r#type == v)
                    && filter
                        .expires_after
                        .as_ref()
                        .is_none_or(|v| &r.expires_at > v)
                    && filter
                        .token_hash
                        .as_ref()
                        .is_none_or(|v| &r.token_hash == v)
            })
            .cloned()
            .collect();
        page(rows, options)
    }
}
#[async_trait]
impl TokenStore for Transaction {
    async fn create_token(&self, record: Token) -> Result<Token, StoreError> {
        self.local.create_token(record).await
    }
    async fn get_token(&self, id: Uuid) -> Result<Token, StoreError> {
        self.local.get_token(id).await
    }
    async fn update_token(&self, record: Token) -> Result<Token, StoreError> {
        self.local.update_token(record).await
    }
    async fn delete_token(&self, id: Uuid) -> Result<(), StoreError> {
        self.local.delete_token(id).await
    }
    async fn list_tokens(
        &self,
        filter: TokenFilter,
        options: ListOptions,
    ) -> Result<Vec<Token>, StoreError> {
        self.local.list_tokens(filter, options).await
    }
}
