use construct_core::store::{
    AgentStore, MessageStore, ModelProviderStore, ModelStore, Store, TaskStore,
};
use construct_event::*;
use construct_store::SqliteStore;
use construct_types::entity::{Agent, Message, Model, ModelProvider, Task};
use futures_util::{FutureExt, StreamExt};
use std::sync::Arc;
use tempfile::TempDir;
use uuid::Uuid;

async fn setup() -> (TempDir, SqliteStore, Arc<EventRouter>) {
    let dir = tempfile::tempdir().unwrap();
    let router = Arc::new(EventRouter::default());
    let store = SqliteStore::open_with_hooks(
        dir.path().join("construct.db"),
        vec![Arc::new(EventHook::new(router.clone()))],
    )
    .await
    .unwrap();
    (dir, store, router)
}

fn pending(subscription: &mut Subscription) {
    assert!(subscription.next().now_or_never().is_none());
}

fn next(
    subscription: &mut Subscription,
    kind: &str,
    action: &str,
    scope: Option<Uuid>,
) -> Arc<Event> {
    let event = subscription
        .next()
        .now_or_never()
        .flatten()
        .unwrap_or_else(|| panic!("no {kind} event ready"));
    assert_eq!(event.r#type, kind);
    assert_eq!(event.action, action);
    assert_eq!(event.task_id, scope);
    event
}

fn deleted(subscription: &mut Subscription, kind: &str, id: Uuid, scope: Option<Uuid>) {
    let event = next(subscription, kind, "deleted", scope);
    let payload = event
        .payload
        .downcast_ref::<DeletedEntityPayload>()
        .unwrap();
    assert_eq!(payload.id, id);
    assert_eq!(payload.task_id, None);
}

fn agent(id: Uuid) -> Agent {
    Agent {
        id,
        name: "original".into(),
        ..Default::default()
    }
}

#[tokio::test]
async fn go_configuration_hooks_publish_committed_snapshots_in_order() {
    let (_dir, store, router) = setup().await;
    let mut subscription = router.subscribe(SubscribeOptions::default());
    let provider_id = Uuid::from_u128(1);
    let model_id = Uuid::from_u128(2);
    let agent_id = Uuid::from_u128(3);
    let mut provider = ModelProvider {
        id: provider_id,
        name: "original".into(),
        secret: vec![1],
        provider_type: "anthropic".into(),
        ..Default::default()
    };
    let mut model = Model {
        id: model_id,
        name: "original".into(),
        model_provider_id: provider_id,
        ..Default::default()
    };
    let mut agent = agent(agent_id);

    store.create_model_provider(provider.clone()).await.unwrap();
    store.create_model(model.clone()).await.unwrap();
    store.create_agent(agent.clone()).await.unwrap();
    provider.name = "changed".into();
    model.name = "changed".into();
    agent.name = "changed".into();
    store.update_model_provider(provider).await.unwrap();
    store.update_model(model).await.unwrap();
    store.update_agent(agent).await.unwrap();
    store.delete_agent(agent_id).await.unwrap();
    store.delete_model(model_id).await.unwrap();
    store.delete_model_provider(provider_id).await.unwrap();

    for (action, name) in [("created", "original"), ("updated", "changed")] {
        let event = next(
            &mut subscription,
            &format!("modelprovider.{action}"),
            action,
            None,
        );
        let provider = &event
            .payload
            .downcast_ref::<ModelProviderEventPayload>()
            .unwrap()
            .model_provider;
        assert_eq!(provider.id, provider_id);
        assert_eq!(provider.name, name);
        let event = next(&mut subscription, &format!("model.{action}"), action, None);
        let model = &event
            .payload
            .downcast_ref::<ModelEventPayload>()
            .unwrap()
            .model;
        assert_eq!(model.id, model_id);
        assert_eq!(model.model_provider_id, provider_id);
        assert_eq!(model.name, name);
        let event = next(&mut subscription, &format!("agent.{action}"), action, None);
        let agent = &event
            .payload
            .downcast_ref::<AgentEventPayload>()
            .unwrap()
            .agent;
        assert_eq!(agent.id, agent_id);
        assert_eq!(agent.name, name);
    }
    deleted(&mut subscription, "agent.deleted", agent_id, None);
    deleted(&mut subscription, "model.deleted", model_id, None);
    deleted(
        &mut subscription,
        "modelprovider.deleted",
        provider_id,
        None,
    );
    pending(&mut subscription);
}

#[tokio::test]
async fn go_task_hooks_are_scoped_and_skip_updates_and_messages() {
    let (_dir, store, router) = setup().await;
    let task_id = Uuid::from_u128(1);
    let message_id = Uuid::from_u128(2);
    let mut all = router.subscribe(SubscribeOptions::default());
    let mut scoped = router.subscribe(SubscribeOptions {
        task_id: task_id.to_string(),
        ..Default::default()
    });
    let mut task = Task {
        id: task_id,
        phase: "awaiting".into(),
        desired_phase: "running".into(),
        ..Default::default()
    };
    store.create_task(task.clone()).await.unwrap();
    for subscription in [&mut all, &mut scoped] {
        let event = next(subscription, "task.created", "created", Some(task_id));
        let payload = event.payload.downcast_ref::<TaskEventPayload>().unwrap();
        assert_eq!(payload.task.id, task_id);
        assert_eq!(payload.task.phase.as_ref(), "awaiting");
    }
    task.phase = "running".into();
    store.update_task(task).await.unwrap();
    let mut message = Message {
        id: message_id,
        task_id,
        source: "user".into(),
        ..Default::default()
    };
    store.create_message(message.clone()).await.unwrap();
    message.source = "assistant".into();
    store.update_message(message.clone()).await.unwrap();
    store.delete_message(message_id).await.unwrap();
    store.create_message(message).await.unwrap();
    pending(&mut all);
    pending(&mut scoped);
    store.delete_task(task_id).await.unwrap();
    assert!(store.get_message(message_id).await.is_err());
    for subscription in [&mut all, &mut scoped] {
        deleted(subscription, "task.deleted", task_id, Some(task_id));
        pending(subscription);
    }
}

#[tokio::test]
async fn transaction_hooks_only_publish_successfully_committed_writes() {
    let (_dir, store, router) = setup().await;
    let mut subscription = router.subscribe(SubscribeOptions::default());
    let id = Uuid::from_u128(1);
    let tx = store.begin().await.unwrap();
    tx.create_agent(agent(id)).await.unwrap();
    let mut updated = agent(id);
    updated.name = "changed".into();
    tx.update_agent(updated).await.unwrap();
    pending(&mut subscription);
    tx.commit().await.unwrap();
    let created = next(&mut subscription, "agent.created", "created", None);
    assert_eq!(
        created
            .payload
            .downcast_ref::<AgentEventPayload>()
            .unwrap()
            .agent
            .name,
        "original"
    );
    let updated = next(&mut subscription, "agent.updated", "updated", None);
    let committed = store.get_agent(id).await.unwrap();
    assert_eq!(
        updated
            .payload
            .downcast_ref::<AgentEventPayload>()
            .unwrap()
            .agent
            .name,
        committed.name
    );
    assert!(store.create_agent(agent(id)).await.is_err());
    assert!(store.delete_agent(Uuid::from_u128(2)).await.is_err());

    let tx = store.begin().await.unwrap();
    tx.delete_agent(id).await.unwrap();
    pending(&mut subscription);
    tx.rollback().await.unwrap();

    let tx = store.begin().await.unwrap();
    tx.delete_agent(id).await.unwrap();
    drop(tx);
    pending(&mut subscription);
    assert!(store.get_agent(id).await.is_ok());

    store.delete_agent(id).await.unwrap();
    deleted(&mut subscription, "agent.deleted", id, None);
    pending(&mut subscription);
}

#[tokio::test]
async fn go_delete_hook_skips_nil_uuid() {
    let (_dir, store, router) = setup().await;
    let mut subscription = router.subscribe(SubscribeOptions::default());
    let id = Uuid::nil();
    store.create_agent(agent(id)).await.unwrap();
    next(&mut subscription, "agent.created", "created", None);
    store.delete_agent(id).await.unwrap();
    assert!(store.get_agent(id).await.is_err());
    pending(&mut subscription);
}
