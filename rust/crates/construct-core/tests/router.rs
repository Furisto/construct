use construct_core::event::*;
use construct_types::Timestamp;
use futures_util::{FutureExt, StreamExt};
use std::{sync::Arc, time::Duration};
use uuid::Uuid;

fn event(kind: &str, task: Option<Uuid>, value: u32) -> Event {
    Event {
        r#type: kind.into(),
        action: String::new(),
        timestamp: Timestamp::default(),
        task_id: task,
        payload: Arc::new(value),
    }
}
fn opts(patterns: &[&str]) -> SubscribeOptions {
    SubscribeOptions {
        event_types: patterns.iter().map(|p| (*p).into()).collect(),
        ..Default::default()
    }
}
fn empty(stream: &mut Subscription) {
    assert!(stream.next().now_or_never().is_none());
}

#[tokio::test]
async fn go_subscribe_unsubscribe_and_multiple_subscribers() {
    let router = EventRouter::default();
    let mut first = router.subscribe(opts(&["task.*"]));
    let mut second = router.subscribe(opts(&["*.created"]));
    assert_eq!(router.subscription_count(), 2);
    router.publish(event("message.chunk", None, 0));
    empty(&mut first);
    empty(&mut second);
    router.publish(event("task.created", None, 42));
    let a = first.next().await.unwrap();
    let b = second.next().await.unwrap();
    assert!(Arc::ptr_eq(&a, &b));
    assert_eq!(a.payload.downcast_ref::<u32>(), Some(&42));
    first.cancel();
    first.cancel();
    assert_eq!(router.subscription_count(), 1);
    assert!(first.next().await.is_none());
    drop(second);
    assert_eq!(router.subscription_count(), 0);
}

#[tokio::test]
async fn go_patterns_empty_scope_invalid_task_and_internal_filtering() {
    let router = EventRouter::default();
    let task = Uuid::from_u128(1);
    let other = Uuid::from_u128(2);
    let mut all = router.subscribe(opts(&[]));
    let mut scoped = router.subscribe(SubscribeOptions {
        task_id: task.to_string(),
        ..opts(&["task.*"])
    });
    let mut invalid = router.subscribe(SubscribeOptions {
        task_id: "bad uuid".into(),
        ..opts(&["task.*"])
    });
    router.publish(event("task.updated", Some(other), 0));
    assert!(all.next().await.is_some());
    assert!(invalid.next().await.is_some());
    empty(&mut scoped);
    router.publish(event("task.updated", None, 0));
    empty(&mut scoped);
    router.publish(event("task.created", Some(task), 0));
    assert!(scoped.next().await.is_some());
    let mut filtered = router.subscribe(opts(&["internal.task.trigger"]));
    let mut mixed = router.subscribe(opts(&["internal.*", "task.*"]));
    let mut internal = router.subscribe(SubscribeOptions {
        internal: true,
        ..opts(&["internal.*"])
    });
    let mut internal_star = router.subscribe(SubscribeOptions {
        internal: true,
        ..opts(&[])
    });
    router.publish(event("internal.task.trigger", Some(task), 0));
    assert!(internal.next().await.is_some());
    empty(&mut filtered);
    empty(&mut mixed);
    empty(&mut internal_star);
    router.publish(event("task.created", Some(task), 0));
    assert!(mixed.next().await.is_some());
    empty(&mut filtered);
}

#[tokio::test]
async fn go_task_scope_accepts_wrapped_and_case_folded_urn_uuids() {
    let router = EventRouter::default();
    let task = Uuid::from_u128(1);
    for task_id in [
        task.simple().to_string(),
        format!("{{{task}}}"),
        format!("[{task}]"),
        format!("URN:UUID:{task}"),
    ] {
        let mut stream = router.subscribe(SubscribeOptions {
            task_id,
            ..Default::default()
        });
        router.publish(event("task.created", Some(Uuid::from_u128(2)), 0));
        empty(&mut stream);
        router.publish(event("task.created", Some(task), 0));
        assert_eq!(stream.next().await.unwrap().task_id, Some(task));
    }
}

#[tokio::test]
async fn go_slow_subscriber_drops_new_events_and_default_buffer() {
    let router = EventRouter::new(2);
    let mut slow = router.subscribe(opts(&[]));
    let mut fast = router.subscribe(opts(&[]));
    for n in 0..10 {
        router.publish(event("task.created", None, n));
        assert!(fast.next().await.is_some());
    }
    router.close();
    for expected in [0, 1] {
        assert_eq!(
            slow.next().await.unwrap().payload.downcast_ref::<u32>(),
            Some(&expected)
        );
    }
    assert!(slow.next().await.is_none());
    let router = EventRouter::new(0);
    let mut stream = router.subscribe(opts(&[]));
    for n in 0..101 {
        router.publish(event("task.created", None, n));
    }
    router.close();
    let mut count = 0;
    while stream.next().await.is_some() {
        count += 1;
    }
    assert_eq!(count, 100);
}

#[tokio::test]
async fn go_close_publish_after_close_context_cancel_and_replay_boundary() {
    let router = EventRouter::default();
    let mut sub = router.subscribe(SubscribeOptions {
        replay_after_message_id: Uuid::nil().to_string(),
        ..opts(&[])
    });
    empty(&mut sub); // replay belongs to the API/store, just as in Go.
    router.publish(event("task.created", None, 0));
    router.close();
    router.close();
    assert_eq!(router.subscription_count(), 0);
    assert!(sub.next().await.is_some());
    assert!(sub.next().await.is_none());
    router.publish(event("task.created", None, 1));
    assert!(router.subscribe(opts(&[])).next().await.is_none());
    let router = EventRouter::default();
    let cancel = construct_core::CancellationToken::new();
    let stream = router.subscribe(opts(&[]));
    cancel.cancel();
    tokio::select! {_ = cancel.cancelled()=>drop(stream), _ = tokio::time::sleep(Duration::from_secs(1))=>panic!("cancellation did not wake")};
    assert_eq!(router.subscription_count(), 0);
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn go_concurrent_publish_subscribe_drop() {
    let router = Arc::new(EventRouter::new(1));
    let mut tasks = Vec::new();
    for _ in 0..8 {
        let router = router.clone();
        tasks.push(tokio::spawn(async move {
            for n in 0..100 {
                let sub = router.subscribe(opts(&[]));
                router.publish(event("task.created", None, n));
                drop(sub);
                tokio::task::yield_now().await;
            }
        }));
    }
    for task in tasks {
        task.await.unwrap();
    }
    assert_eq!(router.subscription_count(), 0);
}
