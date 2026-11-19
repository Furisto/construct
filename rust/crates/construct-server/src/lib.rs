//! Construct's authenticated Connect API services and Go-compatible wire adapters.
#![deny(missing_docs)]

pub mod auth;
pub mod conv;
pub mod error;
mod handlers;
mod runtime;

pub use runtime::{
    api_router, AgentRuntime, Analytics, HandlerOptions, NoopAnalytics, StaticRuntime,
};
