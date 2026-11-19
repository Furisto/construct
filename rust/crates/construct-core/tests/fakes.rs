#![cfg(feature = "testing")]
use construct_core::model::ModelProvider as _;
use construct_core::{model::InvokeOptions, secret::*, store::*, testing::*, CancellationToken};
use construct_types::{
    entity::*,
    memory::*,
    provider::{Message as ProviderMessage, ProviderError, ProviderErrorKind},
    Timestamp,
};
use std::{
    sync::{Arc, Mutex},
    time::Duration,
};
use uuid::Uuid;

fn id(n: u128) -> Uuid {
    Uuid::from_u128(n)
}
fn time(value: &str) -> Timestamp {
    serde_json::from_value(serde_json::json!(value)).unwrap()
}
fn timestamp() -> Timestamp {
    time("2025-01-01T00:00:00Z")
}
async fn seeded() -> Arc<dyn Store> {
    let store: Arc<dyn Store> = Arc::new(MemoryStore::default());
    store
        .create_model_provider(ModelProvider {
            id: id(1),
            name: "p".into(),
            provider_type: "anthropic".into(),
            secret: vec![1],
            enabled: true,
            create_time: timestamp(),
            ..Default::default()
        })
        .await
        .unwrap();
    store
        .create_model(Model {
            id: id(2),
            model_provider_id: id(1),
            name: "m".into(),
            enabled: true,
            create_time: timestamp(),
            ..Default::default()
        })
        .await
        .unwrap();
    store
        .create_agent(Agent {
            id: id(3),
            name: "a".into(),
            model_id: Some(id(2)),
            create_time: timestamp(),
            ..Default::default()
        })
        .await
        .unwrap();
    store
        .create_task(Task {
            id: id(4),
            agent_id: Some(id(3)),
            phase: "awaiting".into(),
            desired_phase: "running".into(),
            create_time: timestamp(),
            ..Default::default()
        })
        .await
        .unwrap();
    store
        .create_message(Message {
            id: id(5),
            task_id: id(4),
            agent_id: Some(id(3)),
            model_id: Some(id(2)),
            source: "user".into(),
            create_time: timestamp(),
            ..Default::default()
        })
        .await
        .unwrap();
    store
        .create_token(Token {
            id: id(6),
            name: "token".into(),
            r#type: "api_token".into(),
            token_hash: vec![6],
            expires_at: timestamp(),
            create_time: timestamp(),
            ..Default::default()
        })
        .await
        .unwrap();
    store
}

#[tokio::test]
async fn secret_fake_success_empty_overwrite_and_missing() {
    let secrets: Arc<dyn SecretProvider> = Arc::new(MemorySecretProvider::default());
    assert!(matches!(
        secrets.get("missing").await,
        Err(SecretError::NotFound { .. })
    ));
    secrets.delete("missing").await.unwrap();
    secrets.set("key", "").await.unwrap();
    assert_eq!(secrets.get("key").await.unwrap(), "");
    secrets.set("key", "replacement").await.unwrap();
    assert_eq!(secrets.get("key").await.unwrap(), "replacement");
    secrets.delete("key").await.unwrap();
    assert!(matches!(
        secrets.get("key").await,
        Err(SecretError::NotFound { .. })
    ));
}

#[tokio::test]
async fn filters_pagination_and_schema_constraints() {
    let store = seeded().await;
    assert_eq!(
        store
            .list_agents(
                AgentFilter {
                    names: vec!["a".into()],
                    model_id: Some(id(2)),
                    builtin: Some(false)
                },
                ListOptions::default()
            )
            .await
            .unwrap()
            .len(),
        1
    );
    assert_eq!(
        store
            .list_models(
                ModelFilter {
                    names: vec!["m".into()],
                    model_provider_id: Some(id(1)),
                    enabled: Some(true)
                },
                ListOptions::default()
            )
            .await
            .unwrap()
            .len(),
        1
    );
    assert_eq!(
        store
            .list_model_providers(
                ModelProviderFilter {
                    provider_types: vec!["anthropic".into()],
                    enabled: Some(true)
                },
                ListOptions::default()
            )
            .await
            .unwrap()
            .len(),
        1
    );
    assert_eq!(
        store
            .list_tasks(
                TaskFilter {
                    agent_id: Some(id(3)),
                    id_prefix: Some(id(4).to_string()),
                    has_messages: Some(true),
                    phase: Some("awaiting".into()),
                    desired_phase: Some("running".into())
                },
                ListOptions::default()
            )
            .await
            .unwrap()
            .len(),
        1
    );
    assert_eq!(
        store
            .list_messages(
                MessageFilter {
                    task_id: Some(id(4)),
                    agent_id: Some(id(3)),
                    sources: vec!["user".into()],
                    processed: Some(false),
                    created_after: Some(time("2024-01-01T00:00:00Z"))
                },
                ListOptions::default()
            )
            .await
            .unwrap()
            .len(),
        1
    );
    assert_eq!(
        store
            .list_tokens(
                TokenFilter {
                    name_prefix: "tok".into(),
                    token_type: Some("api_token".into()),
                    token_hash: Some(vec![6]),
                    expires_after: Some(time("2024-01-01T00:00:00Z"))
                },
                ListOptions::default()
            )
            .await
            .unwrap()
            .len(),
        1
    );
    assert!(store
        .list_tokens(
            TokenFilter {
                expires_after: Some(timestamp()),
                ..Default::default()
            },
            ListOptions::default()
        )
        .await
        .unwrap()
        .is_empty());
    assert!(store
        .list_messages(
            MessageFilter {
                created_after: Some(timestamp()),
                ..Default::default()
            },
            ListOptions::default()
        )
        .await
        .unwrap()
        .is_empty());
    assert!(store
        .list_tasks(
            TaskFilter {
                has_messages: Some(false),
                ..Default::default()
            },
            ListOptions::default()
        )
        .await
        .unwrap()
        .is_empty());
    assert!(matches!(
        store
            .list_tasks(
                TaskFilter::default(),
                ListOptions {
                    sort_field: SortField::Name,
                    ..Default::default()
                }
            )
            .await,
        Err(StoreError::Invalid(_))
    ));
    for result in [
        store.delete_agent(id(3)).await,
        store.delete_model(id(2)).await,
        store.delete_model_provider(id(1)).await,
    ] {
        assert!(matches!(result, Err(StoreError::ConstraintViolation(_))));
    }
    let bad = Message {
        id: id(50),
        task_id: id(4),
        agent_id: Some(id(3)),
        ..Default::default()
    };
    assert!(matches!(
        store.create_message(bad).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    let mut duplicate = store.get_agent(id(3)).await.unwrap();
    duplicate.id = id(30);
    assert!(matches!(
        store.create_agent(duplicate).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    let mut a = store.get_agent(id(3)).await.unwrap();
    a.id = id(30);
    a.name = "z".into();
    store.create_agent(a).await.unwrap();
    let rows = store
        .list_agents(
            AgentFilter::default(),
            ListOptions {
                sort_field: SortField::Name,
                sort_order: SortOrder::Ascending,
                offset: 1,
                limit: Some(1),
            },
        )
        .await
        .unwrap();
    assert_eq!(rows[0].name, "z");
    assert!(store
        .list_agents(
            AgentFilter::default(),
            ListOptions {
                limit: Some(0),
                ..Default::default()
            }
        )
        .await
        .unwrap()
        .is_empty());
    store.delete_task(id(4)).await.unwrap();
    assert!(matches!(
        store.get_message(id(5)).await,
        Err(StoreError::NotFound(_))
    ));
}

#[tokio::test]
async fn task_uuid_prefix_matches_go_sqlite_like() {
    let store = seeded().await;
    store
        .create_task(Task {
            id: Uuid::parse_str("abcdefab-1234-5678-9abc-123456789abc").unwrap(),
            ..Default::default()
        })
        .await
        .unwrap();
    for (prefix, count) in [
        ("ABCDEF", 1),
        ("abcdefab-1234", 1),
        ("abc_ef%", 1),
        ("%", 2),
        ("bad", 0),
    ] {
        let tasks = store
            .list_tasks(
                TaskFilter {
                    id_prefix: Some(prefix.into()),
                    ..Default::default()
                },
                ListOptions::default(),
            )
            .await
            .unwrap();
        assert_eq!(tasks.len(), count, "{prefix}");
    }
}

#[tokio::test]
async fn token_name_prefix_matches_go_sqlite_literal_like() {
    let store = seeded().await;
    store
        .create_token(Token {
            id: id(60),
            name: "token%".into(),
            token_hash: vec![60],
            ..Default::default()
        })
        .await
        .unwrap();
    for (prefix, count) in [("TO", 2), ("token%", 1), ("_", 0)] {
        assert_eq!(
            store
                .list_tokens(
                    TokenFilter {
                        name_prefix: prefix.into(),
                        ..Default::default()
                    },
                    ListOptions::default()
                )
                .await
                .unwrap()
                .len(),
            count
        );
    }
}

#[tokio::test]
async fn transactions_commit_rollback_drop_isolation_and_conflicts() {
    let store = seeded().await;
    let make = |n| Agent {
        id: id(n),
        name: format!("agent-{n}"),
        ..Default::default()
    };
    let tx = store.begin().await.unwrap();
    assert!(matches!(tx.begin().await, Err(StoreError::Invalid(_))));
    tx.create_agent(make(10)).await.unwrap();
    assert!(matches!(
        store.get_agent(id(10)).await,
        Err(StoreError::NotFound(_))
    ));
    tx.create_task(Task {
        id: id(11),
        agent_id: Some(id(10)),
        ..Default::default()
    })
    .await
    .unwrap();
    tx.commit().await.unwrap();
    assert_eq!(store.get_task(id(11)).await.unwrap().agent_id, Some(id(10)));
    let tx = store.begin().await.unwrap();
    tx.create_agent(make(12)).await.unwrap();
    tx.rollback().await.unwrap();
    assert!(store.get_agent(id(12)).await.is_err());
    let tx = store.begin().await.unwrap();
    tx.create_agent(make(13)).await.unwrap();
    drop(tx);
    assert!(store.get_agent(id(13)).await.is_err());
    let tx = store.begin().await.unwrap();
    tx.create_agent(make(14)).await.unwrap();
    store.create_agent(make(15)).await.unwrap();
    assert!(matches!(
        tx.commit().await,
        Err(StoreError::ConstraintViolation(_))
    ));
    assert!(store.get_agent(id(14)).await.is_err());
    assert!(store.get_agent(id(15)).await.is_ok());
    let (ready, started) = tokio::sync::oneshot::channel();
    let child = store.clone();
    let task = tokio::spawn(async move {
        let tx = child.begin().await.unwrap();
        tx.create_agent(Agent {
            id: id(16),
            name: "cancelled".into(),
            ..Default::default()
        })
        .await
        .unwrap();
        ready.send(()).unwrap();
        std::future::pending::<()>().await;
        drop(tx);
    });
    started.await.unwrap();
    task.abort();
    assert!(task.await.unwrap_err().is_cancelled());
    assert!(store.get_agent(id(16)).await.is_err());
}

fn step() -> ModelStep {
    ModelStep {
        expected_model: Some("model".into()),
        chunks: vec!["a".into(), "b".into()],
        retries: vec![],
        result: Ok(ProviderMessage::default()),
    }
}

#[tokio::test]
async fn agent_fake_crud_empty_missing_and_immutable() {
    let store = seeded().await;
    let mut record = store.get_agent(id(3)).await.unwrap();
    record.id = id(100);
    record.name = "new-agent".into();
    let saved = store.create_agent(record.clone()).await.unwrap();
    assert_eq!(saved.id, id(100));
    assert!(matches!(
        store.create_agent(record.clone()).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    assert_eq!(
        store
            .list_agents(AgentFilter::default(), ListOptions::default())
            .await
            .unwrap()
            .len(),
        2
    );
    assert!(store
        .list_agents(
            AgentFilter {
                names: vec!["absent".into()],
                ..Default::default()
            },
            ListOptions::default()
        )
        .await
        .unwrap()
        .is_empty());
    record.description = "changed".into();
    store.update_agent(record.clone()).await.unwrap();
    assert_eq!(
        store.get_agent(id(100)).await.unwrap().description,
        "changed"
    );
    record.create_time = Timestamp::default();
    assert!(matches!(
        store.update_agent(record.clone()).await,
        Err(StoreError::Invalid(_))
    ));
    store.delete_agent(id(100)).await.unwrap();
    assert!(matches!(
        store.get_agent(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.update_agent(record).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.delete_agent(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
}

#[tokio::test]
async fn task_fake_crud_empty_missing_and_immutable() {
    let store = seeded().await;
    let mut record = store.get_task(id(4)).await.unwrap();
    record.id = id(100);
    let saved = store.create_task(record.clone()).await.unwrap();
    assert_eq!(saved.id, id(100));
    assert!(matches!(
        store.create_task(record.clone()).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    assert_eq!(
        store
            .list_tasks(TaskFilter::default(), ListOptions::default())
            .await
            .unwrap()
            .len(),
        2
    );
    assert!(store
        .list_tasks(
            TaskFilter {
                agent_id: Some(id(999)),
                ..Default::default()
            },
            ListOptions::default()
        )
        .await
        .unwrap()
        .is_empty());
    record.description = "changed".into();
    store.update_task(record.clone()).await.unwrap();
    assert_eq!(
        store.get_task(id(100)).await.unwrap().description,
        "changed"
    );
    record.create_time = Timestamp::default();
    assert!(matches!(
        store.update_task(record.clone()).await,
        Err(StoreError::Invalid(_))
    ));
    store.delete_task(id(100)).await.unwrap();
    assert!(matches!(
        store.get_task(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.update_task(record).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.delete_task(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
}

#[tokio::test]
async fn message_fake_crud_empty_missing_and_immutable() {
    let store = seeded().await;
    let mut record = store.get_message(id(5)).await.unwrap();
    record.id = id(100);
    let saved = store.create_message(record.clone()).await.unwrap();
    assert_eq!(saved.id, id(100));
    assert!(matches!(
        store.create_message(record.clone()).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    assert_eq!(
        store
            .list_messages(MessageFilter::default(), ListOptions::default())
            .await
            .unwrap()
            .len(),
        2
    );
    assert!(store
        .list_messages(
            MessageFilter {
                task_id: Some(id(999)),
                ..Default::default()
            },
            ListOptions::default()
        )
        .await
        .unwrap()
        .is_empty());
    record.content = Some(MessageContent {
        blocks: Some(vec![]),
    });
    store.update_message(record.clone()).await.unwrap();
    assert!(store
        .get_message(id(100))
        .await
        .unwrap()
        .content
        .unwrap()
        .blocks
        .unwrap()
        .is_empty());
    record.create_time = Timestamp::default();
    assert!(matches!(
        store.update_message(record.clone()).await,
        Err(StoreError::Invalid(_))
    ));
    store.delete_message(id(100)).await.unwrap();
    assert!(matches!(
        store.get_message(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.update_message(record).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.delete_message(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
}

#[tokio::test]
async fn model_fake_crud_empty_missing_and_immutable() {
    let store = seeded().await;
    let mut record = store.get_model(id(2)).await.unwrap();
    record.id = id(100);
    record.name = "new-model".into();
    let saved = store.create_model(record.clone()).await.unwrap();
    assert_eq!(saved.id, id(100));
    assert!(matches!(
        store.create_model(record.clone()).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    assert_eq!(
        store
            .list_models(ModelFilter::default(), ListOptions::default())
            .await
            .unwrap()
            .len(),
        2
    );
    assert!(store
        .list_models(
            ModelFilter {
                names: vec!["absent".into()],
                ..Default::default()
            },
            ListOptions::default()
        )
        .await
        .unwrap()
        .is_empty());
    record.enabled = false;
    store.update_model(record.clone()).await.unwrap();
    assert!(!store.get_model(id(100)).await.unwrap().enabled);
    record.create_time = Timestamp::default();
    assert!(matches!(
        store.update_model(record.clone()).await,
        Err(StoreError::Invalid(_))
    ));
    store.delete_model(id(100)).await.unwrap();
    assert!(matches!(
        store.get_model(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.update_model(record).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.delete_model(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
}

#[tokio::test]
async fn model_provider_fake_crud_empty_missing_and_immutable() {
    let store = seeded().await;
    let mut record = store.get_model_provider(id(1)).await.unwrap();
    record.id = id(100);
    record.name = "new-provider".into();
    let saved = store.create_model_provider(record.clone()).await.unwrap();
    assert_eq!(saved.id, id(100));
    assert!(matches!(
        store.create_model_provider(record.clone()).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    assert_eq!(
        store
            .list_model_providers(ModelProviderFilter::default(), ListOptions::default())
            .await
            .unwrap()
            .len(),
        2
    );
    assert!(store
        .list_model_providers(
            ModelProviderFilter {
                provider_types: vec!["missing".into()],
                ..Default::default()
            },
            ListOptions::default()
        )
        .await
        .unwrap()
        .is_empty());
    record.url = "http://localhost".into();
    store.update_model_provider(record.clone()).await.unwrap();
    assert_eq!(
        store.get_model_provider(id(100)).await.unwrap().url,
        "http://localhost"
    );
    record.create_time = Timestamp::default();
    assert!(matches!(
        store.update_model_provider(record.clone()).await,
        Err(StoreError::Invalid(_))
    ));
    store.delete_model_provider(id(100)).await.unwrap();
    assert!(matches!(
        store.get_model_provider(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.update_model_provider(record).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.delete_model_provider(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
}

#[tokio::test]
async fn token_fake_crud_empty_missing_and_immutable() {
    let store = seeded().await;
    let mut record = store.get_token(id(6)).await.unwrap();
    record.id = id(100);
    record.name = "new-token".into();
    record.token_hash = vec![42];
    let saved = store.create_token(record.clone()).await.unwrap();
    assert_eq!(saved.id, id(100));
    assert!(matches!(
        store.create_token(record.clone()).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    assert_eq!(
        store
            .list_tokens(TokenFilter::default(), ListOptions::default())
            .await
            .unwrap()
            .len(),
        2
    );
    assert!(store
        .list_tokens(
            TokenFilter {
                name_prefix: "absent".into(),
                ..Default::default()
            },
            ListOptions::default()
        )
        .await
        .unwrap()
        .is_empty());
    record.description = "changed".into();
    store.update_token(record.clone()).await.unwrap();
    assert_eq!(
        store.get_token(id(100)).await.unwrap().description,
        "changed"
    );
    record.create_time = Timestamp::default();
    assert!(matches!(
        store.update_token(record.clone()).await,
        Err(StoreError::Invalid(_))
    ));
    store.delete_token(id(100)).await.unwrap();
    assert!(matches!(
        store.get_token(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.update_token(record).await,
        Err(StoreError::NotFound(_))
    ));
    assert!(matches!(
        store.delete_token(id(100)).await,
        Err(StoreError::NotFound(_))
    ));
}

fn provider_error() -> ProviderError {
    ProviderError {
        provider: "fake".into(),
        kind: ProviderErrorKind::OVERLOADED.into(),
        retry_after: 0,
        err: None,
    }
}

#[tokio::test]
async fn scripted_provider_success_errors_cancellation_and_callbacks() {
    let mut first = step();
    first.retries.push(RetryNotice {
        error: provider_error(),
        delay: Duration::from_secs(20),
    });
    let provider = Arc::new(ScriptedModelProvider::new([
        first,
        ModelStep {
            result: Err(provider_error()),
            ..step()
        },
        step(),
    ]));
    let text = Arc::new(Mutex::new(String::new()));
    let retry = Arc::new(Mutex::new(Vec::new()));
    let (sink, retries, observed) = (text.clone(), retry.clone(), provider.clone());
    let options = InvokeOptions {
        text_sink: Some(Arc::new(move |chunk| sink.lock().unwrap().push_str(chunk))),
        retry_callback: Some(Arc::new(move |error, delay| {
            let _ = observed.calls();
            retries.lock().unwrap().push((error.kind.clone(), delay));
        })),
        ..Default::default()
    };
    provider
        .invoke_model("model", "system", &[], options)
        .await
        .unwrap();
    assert_eq!(*text.lock().unwrap(), "ab");
    assert_eq!(retry.lock().unwrap().len(), 1);
    assert_eq!(provider.calls()[0].system_prompt, "system");
    assert_eq!(
        provider
            .invoke_model("model", "", &[], InvokeOptions::default())
            .await
            .unwrap_err()
            .kind
            .as_ref(),
        "overloaded"
    );
    let cancel = CancellationToken::new();
    cancel.cancel();
    assert_eq!(
        provider
            .invoke_model(
                "model",
                "",
                &[],
                InvokeOptions {
                    cancellation: cancel,
                    ..Default::default()
                }
            )
            .await
            .unwrap_err()
            .kind
            .as_ref(),
        "canceled"
    );
    assert_eq!(provider.remaining(), 1);
    assert_eq!(
        provider
            .invoke_model("wrong", "", &[], InvokeOptions::default())
            .await
            .unwrap_err()
            .kind
            .as_ref(),
        "invalid_request"
    );
    assert_eq!(
        provider
            .invoke_model("model", "", &[], InvokeOptions::default())
            .await
            .unwrap_err()
            .kind
            .as_ref(),
        "internal"
    );
    let provider = ScriptedModelProvider::new([step()]);
    let cancel = CancellationToken::new();
    let callback = cancel.clone();
    assert_eq!(
        provider
            .invoke_model(
                "model",
                "",
                &[],
                InvokeOptions {
                    cancellation: cancel,
                    text_sink: Some(Arc::new(move |_| callback.cancel())),
                    ..Default::default()
                }
            )
            .await
            .unwrap_err()
            .kind
            .as_ref(),
        "canceled"
    );
}
