//! Conversation condensation, including Go's deliberately unfinished summarizer.
use construct_core::{async_trait, model::ModelProvider, CancellationToken};
use construct_types::provider::{Message, MessageSource, Model};
use std::sync::Arc;

/// Changes proposed by a condenser. References retain message identity.
#[derive(Default)]
pub struct CondenserResult<'a> {
    /// Messages to append.
    pub added_messages: Vec<Message>,
    /// Existing messages to remove.
    pub removed_messages: Vec<&'a Message>,
}

/// Conversation truncation contract.
#[async_trait]
pub trait Condenser: Send + Sync {
    /// Propose changes without modifying the conversation.
    async fn condense<'a>(
        &self,
        cancellation: &CancellationToken,
        messages: &'a [Message],
    ) -> Result<CondenserResult<'a>, String>;
}

/// Remove a bounded middle portion when the last model usage reaches a threshold.
pub struct TruncationCondenser {
    /// Maximum context window size.
    pub context_window: i64,
    /// Fraction of the context window triggering truncation.
    pub truncation_ratio: f64,
    /// Number of messages to retain at both ends.
    pub preserve_count: usize,
    /// Maximum fraction of eligible messages removed in one pass.
    pub max_removal_percent: f64,
}

impl TruncationCondenser {
    /// Go defaults: 80% threshold, two preserved at each end, 50% removal.
    pub fn new(context_window: i64) -> Self {
        Self {
            context_window,
            truncation_ratio: 0.8,
            preserve_count: 2,
            max_removal_percent: 0.5,
        }
    }
}

fn total_tokens(message: &Message) -> i64 {
    let usage = &message.usage;
    usage
        .input_tokens
        .wrapping_add(usage.output_tokens)
        .wrapping_add(usage.cache_read_tokens)
        .wrapping_add(usage.cache_write_tokens)
}

#[async_trait]
impl Condenser for TruncationCondenser {
    async fn condense<'a>(
        &self,
        _cancellation: &CancellationToken,
        messages: &'a [Message],
    ) -> Result<CondenserResult<'a>, String> {
        let Some(last) = messages
            .iter()
            .rev()
            .find(|m| m.source.as_ref() == MessageSource::MODEL)
        else {
            return Ok(CondenserResult::default());
        };
        if total_tokens(last) < (self.context_window as f64 * self.truncation_ratio) as i64
            || messages.len() < self.preserve_count.saturating_mul(2).saturating_add(2)
        {
            return Ok(CondenserResult::default());
        }
        let eligible = &messages[self.preserve_count..messages.len() - self.preserve_count];
        let count = ((eligible.len() as f64 * self.max_removal_percent) as usize).max(1);
        let start = (eligible.len() / 2).saturating_sub(count / 2);
        let end = start.saturating_add(count).min(eligible.len());
        Ok(CondenserResult {
            added_messages: Vec::new(),
            removed_messages: eligible[start..end].iter().collect(),
        })
    }
}

/// Go's summarizer currently only checks for a model message and returns no changes.
/// It deliberately does not implement Condenser: Go has a different signature.
pub struct SummarizationCondenser {
    _model_provider: Arc<dyn ModelProvider>,
}

/// Identity-preserving replacement groups, reserved for the unfinished Go summarizer.
pub type SummaryReplacements<'a> = Vec<(&'a Message, Vec<Message>)>;

impl SummarizationCondenser {
    /// Retain a provider for future summarization; Go does not call it yet.
    pub fn new(model_provider: Arc<dyn ModelProvider>) -> Self {
        Self {
            _model_provider: model_provider,
        }
    }

    /// Match Go: error without a model message; otherwise return no replacements.
    pub async fn condense<'a>(
        &self,
        _cancellation: &CancellationToken,
        model: &Model,
        messages: &'a [Message],
    ) -> Result<Option<SummaryReplacements<'a>>, String> {
        let last = messages
            .iter()
            .rev()
            .find(|m| m.source.as_ref() == MessageSource::MODEL)
            .ok_or("no model message found")?;
        if (total_tokens(last) as f64) < model.context_window as f64 * 0.8 {
            return Ok(None);
        }
        Ok(None)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use construct_core::model::InvokeOptions;
    use construct_types::provider::{ProviderError, Usage};

    fn messages(count: usize, tokens: Option<(usize, i64, i64, i64, i64)>) -> Vec<Message> {
        (0..count)
            .map(|i| {
                let mut message = Message {
                    source: MessageSource::USER.into(),
                    ..Default::default()
                };
                if let Some((index, input, output, read, write)) = tokens {
                    if i == index {
                        message.source = MessageSource::MODEL.into();
                        message.usage = Usage {
                            input_tokens: input,
                            output_tokens: output,
                            cache_read_tokens: read,
                            cache_write_tokens: write,
                        };
                    }
                }
                message
            })
            .collect()
    }
    #[tokio::test]
    async fn go_truncation_scenarios() {
        let default = || TruncationCondenser::new(100000);
        let scenarios = [
            ("empty messages", default(), messages(0, None), vec![]),
            ("no model message", default(), messages(5, None), vec![]),
            (
                "below threshold",
                default(),
                messages(4, Some((2, 30000, 20000, 5000, 5000))),
                vec![],
            ),
            (
                "insufficient messages",
                default(),
                messages(5, Some((2, 50000, 30000, 10000, 10000))),
                vec![],
            ),
            (
                "successful truncation",
                default(),
                messages(8, Some((7, 50000, 30000, 10000, 10000))),
                vec![3, 4],
            ),
            (
                "custom configuration",
                TruncationCondenser {
                    context_window: 50000,
                    truncation_ratio: 0.9,
                    preserve_count: 1,
                    max_removal_percent: 0.25,
                },
                messages(6, Some((5, 30000, 20000, 0, 0))),
                vec![3],
            ),
            (
                "threshold equality",
                default(),
                messages(6, Some((5, 80000, 0, 0, 0))),
                vec![3],
            ),
        ];
        for (name, condenser, messages, expected) in scenarios {
            let result = condenser
                .condense(&CancellationToken::new(), &messages)
                .await
                .unwrap();
            assert!(result.added_messages.is_empty(), "{name}");
            let indices = result
                .removed_messages
                .iter()
                .map(|message| {
                    messages
                        .iter()
                        .position(|m| std::ptr::eq(m, *message))
                        .unwrap()
                })
                .collect::<Vec<_>>();
            assert_eq!(indices, expected, "{name}");
        }
    }
    struct UnusedProvider;
    #[async_trait]
    impl ModelProvider for UnusedProvider {
        async fn invoke_model(
            &self,
            _: &str,
            _: &str,
            _: &[Message],
            _: InvokeOptions,
        ) -> Result<Message, ProviderError> {
            panic!("Go summarizer does not invoke the provider")
        }
    }
    #[tokio::test]
    async fn summarization_matches_go_stub() {
        let condenser = SummarizationCondenser::new(Arc::new(UnusedProvider));
        let token = CancellationToken::new();
        let model = Model {
            context_window: 100000,
            ..Default::default()
        };
        assert_eq!(
            condenser
                .condense(&token, &model, &[])
                .await
                .err()
                .as_deref(),
            Some("no model message found")
        );
        for count in [1, 100000] {
            assert!(condenser
                .condense(&token, &model, &messages(1, Some((0, count, 0, 0, 0))))
                .await
                .unwrap()
                .is_none());
        }
    }
}
