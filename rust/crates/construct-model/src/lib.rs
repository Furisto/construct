//! Shared provider catalog, transport and resilience policies.
#![deny(missing_docs)]

pub mod catalog;
pub mod http;
pub mod resilience;

use resilience::{CircuitBreaker, RetryConfig};
use std::{sync::Arc, time::Duration};

/// Provider-wide configuration. Metrics collection is left to callers.
#[derive(Clone)]
pub struct ProviderOptions {
    /// Base URL override; empty selects the provider's normal endpoint.
    pub url: String,
    /// Retry limits matching Go's default provider options.
    pub retry_config: RetryConfig,
    /// Shared breaker, including across cloned provider handles.
    pub circuit_breaker: Arc<CircuitBreaker>,
}

impl ProviderOptions {
    /// Construct Go's DefaultProviderOptions for the named provider.
    pub fn new(provider: impl Into<String>) -> Self {
        Self {
            url: String::new(),
            retry_config: RetryConfig::default(),
            circuit_breaker: Arc::new(CircuitBreaker::new(provider, 5, Duration::from_secs(10))),
        }
    }
}
