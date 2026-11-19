//! Feature-gated, deterministic in-memory fakes. Not production persistence.
use crate::{
    model::{InvokeOptions, ModelProfile, ModelProvider, ToolDefinition},
    secret::{SecretError, SecretProvider},
};
use async_trait::async_trait;
use construct_types::provider::{Message, ProviderError, ProviderErrorKind};
use std::{
    collections::{HashMap, VecDeque},
    sync::Mutex,
    time::Duration,
};

#[path = "testing_store.rs"]
mod store;
/// HashMap-backed repositories with isolated optimistic transactions.
pub use store::MemoryStore;

/// Plaintext fake secret storage; clones should be shared via Arc, not serialized.
#[derive(Default)]
pub struct MemorySecretProvider {
    values: Mutex<HashMap<String, String>>,
}
#[async_trait]
impl SecretProvider for MemorySecretProvider {
    async fn get(&self, key: &str) -> Result<String, SecretError> {
        self.values
            .lock()
            .expect("secret fake poisoned")
            .get(key)
            .cloned()
            .ok_or_else(|| SecretError::NotFound {
                key: key.into(),
                cause: "missing fake secret".into(),
            })
    }
    async fn set(&self, key: &str, value: &str) -> Result<(), SecretError> {
        self.values
            .lock()
            .expect("secret fake poisoned")
            .insert(key.into(), value.into());
        Ok(())
    }
    async fn delete(&self, key: &str) -> Result<(), SecretError> {
        self.values
            .lock()
            .expect("secret fake poisoned")
            .remove(key);
        Ok(())
    }
}

/// A scripted provider retry notification (no sleep is performed).
#[derive(Clone, Debug)]
pub struct RetryNotice {
    /// Failure reported to the observer.
    pub error: ProviderError,
    /// Advertised next delay.
    pub delay: Duration,
}
/// One fake invocation. Expected-model mismatches consume the step and fail.
pub struct ModelStep {
    /// Optional exact model assertion.
    pub expected_model: Option<String>,
    /// Ordered text notifications before the result.
    pub chunks: Vec<String>,
    /// Ordered retry notifications before text chunks; no policy is simulated.
    pub retries: Vec<RetryNotice>,
    /// Final message or provider failure.
    pub result: Result<Message, ProviderError>,
}

/// Captured accepted invocation, excluding callbacks and secret-bearing debug output.
#[derive(Clone)]
pub struct Invocation {
    /// Model name.
    pub model: String,
    /// System prompt.
    pub system_prompt: String,
    /// Input conversation.
    pub messages: Vec<Message>,
    /// Tool metadata.
    pub tools: Vec<ToolDefinition>,
    /// Explicit profile, if supplied.
    pub profile: Option<ModelProfile>,
}

/// Scripted fake: cancellation is checked before consuming a step and between
/// callbacks. Exhaustion returns Internal, never silently invents a response.
#[derive(Default)]
pub struct ScriptedModelProvider {
    steps: Mutex<VecDeque<ModelStep>>,
    calls: Mutex<Vec<Invocation>>,
}
impl ScriptedModelProvider {
    /// Initialize the FIFO script.
    pub fn new(steps: impl IntoIterator<Item = ModelStep>) -> Self {
        Self {
            steps: Mutex::new(steps.into_iter().collect()),
            calls: Mutex::new(Vec::new()),
        }
    }
    /// Snapshot accepted calls (pre-cancelled calls are not recorded).
    pub fn calls(&self) -> Vec<Invocation> {
        self.calls.lock().expect("model fake poisoned").clone()
    }
    /// Number of unconsumed scripted steps.
    pub fn remaining(&self) -> usize {
        self.steps.lock().expect("model fake poisoned").len()
    }
}
fn failure(kind: &str, message: &str) -> ProviderError {
    ProviderError {
        provider: "scripted".into(),
        kind: kind.into(),
        err: Some(message.into()),
        retry_after: 0,
    }
}
#[async_trait]
impl ModelProvider for ScriptedModelProvider {
    async fn invoke_model(
        &self,
        model: &str,
        system_prompt: &str,
        messages: &[Message],
        options: InvokeOptions,
    ) -> Result<Message, ProviderError> {
        let check_cancel = || {
            if options.cancellation.is_cancelled() {
                Err(failure(ProviderErrorKind::CANCELED, "cancelled"))
            } else {
                Ok(())
            }
        };
        check_cancel()?;
        self.calls
            .lock()
            .expect("model fake poisoned")
            .push(Invocation {
                model: model.into(),
                system_prompt: system_prompt.into(),
                messages: messages.to_vec(),
                tools: options.tools.clone(),
                profile: options.profile.clone(),
            });
        let step = self
            .steps
            .lock()
            .expect("model fake poisoned")
            .pop_front()
            .ok_or_else(|| failure(ProviderErrorKind::INTERNAL, "script exhausted"))?;
        if step.expected_model.as_ref().is_some_and(|v| v != model) {
            return Err(failure(
                ProviderErrorKind::INVALID_REQUEST,
                "unexpected model",
            ));
        }
        for notice in step.retries {
            check_cancel()?;
            if let Some(callback) = &options.retry_callback {
                callback(&notice.error, notice.delay);
            }
        }
        for chunk in step.chunks {
            check_cancel()?;
            if let Some(sink) = &options.text_sink {
                sink(&chunk);
            }
        }
        check_cancel()?;
        step.result
    }
}
