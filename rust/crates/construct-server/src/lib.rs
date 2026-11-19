//! Construct's authenticated Connect API services and Go-compatible wire adapters.
#![deny(missing_docs)]

pub mod auth;
pub mod conv;
pub mod error;
mod handlers;
pub mod listener;
mod runtime;
mod server;

pub use server::Server;

pub use runtime::{
    api_router, AgentRuntime, Analytics, HandlerOptions, NoopAnalytics, StaticRuntime,
};
