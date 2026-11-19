//! Object-safe contracts shared by Construct implementations.
//! Implementations own I/O, retry policy, encryption, and database drivers.
//! See the workspace README for lifecycle and transaction contracts.
#![deny(missing_docs)]

/// Async trait macro used by every core implementation.
pub use async_trait::async_trait;
/// Cancellation primitive shared by model calls and tools.
pub use tokio_util::sync::CancellationToken;

pub mod event;
pub mod model;
pub mod secret;
pub mod store;
pub mod tool;

#[cfg(feature = "testing")]
pub mod testing;
