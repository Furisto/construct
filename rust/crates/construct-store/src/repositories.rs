use crate::{
    codec::{self, Record, Selection, Value},
    SqliteStore,
};
use async_trait::async_trait;
use construct_core::store::*;
use construct_types::entity::*;
use sqlx::SqliteConnection;
use uuid::Uuid;

fn text(value: Option<impl AsRef<str>>) -> Option<Value> {
    value.map(|v| Value::Text(v.as_ref().into()))
}
fn texts(values: Vec<impl AsRef<str>>) -> Vec<Value> {
    values
        .into_iter()
        .map(|v| Value::Text(v.as_ref().into()))
        .collect()
}
fn uuid(value: Option<Uuid>) -> Option<Value> {
    value.map(|v| Value::Text(v.to_string()))
}
fn boolean(value: Option<bool>) -> Option<Value> {
    value.map(|v| Value::Integer(i64::from(v)))
}

async fn list<T: Record>(
    query: Selection,
    connection: &mut SqliteConnection,
    options: ListOptions,
    has_time_bound: bool,
    time_filter: impl FnMut(&T) -> bool + Send,
) -> Result<Vec<T>, StoreError> {
    // Go times may have offsets, variable fractions and monotonic suffixes; text
    // order is not instant order, and SQLite time functions lose nanoseconds.
    // Apply time bounds/order and then pagination after decoding SQL matches.
    let sql_page = options.sort_field == SortField::Name && !has_time_bound;
    let rows = query
        .fetch::<T>(connection, sql_page.then_some(&options))
        .await?;
    if sql_page {
        Ok(rows)
    } else {
        page(rows.into_iter().filter(time_filter).collect(), options)
    }
}

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
                let mut query = Selection::default();
                query.one_of("name", texts(filter.names));
                query.equal("model_id", uuid(filter.model_id));
                query.equal("builtin", boolean(filter.builtin));
                Ok((
                    list::<Agent>(query, connection, options, false, |_| true).await?,
                    None,
                ))
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
                if options.sort_field == SortField::Name {
                    return Err(StoreError::Invalid("entity has no name column".into()));
                }
                let mut query = Selection::default();
                query.equal("agent_id", uuid(filter.agent_id));
                query.equal("phase", text(filter.phase));
                query.equal("desired_phase", text(filter.desired_phase));
                if let Some(prefix) = filter.id_prefix {
                    query.predicate("id LIKE ?", vec![Value::Text(format!("{prefix}%"))]);
                }
                if let Some(has_messages) = filter.has_messages {
                    query.predicate(
                        if has_messages {
                            "EXISTS (SELECT 1 FROM messages WHERE messages.task_id = tasks.id)"
                        } else {
                            "NOT EXISTS (SELECT 1 FROM messages WHERE messages.task_id = tasks.id)"
                        },
                        Vec::new(),
                    );
                }
                Ok((
                    list::<Task>(query, connection, options, false, |_| true).await?,
                    None,
                ))
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
                if options.sort_field == SortField::Name {
                    return Err(StoreError::Invalid("entity has no name column".into()));
                }
                let mut query = Selection::default();
                query.equal("task_id", uuid(filter.task_id));
                query.equal("agent_id", uuid(filter.agent_id));
                query.one_of("source", texts(filter.sources));
                if let Some(processed) = filter.processed {
                    // modernc writes Go's zero time with time.Time.String(); also accept RFC3339/SQLite forms.
                    let unset = "(processed_time IS NULL OR processed_time IN (?, ?, ?, ?, ?, ?))";
                    let predicate = if processed {
                        format!("NOT {unset}")
                    } else {
                        unset.into()
                    };
                    query.predicate(
                        &predicate,
                        texts(vec![
                            "0001-01-01 00:00:00 +0000 UTC",
                            "0001-01-01T00:00:00Z",
                            "0001-01-01 00:00:00",
                            "0001-01-01T00:00:00",
                            "0001-01-01 00:00:00+00:00",
                            "0001-01-01T00:00:00+00:00",
                        ]),
                    );
                }
                let has_time_bound = filter.created_after.is_some();
                Ok((
                    list::<Message>(query, connection, options, has_time_bound, move |r| {
                        filter
                            .created_after
                            .as_ref()
                            .is_none_or(|v| &r.create_time > v)
                    })
                    .await?,
                    None,
                ))
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
                let mut query = Selection::default();
                query.one_of("name", texts(filter.names));
                query.equal("model_provider_id", uuid(filter.model_provider_id));
                query.equal("enabled", boolean(filter.enabled));
                Ok((
                    list::<Model>(query, connection, options, false, |_| true).await?,
                    None,
                ))
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
                let mut query = Selection::default();
                query.equal("enabled", boolean(filter.enabled));
                query.one_of("provider_type", texts(filter.provider_types));
                Ok((
                    list::<ModelProvider>(query, connection, options, false, |_| true).await?,
                    None,
                ))
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
                let mut query = Selection::default();
                let escaped = filter
                    .name_prefix
                    .replace('\\', "\\\\")
                    .replace('%', "\\%")
                    .replace('_', "\\_");
                query.predicate(
                    "name LIKE ? ESCAPE '\\'",
                    vec![Value::Text(format!("{escaped}%"))],
                );
                query.equal("type", text(filter.token_type));
                query.equal("token_hash", filter.token_hash.map(Value::Bytes));
                let has_time_bound = filter.expires_after.is_some();
                Ok((
                    list::<Token>(query, connection, options, has_time_bound, move |r| {
                        filter
                            .expires_after
                            .as_ref()
                            .is_none_or(|v| &r.expires_at > v)
                    })
                    .await?,
                    None,
                ))
            })
        })
        .await
    }
}
