//! Domain event construction, live routing, and SQLite post-commit publication.
//! RPC conversion and replay belong to the event service, not the live router.
#![deny(missing_docs)]

mod constructors;
mod hooks;

pub use construct_core::event::{
    Event, EventPayload, EventPublisher, EventRouter, EventStream, EventSubscriber,
    SubscribeOptions, Subscription, DEFAULT_CHANNEL_BUFFER_SIZE,
};
pub use constructors::*;
pub use hooks::EventHook;
