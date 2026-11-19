//! Go circuit breaker and Anthropic's internal retry policy.
use construct_core::{model::InvokeOptions, CancellationToken};
use construct_types::provider::{ProviderError, ProviderErrorKind};
use std::{future::Future, sync::Mutex, time::Duration};
use tokio::time::Instant;

/// Go's shared retry configuration.
#[derive(Clone, Debug)]
pub struct RetryConfig {
    /// Total attempts, including the initial request; zero means unlimited.
    pub max_attempts: u32,
    /// Retained for Go compatibility; Go's provider loop uses 500ms instead.
    pub initial_delay: Duration,
    /// Maximum total elapsed time; zero means unlimited.
    pub max_delay: Duration,
    /// Retained for compatibility; Go's internal loop does not use provider backoff.
    pub use_provider_backoff: bool,
    /// Retained for compatibility; Go's provider loop uses 1.5 instead.
    pub backoff_multiplier: f64,
}

impl Default for RetryConfig {
    fn default() -> Self {
        Self {
            max_attempts: 5,
            initial_delay: Duration::from_secs(1),
            max_delay: Duration::from_secs(10),
            use_provider_backoff: false,
            backoff_multiplier: 2.0,
        }
    }
}

/// Circuit state, matching shared/resilience.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CircuitState {
    /// Requests are allowed.
    Closed,
    /// Requests are rejected until the reset deadline.
    Open,
    /// Requests are allowed while probing recovery (not single-flight).
    HalfOpen,
}

struct State {
    circuit: CircuitState,
    failures: usize,
    reopen_at: Instant,
}

/// Thread-safe consecutive-failure circuit breaker.
pub struct CircuitBreaker {
    provider: String,
    threshold: usize,
    reset_timeout: Duration,
    state: Mutex<State>,
}

impl CircuitBreaker {
    /// Create a closed breaker for a provider.
    pub fn new(provider: impl Into<String>, threshold: usize, reset_timeout: Duration) -> Self {
        Self {
            provider: provider.into(),
            threshold,
            reset_timeout,
            state: Mutex::new(State {
                circuit: CircuitState::Closed,
                failures: 0,
                reopen_at: Instant::now(),
            }),
        }
    }

    /// Return current state without advancing the reset deadline.
    pub fn state(&self) -> CircuitState {
        self.state.lock().unwrap().circuit
    }

    /// Allow a request, moving an expired open circuit to half-open.
    pub fn allow(&self) -> bool {
        let mut state = self.state.lock().unwrap();
        if state.circuit == CircuitState::Open {
            if Instant::now() <= state.reopen_at {
                return false;
            }
            state.circuit = CircuitState::HalfOpen;
        }
        true
    }

    /// Record success or failure. Every failed attempt counts, regardless of kind.
    pub fn record_result(&self, success: bool) {
        let mut state = self.state.lock().unwrap();
        if success {
            state.failures = 0;
            if state.circuit == CircuitState::HalfOpen {
                state.circuit = CircuitState::Closed;
            }
        } else {
            state.failures = state.failures.saturating_add(1);
            if state.failures >= self.threshold {
                state.circuit = CircuitState::Open;
                state.reopen_at = Instant::now() + self.reset_timeout;
            }
        }
    }
}

/// Cancellation error suitable for any shared transport or retry operation.
pub fn canceled(provider: &str) -> ProviderError {
    ProviderError {
        provider: provider.into(),
        kind: ProviderErrorKind::CANCELED.into(),
        ..Default::default()
    }
}

/// Await an operation or cancellation, dropping in-flight work when canceled.
pub async fn cancelable<T>(
    provider: &str,
    cancellation: &CancellationToken,
    operation: impl Future<Output = Result<T, ProviderError>>,
) -> Result<T, ProviderError> {
    tokio::select! {
        biased;
        _ = cancellation.cancelled() => Err(canceled(provider)),
        result = operation => result,
    }
}

/// Run Go's internal provider retry loop.
///
/// Only overloaded/internal/timeout failures retry, unlike the *public*
/// ProviderError::retryable policy (rate limits are permanent here).
/// Matches backoff/v5 defaults: 500ms initial interval, 1.5 multiplier,
/// 60s interval cap and uniform 0.5..1.5 jitter. RetryConfig's initial delay,
/// multiplier and provider-backoff fields are intentionally unused, as in Go.
/// The attempt must include the complete stream consumption, not just headers.
pub async fn retry<T, F, Fut>(
    config: &RetryConfig,
    breaker: &CircuitBreaker,
    options: &InvokeOptions,
    mut attempt: F,
) -> Result<T, ProviderError>
where
    F: FnMut() -> Fut,
    Fut: Future<Output = Result<T, ProviderError>>,
{
    let start = Instant::now();
    let mut tries = 0_u32;
    let mut interval = 0.5_f64;
    loop {
        if options.cancellation.is_cancelled() {
            return Err(canceled(&breaker.provider));
        }
        if !breaker.allow() {
            return Err(ProviderError {
                provider: breaker.provider.clone(),
                kind: ProviderErrorKind::UNKNOWN.into(),
                err: Some(format!(
                    "too many errors from {} provider, circuit breaker open",
                    breaker.provider
                )),
                ..Default::default()
            });
        }
        tries = tries.saturating_add(1);
        let result = cancelable(&breaker.provider, &options.cancellation, attempt()).await;
        breaker.record_result(result.is_ok());
        let error = match result {
            Ok(value) => return Ok(value),
            Err(error) => error,
        };
        if (config.max_attempts > 0 && tries >= config.max_attempts)
            || !matches!(
                error.kind.as_ref(),
                ProviderErrorKind::OVERLOADED
                    | ProviderErrorKind::INTERNAL
                    | ProviderErrorKind::TIMEOUT
            )
        {
            return Err(error);
        }
        let delay = Duration::from_secs_f64(interval * rand::random_range(0.5..1.5));
        interval = (interval * 1.5).min(60.0);
        if !config.max_delay.is_zero() && start.elapsed() + delay > config.max_delay {
            return Err(error);
        }
        if let Some(callback) = &options.retry_callback {
            callback(&error, delay);
        }
        cancelable(&breaker.provider, &options.cancellation, async {
            tokio::time::sleep(delay).await;
            Ok(())
        })
        .await?;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{
        atomic::{AtomicUsize, Ordering},
        Arc,
    };
    fn error(kind: &str) -> ProviderError {
        ProviderError {
            kind: kind.into(),
            ..Default::default()
        }
    }
    #[tokio::test(start_paused = true)]
    async fn breaker_recovery() {
        let cb = CircuitBreaker::new("test", 2, Duration::from_secs(10));
        cb.record_result(false);
        assert!(cb.allow());
        cb.record_result(true);
        cb.record_result(false);
        assert_eq!(cb.state(), CircuitState::Closed);
        cb.record_result(false);
        assert!(!cb.allow());
        tokio::time::advance(Duration::from_secs(10)).await;
        assert!(!cb.allow());
        tokio::time::advance(Duration::from_nanos(1)).await;
        assert!(cb.allow());
        assert!(cb.allow());
        cb.record_result(false);
        assert!(!cb.allow());
        tokio::time::advance(Duration::from_secs(11)).await;
        assert!(cb.allow());
        cb.record_result(true);
        assert_eq!(cb.state(), CircuitState::Closed);
    }
    #[tokio::test(start_paused = true)]
    async fn retry_success_and_callback() {
        let cb = CircuitBreaker::new("test", 5, Duration::from_secs(10));
        let calls = Arc::new(AtomicUsize::new(0));
        let observed = calls.clone();
        let options = InvokeOptions {
            retry_callback: Some(Arc::new(move |_, delay| {
                assert!((0.25..0.75).contains(&delay.as_secs_f64()));
                observed.fetch_add(1, Ordering::SeqCst);
            })),
            ..Default::default()
        };
        let mut n = 0;
        let result = retry(&RetryConfig::default(), &cb, &options, || {
            n += 1;
            std::future::ready(if n == 1 {
                Err(error(ProviderErrorKind::INTERNAL))
            } else {
                Ok(42)
            })
        })
        .await;
        assert_eq!(result.unwrap(), 42);
        assert_eq!(n, 2);
        assert_eq!(calls.load(Ordering::SeqCst), 1);
    }
    #[tokio::test(start_paused = true)]
    async fn retry_limits_permanent_and_cancellation() {
        for (kind, expected) in [
            (ProviderErrorKind::INTERNAL, 3),
            (ProviderErrorKind::RATE_LIMIT_EXCEEDED, 1),
        ] {
            let cb = CircuitBreaker::new("test", 10, Duration::from_secs(10));
            let config = RetryConfig {
                max_attempts: 3,
                ..Default::default()
            };
            let mut n = 0;
            let result = retry::<(), _, _>(&config, &cb, &InvokeOptions::default(), || {
                n += 1;
                std::future::ready(Err(error(kind)))
            })
            .await;
            assert_eq!(result.unwrap_err().kind.as_ref(), kind);
            assert_eq!(n, expected);
        }
        let options = InvokeOptions::default();
        let token = options.cancellation.clone();
        let cb = CircuitBreaker::new("test", 10, Duration::from_secs(10));
        let result = retry::<(), _, _>(&RetryConfig::default(), &cb, &options, || {
            token.cancel();
            std::future::pending()
        })
        .await;
        assert_eq!(
            result.unwrap_err().kind.as_ref(),
            ProviderErrorKind::CANCELED
        );
    }
    #[tokio::test(start_paused = true)]
    async fn elapsed_limit_and_open_circuit() {
        let cb = CircuitBreaker::new("test", 1, Duration::from_secs(10));
        let config = RetryConfig {
            max_delay: Duration::from_millis(1),
            ..Default::default()
        };
        let result = retry::<(), _, _>(&config, &cb, &InvokeOptions::default(), || {
            std::future::ready(Err(error(ProviderErrorKind::INTERNAL)))
        })
        .await;
        assert_eq!(
            result.unwrap_err().kind.as_ref(),
            ProviderErrorKind::INTERNAL
        );
        let result = retry::<(), _, _>(&config, &cb, &InvokeOptions::default(), || async {
            panic!("open circuit invoked")
        })
        .await;
        assert!(result
            .unwrap_err()
            .err
            .unwrap()
            .contains("circuit breaker open"));
    }
}
