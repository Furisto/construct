use super::*;
use construct_core::testing::MemoryStore;
use construct_proto::construct::v1::EventService;
use construct_secret::Keyset;
use futures_util::StreamExt;
use std::sync::Mutex;

struct ReplayStore {
    inner: MemoryStore,
    events: Arc<EventRouter>,
    arrivals: Mutex<Vec<db::Message>>,
    fail: bool,
}
macro_rules! delegate {
    ($trait:ident, $entity:ident, $filter:ident, $create:ident, $get:ident, $update:ident, $delete:ident, $list:ident) => {
        #[async_trait]
        impl $trait for ReplayStore {
            async fn $create(&self, value: db::$entity) -> Result<db::$entity, StoreError> {
                self.inner.$create(value).await
            }
            async fn $get(&self, id: Uuid) -> Result<db::$entity, StoreError> {
                self.inner.$get(id).await
            }
            async fn $update(&self, value: db::$entity) -> Result<db::$entity, StoreError> {
                self.inner.$update(value).await
            }
            async fn $delete(&self, id: Uuid) -> Result<(), StoreError> {
                self.inner.$delete(id).await
            }
            async fn $list(
                &self,
                filter: $filter,
                opts: ListOptions,
            ) -> Result<Vec<db::$entity>, StoreError> {
                self.inner.$list(filter, opts).await
            }
        }
    };
}
delegate!(
    AgentStore,
    Agent,
    AgentFilter,
    create_agent,
    get_agent,
    update_agent,
    delete_agent,
    list_agents
);
delegate!(
    ModelStore,
    Model,
    ModelFilter,
    create_model,
    get_model,
    update_model,
    delete_model,
    list_models
);
delegate!(
    ModelProviderStore,
    ModelProvider,
    ModelProviderFilter,
    create_model_provider,
    get_model_provider,
    update_model_provider,
    delete_model_provider,
    list_model_providers
);
delegate!(
    TaskStore,
    Task,
    TaskFilter,
    create_task,
    get_task,
    update_task,
    delete_task,
    list_tasks
);
delegate!(
    TokenStore,
    Token,
    TokenFilter,
    create_token,
    get_token,
    update_token,
    delete_token,
    list_tokens
);
#[async_trait]
impl MessageStore for ReplayStore {
    async fn create_message(&self, m: db::Message) -> Result<db::Message, StoreError> {
        self.inner.create_message(m).await
    }
    async fn get_message(&self, id: Uuid) -> Result<db::Message, StoreError> {
        self.inner.get_message(id).await
    }
    async fn update_message(&self, m: db::Message) -> Result<db::Message, StoreError> {
        self.inner.update_message(m).await
    }
    async fn delete_message(&self, id: Uuid) -> Result<(), StoreError> {
        self.inner.delete_message(id).await
    }
    async fn list_messages(
        &self,
        filter: MessageFilter,
        opts: ListOptions,
    ) -> Result<Vec<db::Message>, StoreError> {
        assert_eq!(
            self.events.subscription_count(),
            1,
            "live subscription must precede replay reads"
        );
        for message in self.arrivals.lock().unwrap().drain(..) {
            self.events
                .publish(construct_event::new_message_created_event(message));
        }
        if self.fail {
            return Err(StoreError::Internal("replay failure".into()));
        }
        self.inner.list_messages(filter, opts).await
    }
}
#[async_trait]
impl Store for ReplayStore {
    async fn begin(&self) -> Result<Box<dyn StoreTransaction>, StoreError> {
        self.inner.begin().await
    }
}

#[tokio::test]
async fn replay_read_race_is_lossless_and_deduplicated() {
    for fail in [false, true] {
        let events = Arc::new(EventRouter::default());
        let store = Arc::new(ReplayStore {
            inner: MemoryStore::default(),
            events: events.clone(),
            arrivals: Mutex::new(vec![]),
            fail,
        });
        let task_id = Uuid::new_v4();
        store
            .create_task(db::Task {
                id: task_id,
                phase: "awaiting".into(),
                desired_phase: "running".into(),
                ..Default::default()
            })
            .await
            .unwrap();
        let first = db::Message {
            id: Uuid::new_v4(),
            task_id,
            source: "user".into(),
            create_time: Timestamp(Utc::now() - chrono::Duration::seconds(10)),
            ..Default::default()
        };
        let second = db::Message {
            id: Uuid::new_v4(),
            task_id,
            source: "assistant".into(),
            create_time: Timestamp(Utc::now() - chrono::Duration::seconds(5)),
            ..Default::default()
        };
        let live = db::Message {
            id: Uuid::new_v4(),
            task_id,
            source: "assistant".into(),
            create_time: now(),
            ..Default::default()
        };
        let mut bad = first.clone();
        bad.id = Uuid::new_v4();
        bad.create_time = Timestamp(first.create_time.0 - chrono::Duration::seconds(1));
        bad.content = Some(construct_types::memory::MessageContent {
            blocks: Some(vec![construct_types::memory::MessageBlock {
                kind: "tool_call".into(),
                payload: "invalid JSON".into(),
            }]),
        });
        store.create_message(bad).await.unwrap();
        store.create_message(second.clone()).await.unwrap();
        store.create_message(first.clone()).await.unwrap();
        *store.arrivals.lock().unwrap() = if fail {
            vec![live.clone()]
        } else {
            vec![first.clone(), second.clone(), live.clone()]
        };
        let service = Services {
            store,
            events: events.clone(),
            encryption: Arc::new(Encryption::new(Keyset::generate()).unwrap()),
            analytics: Arc::new(crate::NoopAnalytics),
            tokens: Arc::new(TokenProvider::new()),
        };
        let mut stream = service
            .subscribe(Request::new(pb::EventSubscribeRequest {
                task_id: Some(task_id.to_string()),
                ..Default::default()
            }))
            .await
            .unwrap()
            .message;
        assert_eq!(events.subscription_count(), 1);
        let expected = if fail {
            vec![live.id]
        } else {
            vec![first.id, second.id, live.id]
        };
        for id in expected {
            let result = stream.next().await.unwrap().unwrap();
            match result.event.unwrap().payload.unwrap() {
                pb::event::Payload::Message(m) => {
                    assert_eq!(m.message.unwrap().metadata.unwrap().id, id.to_string())
                }
                other => panic!("unexpected payload: {other:?}"),
            }
        }
        events.close();
        assert!(stream.next().await.is_none());
        drop(stream);
        assert_eq!(events.subscription_count(), 0);
    }
}

#[tokio::test]
async fn dropping_an_unpolled_stream_unregisters_its_subscription() {
    let events = Arc::new(EventRouter::default());
    let service = Services {
        store: Arc::new(MemoryStore::default()),
        events: events.clone(),
        encryption: Arc::new(Encryption::new(Keyset::generate()).unwrap()),
        analytics: Arc::new(crate::NoopAnalytics),
        tokens: Arc::new(TokenProvider::new()),
    };
    let response = service
        .subscribe(Request::new(pb::EventSubscribeRequest::default()))
        .await
        .unwrap();
    assert_eq!(events.subscription_count(), 1);
    drop(response);
    assert_eq!(events.subscription_count(), 0);
}

#[test]
fn uuid_parsing_matches_google_uuid() {
    let expected = Uuid::new_v4();
    for id in [
        expected.to_string(),
        expected.simple().to_string(),
        format!("URN:UUID:{expected}"),
        format!("{{{expected}}}"),
        format!("x{expected}y"),
    ] {
        assert_eq!(parse_id(&id, "ID").unwrap(), expected);
    }
    for (input, message) in [
        ("", "invalid UUID length: 0"),
        (
            "00000000-0000-0000-0000-00000000000x",
            "invalid UUID format",
        ),
        (
            "00000000x0000-0000-0000-000000000000",
            "invalid UUID format",
        ),
        ("0000000000000000000000000000000x", "invalid UUID format"),
        (
            "xxxxxxxxx00000000-0000-0000-0000-000000000000",
            "invalid urn prefix: \"xxxxxxxxx\"",
        ),
    ] {
        assert_eq!(
            parse_id(input, "ID").unwrap_err().message,
            format!("invalid ID format: {message}")
        );
    }
}
