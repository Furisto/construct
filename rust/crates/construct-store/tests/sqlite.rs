use construct_core::store::*;
use construct_store::{EntityKind, HookRecord, MutationHook, SqliteStore};
use construct_types::{entity::*, memory::*, Timestamp};
use std::sync::{Arc, Mutex};
use uuid::Uuid;

fn id(n: u128) -> Uuid {
    Uuid::from_u128(n)
}
fn agent(n: u128) -> Agent {
    Agent {
        id: id(n),
        name: format!("a{n}"),
        ..Default::default()
    }
}
fn task(n: u128) -> Task {
    Task {
        id: id(n),
        phase: "awaiting".into(),
        desired_phase: "running".into(),
        ..Default::default()
    }
}
fn kind(r: &HookRecord) -> EntityKind {
    match r {
        HookRecord::Agent(_) => EntityKind::Agent,
        HookRecord::Model(_) => EntityKind::Model,
        HookRecord::ModelProvider(_) => EntityKind::ModelProvider,
        HookRecord::Task(_) => EntityKind::Task,
    }
}
#[derive(Default)]
struct Hooks(Mutex<Vec<(char, EntityKind)>>);
impl MutationHook for Hooks {
    fn on_create(&self, r: &HookRecord) {
        self.0.lock().unwrap().push(('c', kind(r)));
    }
    fn on_update(&self, r: &HookRecord) {
        self.0.lock().unwrap().push(('u', kind(r)));
    }
    fn on_delete(&self, k: EntityKind, _: Uuid) {
        self.0.lock().unwrap().push(('d', k));
    }
}

#[tokio::test]
async fn transactions_isolation_rollback_drop_cancel_conflict_and_hooks() {
    let directory = tempfile::tempdir().unwrap();
    let path = directory.path().join("store.db");
    let hooks = Arc::new(Hooks::default());
    let store = Arc::new(
        SqliteStore::open_with_hooks(&path, vec![hooks.clone()])
            .await
            .unwrap(),
    );
    let tx = store.begin().await.unwrap();
    assert!(matches!(tx.begin().await, Err(StoreError::Invalid(_))));
    tx.create_agent(agent(1)).await.unwrap();
    assert!(hooks.0.lock().unwrap().is_empty());
    assert!(matches!(
        store.get_agent(id(1)).await,
        Err(StoreError::NotFound(_))
    ));
    tx.commit().await.unwrap();
    assert_eq!(*hooks.0.lock().unwrap(), [('c', EntityKind::Agent)]);
    store.update_agent(agent(1)).await.unwrap();
    assert!(store.create_agent(agent(1)).await.is_err());
    let tx = store.begin().await.unwrap();
    tx.delete_agent(id(1)).await.unwrap();
    tx.rollback().await.unwrap();
    assert!(store.get_agent(id(1)).await.is_ok());
    let tx = store.begin().await.unwrap();
    tx.create_agent(agent(2)).await.unwrap();
    drop(tx);
    assert!(store.get_agent(id(2)).await.is_err());
    let (ready, started) = tokio::sync::oneshot::channel();
    let child = store.clone();
    let job = tokio::spawn(async move {
        let tx = child.begin().await.unwrap();
        tx.create_agent(agent(3)).await.unwrap();
        ready.send(()).unwrap();
        std::future::pending::<()>().await;
        drop(tx);
    });
    started.await.unwrap();
    job.abort();
    assert!(job.await.unwrap_err().is_cancelled());
    assert!(store.get_agent(id(3)).await.is_err());
    // A reader whose WAL snapshot is invalidated cannot upgrade to a writer.
    let tx = store.begin().await.unwrap();
    tx.get_agent(id(1)).await.unwrap();
    store.create_agent(agent(4)).await.unwrap();
    assert!(matches!(
        tx.create_agent(agent(5)).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    tx.rollback().await.unwrap();
    assert!(store.get_agent(id(5)).await.is_err());
    store.create_task(task(10)).await.unwrap();
    store.update_task(task(10)).await.unwrap();
    store
        .create_message(Message {
            id: id(11),
            task_id: id(10),
            source: "user".into(),
            ..Default::default()
        })
        .await
        .unwrap();
    store
        .create_token(Token {
            id: id(12),
            name: "token".into(),
            r#type: "api_token".into(),
            ..Default::default()
        })
        .await
        .unwrap();
    store.delete_task(id(10)).await.unwrap();
    store.delete_token(id(12)).await.unwrap();
    store.delete_agent(id(1)).await.unwrap();
    assert_eq!(
        *hooks.0.lock().unwrap(),
        [
            ('c', EntityKind::Agent),
            ('u', EntityKind::Agent),
            ('c', EntityKind::Agent),
            ('c', EntityKind::Task),
            ('d', EntityKind::Task),
            ('d', EntityKind::Agent)
        ]
    );
    drop(store);
    let reopened = SqliteStore::open(path).await.unwrap();
    assert!(reopened.get_agent(id(4)).await.is_ok());
    assert!(reopened.get_agent(id(5)).await.is_err());
}

#[tokio::test]
async fn foreign_key_actions_checks_validation_and_sanitized_errors() {
    let directory = tempfile::tempdir().unwrap();
    let hooks = Arc::new(Hooks::default());
    let store =
        SqliteStore::open_with_hooks(directory.path().join("store.db"), vec![hooks.clone()])
            .await
            .unwrap();
    store
        .create_model_provider(ModelProvider {
            id: id(1),
            name: "p".into(),
            secret: vec![1],
            provider_type: "anthropic".into(),
            ..Default::default()
        })
        .await
        .unwrap();
    store
        .create_model(Model {
            id: id(2),
            model_provider_id: id(1),
            ..Default::default()
        })
        .await
        .unwrap();
    let p = store.get_model_provider(id(1)).await.unwrap();
    store.update_model_provider(p).await.unwrap();
    let model = store.get_model(id(2)).await.unwrap();
    store.update_model(model).await.unwrap();
    let mut a = agent(3);
    a.model_id = Some(id(2));
    store.create_agent(a).await.unwrap();
    let mut t = task(4);
    t.agent_id = Some(id(3));
    store.create_task(t).await.unwrap();
    let mut m = Message {
        id: id(5),
        task_id: id(4),
        agent_id: Some(id(3)),
        source: "assistant".into(),
        ..Default::default()
    };
    assert!(matches!(
        store.create_message(m.clone()).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    m.model_id = Some(id(2));
    store.create_message(m).await.unwrap();
    assert!(matches!(
        store.delete_model(id(2)).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    assert!(matches!(
        store.delete_model_provider(id(1)).await,
        Err(StoreError::ConstraintViolation(_))
    ));
    store.delete_agent(id(3)).await.unwrap();
    assert!(store.get_task(id(4)).await.unwrap().agent_id.is_none());
    assert!(store.get_message(id(5)).await.unwrap().agent_id.is_none());
    store.delete_model(id(2)).await.unwrap();
    assert!(store.get_message(id(5)).await.unwrap().model_id.is_none());
    store.delete_model_provider(id(1)).await.unwrap();
    store.delete_task(id(4)).await.unwrap();
    assert!(matches!(
        store.get_message(id(5)).await,
        Err(StoreError::NotFound(_))
    ));
    let error = store
        .create_model_provider(ModelProvider {
            id: id(9),
            name: "SECRET_SENTINEL".into(),
            secret: b"SECRET_SENTINEL".to_vec(),
            provider_type: "invalid".into(),
            ..Default::default()
        })
        .await
        .unwrap_err();
    assert!(!error.to_string().contains("SECRET_SENTINEL"));
    assert!(matches!(error, StoreError::ConstraintViolation(_)));
    assert!(matches!(
        store
            .create_task(Task {
                cost: f64::NAN,
                ..task(20)
            })
            .await,
        Err(StoreError::Invalid(_))
    ));
    assert_eq!(
        *hooks.0.lock().unwrap(),
        [
            ('c', EntityKind::ModelProvider),
            ('c', EntityKind::Model),
            ('u', EntityKind::ModelProvider),
            ('u', EntityKind::Model),
            ('c', EntityKind::Agent),
            ('c', EntityKind::Task),
            ('d', EntityKind::Agent),
            ('d', EntityKind::Model),
            ('d', EntityKind::ModelProvider),
            ('d', EntityKind::Task)
        ]
    );
}

#[tokio::test]
async fn ordering_pagination_zero_processed_json_null_and_empty() {
    let directory = tempfile::tempdir().unwrap();
    let store = SqliteStore::open(directory.path().join("store.db"))
        .await
        .unwrap();
    for n in [3, 1, 2] {
        store.create_agent(agent(n)).await.unwrap();
    }
    for (sort_order, expected) in [
        (SortOrder::Ascending, vec![id(1), id(2), id(3)]),
        (SortOrder::Descending, vec![id(3), id(2), id(1)]),
    ] {
        for sort_field in [SortField::CreatedAt, SortField::UpdatedAt] {
            let rows = store
                .list_agents(
                    AgentFilter::default(),
                    ListOptions {
                        sort_field,
                        sort_order,
                        ..Default::default()
                    },
                )
                .await
                .unwrap();
            assert_eq!(rows.iter().map(|r| r.id).collect::<Vec<_>>(), expected);
        }
    }
    assert!(store
        .list_agents(
            AgentFilter::default(),
            ListOptions {
                offset: usize::MAX,
                ..Default::default()
            }
        )
        .await
        .unwrap()
        .is_empty());
    store.create_task(task(4)).await.unwrap();
    let mut m = Message {
        id: id(5),
        task_id: id(4),
        source: "system".into(),
        processed_time: Some(Timestamp::default()),
        ..Default::default()
    };
    store.create_message(m.clone()).await.unwrap();
    assert!(store.get_message(id(5)).await.unwrap().content.is_none());
    assert_eq!(
        store
            .list_messages(
                MessageFilter {
                    processed: Some(false),
                    ..Default::default()
                },
                ListOptions::default()
            )
            .await
            .unwrap()
            .len(),
        1
    );
    m.content = Some(MessageContent {
        blocks: Some(vec![]),
    });
    m.usage = Some(MessageUsage::default());
    m.processed_time = Some(serde_json::from_str("\"2025-01-01T00:00:00Z\"").unwrap());
    store.update_message(m).await.unwrap();
    let saved = store.get_message(id(5)).await.unwrap();
    assert!(saved.content.unwrap().blocks.unwrap().is_empty());
    assert!(saved.usage.is_some());
    assert_eq!(
        store
            .list_messages(
                MessageFilter {
                    processed: Some(true),
                    ..Default::default()
                },
                ListOptions::default()
            )
            .await
            .unwrap()
            .len(),
        1
    );
    store.delete_task(id(4)).await.unwrap();
    assert!(matches!(
        store
            .list_messages(
                MessageFilter::default(),
                ListOptions {
                    sort_field: SortField::Name,
                    ..Default::default()
                }
            )
            .await,
        Err(StoreError::Invalid(_))
    ));
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
}
