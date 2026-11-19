//! Go-compatible supporting services for agent orchestration.
//! Task reconciliation and runtime scheduling are intentionally separate.

pub mod analytics;
pub mod conv;
pub mod environment;
pub mod metrics;
pub mod prompt;
pub mod provider;
pub mod title;

/// Supporting-service error with the Go operation context retained.
pub type Error = Box<dyn std::error::Error + Send + Sync>;
/// Supporting-service result.
pub type Result<T> = std::result::Result<T, Error>;
