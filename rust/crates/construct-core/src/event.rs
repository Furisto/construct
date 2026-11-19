//! Go-compatible in-memory pub/sub; no replay database or network transport.
use construct_types::event::{is_internal_event, match_pattern, StreamEvent};
use futures_util::Stream;
use std::{
    any::Any,
    collections::HashMap,
    pin::Pin,
    sync::{Arc, Mutex},
    task::{Context, Poll},
};
use tokio::sync::mpsc;
use uuid::Uuid;

/// The Go router's options, including internal patterns and optional task scope.
pub use construct_types::event::SubscribeOptions;
/// Shared arbitrary domain payload, equivalent to Go's any (downcast to records).
pub type EventPayload = Arc<dyn Any + Send + Sync>;
/// Internal domain event; not a serialized RPC message.
pub type Event = StreamEvent<EventPayload>;
/// Type-erased event source for alternate subscriber implementations.
pub type EventStream = Pin<Box<dyn Stream<Item = Arc<Event>> + Send + 'static>>;
/// Go's fallback subscriber buffer size.
pub const DEFAULT_CHANNEL_BUFFER_SIZE: usize = 100;

/// Non-blocking event publication; a slow subscriber drops the NEW event.
pub trait EventPublisher: Send + Sync {
    /// Publish synchronously without waiting for receiver capacity. Closed routers ignore it.
    fn publish(&self, event: Event);
}

/// Event subscription independent of persistence/replay.
pub trait EventSubscriber: Send + Sync {
    /// Empty patterns mean external wildcard; invalid task UUID means no scope,
    /// as in Go. ReplayAfterMessageID is handled by API/store replay, NOT the router.
    fn subscribe(&self, options: SubscribeOptions) -> Subscription;
}

struct Subscriber {
    patterns: Vec<String>,
    task_id: Option<Uuid>,
    sender: mpsc::Sender<Arc<Event>>,
}

fn parse_task_id(task_id: &str) -> Option<Uuid> {
    let bytes = task_id.as_bytes();
    // google/uuid accepts any single-byte wrappers and ASCII case-folds the URN prefix.
    let raw = match bytes.len() {
        38 => &bytes[1..37],
        45 if bytes[..9].eq_ignore_ascii_case(b"urn:uuid:") => &bytes[9..],
        32 | 36 => bytes,
        _ => return None,
    };
    Uuid::try_parse_ascii(raw).ok()
}

#[derive(Default)]
struct State {
    subscribers: HashMap<u64, Subscriber>,
    next_id: u64,
    closed: bool,
}

/// Reference router. Clones share subscriptions; it never spawns background tasks.
/// Mutex critical sections are bounded by subscriber count, as Go's RWMutex is;
/// publishing never waits for stream consumption or performs asynchronous I/O.
#[derive(Clone)]
pub struct EventRouter {
    state: Arc<Mutex<State>>,
    buffer_size: usize,
}

impl Default for EventRouter {
    fn default() -> Self {
        Self::new(0)
    }
}

impl EventRouter {
    /// Create a router; zero selects Go's default capacity (100).
    pub fn new(buffer_size: usize) -> Self {
        Self {
            state: Arc::new(Mutex::new(State::default())),
            buffer_size: if buffer_size == 0 {
                DEFAULT_CHANNEL_BUFFER_SIZE
            } else {
                buffer_size
            },
        }
    }

    /// Close all streams after buffered events drain; idempotent.
    pub fn close(&self) {
        let mut state = self.state.lock().expect("event router poisoned");
        state.closed = true;
        state.subscribers.clear();
    }

    /// Active subscription count (drops and cancellation remove entries immediately).
    pub fn subscription_count(&self) -> usize {
        self.state
            .lock()
            .expect("event router poisoned")
            .subscribers
            .len()
    }
}

impl EventPublisher for EventRouter {
    fn publish(&self, event: Event) {
        let event = Arc::new(event);
        let state = self.state.lock().expect("event router poisoned");
        if state.closed {
            return;
        }
        for sub in state.subscribers.values() {
            if sub.task_id.is_some_and(|task| event.task_id != Some(task)) {
                continue;
            }
            if sub.patterns.iter().any(|p| match_pattern(p, &event.r#type)) {
                let _ = sub.sender.try_send(event.clone());
            }
        }
    }
}

impl EventSubscriber for EventRouter {
    fn subscribe(&self, options: SubscribeOptions) -> Subscription {
        let mut state = self.state.lock().expect("event router poisoned");
        let (sender, receiver) = mpsc::channel(self.buffer_size);
        let stream: EventStream = Box::pin(ReceiverEvents(receiver));
        if state.closed {
            return Subscription::new(stream, || {});
        }
        let mut patterns = if options.event_types.is_empty() {
            vec!["*".into()]
        } else {
            options.event_types
        };
        if !options.internal {
            patterns.retain(|p| !is_internal_event(p));
        }
        let task_id = parse_task_id(&options.task_id);
        let id = state.next_id;
        state.next_id += 1;
        state.subscribers.insert(
            id,
            Subscriber {
                patterns,
                task_id,
                sender,
            },
        );
        let state = Arc::downgrade(&self.state);
        Subscription::new(stream, move || {
            if let Some(state) = state.upgrade() {
                state
                    .lock()
                    .expect("event router poisoned")
                    .subscribers
                    .remove(&id);
            }
        })
    }
}

/// An owned event stream. Drop or cancel unsubscribes immediately. No cleanup
/// task is needed. To mirror Go context cancellation, select next() against your
/// CancellationToken and drop the stream when canceled. Buffered events drain
/// after explicit cancel/close, matching closed Go channels.
pub struct Subscription {
    stream: EventStream,
    unsubscribe: Option<Box<dyn FnOnce() + Send>>,
}

impl Subscription {
    /// Wrap any source and a cleanup callback, allowing alternate dyn subscribers.
    /// Cleanup must stop new delivery/close the source, retain any queued events,
    /// and must not block on I/O. It is called exactly once on cancel or drop.
    pub fn new(stream: EventStream, unsubscribe: impl FnOnce() + Send + 'static) -> Self {
        Self {
            stream,
            unsubscribe: Some(Box::new(unsubscribe)),
        }
    }
    /// Stop delivery, retaining already-buffered messages for subsequent polls.
    pub fn cancel(&mut self) {
        if let Some(unsubscribe) = self.unsubscribe.take() {
            unsubscribe();
        }
    }
}
impl Stream for Subscription {
    type Item = Arc<Event>;
    fn poll_next(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Option<Self::Item>> {
        self.stream.as_mut().poll_next(cx)
    }
}
impl Drop for Subscription {
    fn drop(&mut self) {
        self.cancel();
    }
}

struct ReceiverEvents(mpsc::Receiver<Arc<Event>>);
impl Stream for ReceiverEvents {
    type Item = Arc<Event>;
    fn poll_next(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Option<Self::Item>> {
        self.0.poll_recv(cx)
    }
}
