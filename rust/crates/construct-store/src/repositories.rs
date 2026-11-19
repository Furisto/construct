use crate::{
    codec::{self, Record},
    SqliteStore,
};
use async_trait::async_trait;
use construct_core::store::*;
use construct_types::{entity::*, Timestamp};
use uuid::Uuid;

fn page<T: Record>(mut rows: Vec<T>, options: ListOptions) -> Result<Vec<T>, StoreError> {
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
impl AgentStore for SqliteStore {
    async fn create_agent(&self, record: Agent) -> Result<Agent, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, false)))
            .await
    }
    async fn get_agent(&self, id: Uuid) -> Result<Agent, StoreError> {
        self.run(move |c| Box::pin(async move { Ok((codec::get(c, id).await?, None)) }))
            .await
    }
    async fn update_agent(&self, record: Agent) -> Result<Agent, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, true)))
            .await
    }
    async fn delete_agent(&self, id: Uuid) -> Result<(), StoreError> {
        self.run(move |c| Box::pin(codec::delete::<Agent>(c, id)))
            .await
    }
    async fn list_agents(
        &self,
        filter: AgentFilter,
        options: ListOptions,
    ) -> Result<Vec<Agent>, StoreError> {
        self.run(move |connection| {
            Box::pin(async move {
                let records = codec::all::<Agent>(connection).await?;

                let rows = records
                    .iter()
                    .filter(|r| {
                        (filter.names.is_empty() || filter.names.contains(&r.name))
                            && filter.model_id.is_none_or(|v| r.model_id == Some(v))
                            && filter.builtin.is_none_or(|v| r.builtin == v)
                    })
                    .cloned()
                    .collect();
                Ok((page(rows, options)?, None))
            })
        })
        .await
    }
}
#[async_trait]
impl TaskStore for SqliteStore {
    async fn create_task(&self, record: Task) -> Result<Task, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, false)))
            .await
    }
    async fn get_task(&self, id: Uuid) -> Result<Task, StoreError> {
        self.run(move |c| Box::pin(async move { Ok((codec::get(c, id).await?, None)) }))
            .await
    }
    async fn update_task(&self, record: Task) -> Result<Task, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, true)))
            .await
    }
    async fn delete_task(&self, id: Uuid) -> Result<(), StoreError> {
        self.run(move |c| Box::pin(codec::delete::<Task>(c, id)))
            .await
    }
    async fn list_tasks(
        &self,
        filter: TaskFilter,
        options: ListOptions,
    ) -> Result<Vec<Task>, StoreError> {
        self.run(move |connection| {
            Box::pin(async move {
                let records = if let Some(prefix) = &filter.id_prefix {
                    codec::like::<Task>(connection, "id", format!("{prefix}%"), false).await?
                } else {
                    codec::all::<Task>(connection).await?
                };
                let messages = codec::all::<Message>(connection).await?;

                if options.sort_field == SortField::Name {
                    return Err(StoreError::Invalid("entity has no name column".into()));
                }
                let rows = records
                    .iter()
                    .filter(|r| {
                        filter.agent_id.is_none_or(|v| r.agent_id == Some(v))
                            && filter
                                .has_messages
                                .is_none_or(|v| messages.iter().any(|m| m.task_id == r.id) == v)
                            && filter.phase.as_ref().is_none_or(|v| &r.phase == v)
                            && filter
                                .desired_phase
                                .as_ref()
                                .is_none_or(|v| &r.desired_phase == v)
                    })
                    .cloned()
                    .collect();
                Ok((page(rows, options)?, None))
            })
        })
        .await
    }
}
#[async_trait]
impl MessageStore for SqliteStore {
    async fn create_message(&self, record: Message) -> Result<Message, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, false)))
            .await
    }
    async fn get_message(&self, id: Uuid) -> Result<Message, StoreError> {
        self.run(move |c| Box::pin(async move { Ok((codec::get(c, id).await?, None)) }))
            .await
    }
    async fn update_message(&self, record: Message) -> Result<Message, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, true)))
            .await
    }
    async fn delete_message(&self, id: Uuid) -> Result<(), StoreError> {
        self.run(move |c| Box::pin(codec::delete::<Message>(c, id)))
            .await
    }
    async fn list_messages(
        &self,
        filter: MessageFilter,
        options: ListOptions,
    ) -> Result<Vec<Message>, StoreError> {
        self.run(move |connection| {
            Box::pin(async move {
                let records = codec::all::<Message>(connection).await?;

                if options.sort_field == SortField::Name {
                    return Err(StoreError::Invalid("entity has no name column".into()));
                }
                let rows = records
                    .iter()
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
                Ok((page(rows, options)?, None))
            })
        })
        .await
    }
}
#[async_trait]
impl ModelStore for SqliteStore {
    async fn create_model(&self, record: Model) -> Result<Model, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, false)))
            .await
    }
    async fn get_model(&self, id: Uuid) -> Result<Model, StoreError> {
        self.run(move |c| Box::pin(async move { Ok((codec::get(c, id).await?, None)) }))
            .await
    }
    async fn update_model(&self, record: Model) -> Result<Model, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, true)))
            .await
    }
    async fn delete_model(&self, id: Uuid) -> Result<(), StoreError> {
        self.run(move |c| Box::pin(codec::delete::<Model>(c, id)))
            .await
    }
    async fn list_models(
        &self,
        filter: ModelFilter,
        options: ListOptions,
    ) -> Result<Vec<Model>, StoreError> {
        self.run(move |connection| {
            Box::pin(async move {
                let records = codec::all::<Model>(connection).await?;

                let rows = records
                    .iter()
                    .filter(|r| {
                        (filter.names.is_empty() || filter.names.contains(&r.name))
                            && filter
                                .model_provider_id
                                .is_none_or(|v| r.model_provider_id == v)
                            && filter.enabled.is_none_or(|v| r.enabled == v)
                    })
                    .cloned()
                    .collect();
                Ok((page(rows, options)?, None))
            })
        })
        .await
    }
}
#[async_trait]
impl ModelProviderStore for SqliteStore {
    async fn create_model_provider(
        &self,
        record: ModelProvider,
    ) -> Result<ModelProvider, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, false)))
            .await
    }
    async fn get_model_provider(&self, id: Uuid) -> Result<ModelProvider, StoreError> {
        self.run(move |c| Box::pin(async move { Ok((codec::get(c, id).await?, None)) }))
            .await
    }
    async fn update_model_provider(
        &self,
        record: ModelProvider,
    ) -> Result<ModelProvider, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, true)))
            .await
    }
    async fn delete_model_provider(&self, id: Uuid) -> Result<(), StoreError> {
        self.run(move |c| Box::pin(codec::delete::<ModelProvider>(c, id)))
            .await
    }
    async fn list_model_providers(
        &self,
        filter: ModelProviderFilter,
        options: ListOptions,
    ) -> Result<Vec<ModelProvider>, StoreError> {
        self.run(move |connection| {
            Box::pin(async move {
                let records = codec::all::<ModelProvider>(connection).await?;

                let rows = records
                    .iter()
                    .filter(|r| {
                        filter.enabled.is_none_or(|v| r.enabled == v)
                            && (filter.provider_types.is_empty()
                                || filter.provider_types.contains(&r.provider_type))
                    })
                    .cloned()
                    .collect();
                Ok((page(rows, options)?, None))
            })
        })
        .await
    }
}
#[async_trait]
impl TokenStore for SqliteStore {
    async fn create_token(&self, record: Token) -> Result<Token, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, false)))
            .await
    }
    async fn get_token(&self, id: Uuid) -> Result<Token, StoreError> {
        self.run(move |c| Box::pin(async move { Ok((codec::get(c, id).await?, None)) }))
            .await
    }
    async fn update_token(&self, record: Token) -> Result<Token, StoreError> {
        self.run(move |c| Box::pin(codec::write(c, record, true)))
            .await
    }
    async fn delete_token(&self, id: Uuid) -> Result<(), StoreError> {
        self.run(move |c| Box::pin(codec::delete::<Token>(c, id)))
            .await
    }
    async fn list_tokens(
        &self,
        filter: TokenFilter,
        options: ListOptions,
    ) -> Result<Vec<Token>, StoreError> {
        self.run(move |connection| {
            Box::pin(async move {
                let escaped = filter
                    .name_prefix
                    .replace('\\', "\\\\")
                    .replace('%', "\\%")
                    .replace('_', "\\_");
                let records =
                    codec::like::<Token>(connection, "name", format!("{escaped}%"), true).await?;

                let rows = records
                    .iter()
                    .filter(|r| {
                        filter.token_type.as_ref().is_none_or(|v| &r.r#type == v)
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
                Ok((page(rows, options)?, None))
            })
        })
        .await
    }
}
