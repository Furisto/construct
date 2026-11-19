//! Go-compatible task orchestration, runtime scheduling and supporting services.

pub mod analytics;
pub mod conv;
pub mod environment;
pub mod metrics;
pub mod prompt;
pub mod provider;
pub mod queue;
pub mod reconciler;
pub mod runtime;
pub mod title;
pub mod wire;

pub use runtime::{Runtime, RuntimeOptions};

/// Supporting-service error with the Go operation context retained.
pub type Error = Box<dyn std::error::Error + Send + Sync>;
/// Supporting-service result.
pub type Result<T> = std::result::Result<T, Error>;
