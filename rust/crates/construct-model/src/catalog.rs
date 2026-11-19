//! Provider model catalogs. Ordering and even duplicate UUIDs match Go.
use construct_types::provider::{Capability, Model, ModelPricing, ProviderKind};

/// Models available from a provider; unknown/unported providers return an empty list.
pub fn supported_models(kind: &ProviderKind) -> Vec<Model> {
    match kind.as_ref() {
        ProviderKind::ANTHROPIC => anthropic::supported_models(),
        ProviderKind::OPEN_AI => openai::supported_models(),
        ProviderKind::XAI => xai::supported_models(),
        ProviderKind::GEMINI => gemini::supported_models(),
        _ => Vec::new(),
    }
}

/// Default model; unported/unsupported providers return "model not supported".
pub fn default_model(kind: &ProviderKind) -> Result<Model, &'static str> {
    match kind.as_ref() {
        ProviderKind::ANTHROPIC => Ok(anthropic::supported_models().remove(0)),
        ProviderKind::GEMINI => gemini::default_model(),
        _ => Err("model not supported"),
    }
}

/// Anthropic's complete Go catalog.
pub mod anthropic {
    use super::*;
    /// Go's budget model name.
    pub const BUDGET_MODEL: &str = "claude-haiku-4-5-20251001";
    /// Go's default invocation model name (catalog default is the first entry).
    pub const DEFAULT_MODEL: &str = "claude-sonnet-4-5-20250929";
    /// Go's planning model name.
    pub const PLAN_MODEL: &str = "claude-opus-4-5-20251101";
    /// All supported models in Go order.
    pub fn supported_models() -> Vec<Model> {
        vec![
            Model {
                id: uuid::uuid!("019abac1-a52d-7dbf-bf8d-d9155ca475dc"),
                name: "claude-opus-4-5-20251101".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::COMPUTER_USE.into(),
                    Capability::PROMPT_CACHE.into(),
                    Capability::EXTENDED_THINKING.into(),
                ]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 5.0,
                    output: 25.0,
                    cache_write: 6.25,
                    cache_read: 0.5,
                },
            },
            Model {
                id: uuid::uuid!("0199ee5a-ffd4-721f-9e41-ad8167f7d909"),
                name: "claude-haiku-4-5-20251001".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::COMPUTER_USE.into(),
                    Capability::PROMPT_CACHE.into(),
                    Capability::EXTENDED_THINKING.into(),
                ]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 1.0,
                    output: 5.0,
                    cache_write: 1.25,
                    cache_read: 0.1,
                },
            },
            Model {
                id: uuid::uuid!("0199ee5a-ffd4-721f-9e41-ad8167f7d909"),
                name: "claude-sonnet-4-5-20250929".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::COMPUTER_USE.into(),
                    Capability::PROMPT_CACHE.into(),
                    Capability::EXTENDED_THINKING.into(),
                ]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 3.0,
                    output: 15.0,
                    cache_write: 3.75,
                    cache_read: 0.3,
                },
            },
            Model {
                id: uuid::uuid!("0197e0d5-7567-70c6-8f64-e217dee9eb05"),
                name: "claude-sonnet-4-20250514".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::COMPUTER_USE.into(),
                    Capability::PROMPT_CACHE.into(),
                    Capability::EXTENDED_THINKING.into(),
                ]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 3.0,
                    output: 15.0,
                    cache_write: 3.75,
                    cache_read: 0.3,
                },
            },
            Model {
                id: uuid::uuid!("0197e0d5-8f08-7609-9fe0-d407b2563375"),
                name: "claude-opus-4-20250514".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::COMPUTER_USE.into(),
                    Capability::PROMPT_CACHE.into(),
                    Capability::EXTENDED_THINKING.into(),
                ]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 15.0,
                    output: 75.0,
                    cache_write: 18.75,
                    cache_read: 1.5,
                },
            },
            Model {
                id: uuid::uuid!("0195b4e2-45b6-76df-b208-f48b7b0d5f51"),
                name: "claude-3-7-sonnet-20250219".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::COMPUTER_USE.into(),
                    Capability::PROMPT_CACHE.into(),
                    Capability::EXTENDED_THINKING.into(),
                ]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 3.0,
                    output: 15.0,
                    cache_write: 3.75,
                    cache_read: 0.3,
                },
            },
            Model {
                id: uuid::uuid!("0195b4e2-7d71-79e0-97da-3045fb1ffc3e"),
                name: "claude-3-5-sonnet-20241022".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::COMPUTER_USE.into(),
                    Capability::PROMPT_CACHE.into(),
                ]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 3.0,
                    output: 15.0,
                    cache_write: 3.75,
                    cache_read: 0.3,
                },
            },
            Model {
                id: uuid::uuid!("0195b4e2-a5df-736d-82ea-00f46db3dadc"),
                name: "claude-3-5-sonnet-20240620".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::COMPUTER_USE.into(),
                    Capability::PROMPT_CACHE.into(),
                ]),
                context_window: 100000,
                pricing: ModelPricing {
                    input: 3.0,
                    output: 15.0,
                    cache_write: 3.75,
                    cache_read: 0.3,
                },
            },
            Model {
                id: uuid::uuid!("0195b4e2-c741-724d-bb2a-3b0f7fdbc5f4"),
                name: "claude-3-5-haiku-20241022".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![Capability::PROMPT_CACHE.into()]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 0.8,
                    output: 4.0,
                    cache_write: 1.0,
                    cache_read: 0.08,
                },
            },
            Model {
                id: uuid::uuid!("0195b4e2-efd4-7c5c-a9a2-219318e0e181"),
                name: "claude-3-opus-20240229".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::PROMPT_CACHE.into(),
                ]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 15.0,
                    output: 75.0,
                    cache_write: 18.75,
                    cache_read: 1.5,
                },
            },
            Model {
                id: uuid::uuid!("0195b4e3-1da7-71af-ba34-6689aed6c4a2"),
                name: "claude-3-haiku-20240307".into(),
                provider: ProviderKind::ANTHROPIC.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::PROMPT_CACHE.into(),
                ]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 0.25,
                    output: 1.25,
                    cache_write: 0.3,
                    cache_read: 0.03,
                },
            },
        ]
    }
}
/// Porting seam for the openai provider (not yet ported).
pub mod openai {
    use super::*;
    /// All supported models in Go order.
    pub fn supported_models() -> Vec<Model> {
        vec![
            Model {
                id: uuid::uuid!("01960000-0001-7000-8000-000000000001"),
                name: "chatgpt-4o-latest".into(),
                provider: ProviderKind::OPEN_AI.into(),
                capabilities: Some(vec![Capability::IMAGE.into()]),
                context_window: 128000,
                pricing: ModelPricing {
                    input: 2.500,
                    output: 10.000,
                    cache_write: 1.250,
                    cache_read: 0.250,
                },
            },
            Model {
                id: uuid::uuid!("01960000-0002-7000-8000-000000000002"),
                name: "o4-mini".into(),
                provider: ProviderKind::OPEN_AI.into(),
                capabilities: Some(vec![Capability::IMAGE.into()]),
                context_window: 128000,
                pricing: ModelPricing {
                    input: 0.150,
                    output: 0.600,
                    cache_write: 0.075,
                    cache_read: 0.015,
                },
            },
            Model {
                id: uuid::uuid!("01960000-0003-7000-8000-000000000003"),
                name: "gpt-4-turbo".into(),
                provider: ProviderKind::OPEN_AI.into(),
                capabilities: Some(vec![Capability::IMAGE.into()]),
                context_window: 128000,
                pricing: ModelPricing {
                    input: 10.000,
                    output: 30.000,
                    cache_write: 5.000,
                    cache_read: 1.000,
                },
            },
            Model {
                id: uuid::uuid!("01960000-0004-7000-8000-000000000004"),
                name: "gpt-3.5-turbo".into(),
                provider: ProviderKind::OPEN_AI.into(),
                capabilities: Some(vec![]),
                context_window: 16385,
                pricing: ModelPricing {
                    input: 0.500,
                    output: 1.500,
                    cache_write: 0.250,
                    cache_read: 0.050,
                },
            },
            Model {
                id: uuid::uuid!("01960000-0005-7000-8000-000000000005"),
                name: "o1".into(),
                provider: ProviderKind::OPEN_AI.into(),
                capabilities: Some(vec![]),
                context_window: 200000,
                pricing: ModelPricing {
                    input: 15.000,
                    output: 60.000,
                    cache_write: 7.500,
                    cache_read: 1.500,
                },
            },
            Model {
                id: uuid::uuid!("01960000-0006-7000-8000-000000000006"),
                name: "o1-mini".into(),
                provider: ProviderKind::OPEN_AI.into(),
                capabilities: Some(vec![]),
                context_window: 128000,
                pricing: ModelPricing {
                    input: 3.000,
                    output: 12.000,
                    cache_write: 1.500,
                    cache_read: 0.300,
                },
            },
            Model {
                id: uuid::uuid!("01960000-0007-7000-8000-000000000007"),
                name: "gpt-5-2025-08-07".into(),
                provider: ProviderKind::OPEN_AI.into(),
                capabilities: Some(vec![
                    Capability::IMAGE.into(),
                    Capability::COMPUTER_USE.into(),
                    Capability::PROMPT_CACHE.into(),
                    Capability::EXTENDED_THINKING.into(),
                ]),
                context_window: 128000,
                pricing: ModelPricing {
                    input: 1.250,
                    output: 10.000,
                    cache_write: 1.250,
                    cache_read: 0.250,
                },
            },
        ]
    }
    /// Placeholder default until the provider catalog is ported.
    pub fn default_model() -> Result<Model, &'static str> {
        Err("model not supported")
    }
}
/// Porting seam for the xai provider (not yet ported).
pub mod xai {
    use super::*;
    /// All supported models in Go order.
    pub fn supported_models() -> Vec<Model> {
        vec![
            Model {
                id: uuid::uuid!("01980000-0001-7000-8000-000000000001"),
                name: "grok-code-fast-1".into(),
                provider: ProviderKind::XAI.into(),
                capabilities: Some(vec![Capability::IMAGE.into()]),
                context_window: 256000,
                pricing: ModelPricing {
                    input: 0.200,
                    output: 1.500,
                    cache_write: 0.000,
                    cache_read: 0.000,
                },
            },
            Model {
                id: uuid::uuid!("01980000-0002-7000-8000-000000000002"),
                name: "grok-4-0709".into(),
                provider: ProviderKind::XAI.into(),
                capabilities: Some(vec![Capability::IMAGE.into()]),
                context_window: 256000,
                pricing: ModelPricing {
                    input: 3.000,
                    output: 15.000,
                    cache_write: 0.000,
                    cache_read: 0.000,
                },
            },
            Model {
                id: uuid::uuid!("01980000-0003-7000-8000-000000000003"),
                name: "grok-3".into(),
                provider: ProviderKind::XAI.into(),
                capabilities: Some(vec![Capability::IMAGE.into()]),
                context_window: 131072,
                pricing: ModelPricing {
                    input: 3.000,
                    output: 15.000,
                    cache_write: 0.000,
                    cache_read: 0.000,
                },
            },
            Model {
                id: uuid::uuid!("01980000-0004-7000-8000-000000000004"),
                name: "grok-3-mini".into(),
                provider: ProviderKind::XAI.into(),
                capabilities: Some(vec![Capability::IMAGE.into()]),
                context_window: 131072,
                pricing: ModelPricing {
                    input: 0.300,
                    output: 0.500,
                    cache_write: 0.000,
                    cache_read: 0.000,
                },
            },
        ]
    }
    /// Placeholder default until the provider catalog is ported.
    pub fn default_model() -> Result<Model, &'static str> {
        Err("model not supported")
    }
}
/// Gemini's complete Go catalog.
pub mod gemini {
    use super::*;
    /// All supported models in Go order, including Go's pricing.
    pub fn supported_models() -> Vec<Model> {
        [
            (
                "01970000-0001-7000-8000-000000000001",
                "gemini-2.5-pro",
                1048576,
                1.125,
                10.0,
            ),
            (
                "01970000-0002-7000-8000-000000000002",
                "gemini-2.5-flash",
                1048576,
                1.25,
                5.0,
            ),
            (
                "01970000-0003-7000-8000-000000000003",
                "gemini-2.5-flash-lite",
                1000000,
                0.075,
                0.3,
            ),
        ]
        .into_iter()
        .map(|(id, name, context_window, input, output)| Model {
            id: uuid::Uuid::parse_str(id).unwrap(),
            name: name.into(),
            provider: ProviderKind::GEMINI.into(),
            capabilities: Some(vec![Capability::IMAGE.into(), Capability::AUDIO.into()]),
            context_window,
            pricing: ModelPricing {
                input,
                output,
                cache_write: 0.0,
                cache_read: 0.0,
            },
        })
        .collect()
    }
    /// Go defaults to Gemini 2.5 Pro.
    pub fn default_model() -> Result<Model, &'static str> {
        Ok(supported_models().remove(0))
    }
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn catalog_matches_go() {
        let models = supported_models(&ProviderKind::ANTHROPIC.into());
        assert_eq!(models.len(), 11);
        assert_eq!(models[0].name, anthropic::PLAN_MODEL);
        assert_eq!(models[1].id, models[2].id);
        assert_eq!(models[7].context_window, 100000);
        assert_eq!(models[10].pricing.cache_read, 0.03);
        assert_eq!(
            default_model(&ProviderKind::ANTHROPIC.into()).unwrap().id,
            models[0].id
        );
<<<<<<< HEAD
        let gemini = supported_models(&ProviderKind::GEMINI.into());
        assert_eq!(gemini.len(), 3);
        assert_eq!(gemini[0].name, "gemini-2.5-pro");
        assert_eq!(gemini[2].context_window, 1000000);
        assert_eq!(gemini[0].pricing.input, 1.125);
        assert_eq!(
            default_model(&ProviderKind::GEMINI.into()).unwrap().id,
            gemini[0].id
        );
        for kind in ["openai", "xai", "deepseek", "bedrock", "unknown"] {
||||||| 25b5fc6
        for kind in ["openai", "gemini", "xai", "deepseek", "bedrock", "unknown"] {
=======
        assert_eq!(openai::supported_models().len(), 7);
        assert_eq!(xai::supported_models().len(), 4);
        for kind in ["openai", "xai"] {
            assert_eq!(
                default_model(&kind.into()).unwrap_err(),
                "model not supported"
            );
        }
        for kind in ["gemini", "deepseek", "bedrock", "unknown"] {
>>>>>>> origin/devin/1795094873-rust-openai-xai
            assert!(supported_models(&kind.into()).is_empty());
            assert_eq!(
                default_model(&kind.into()).unwrap_err(),
                "model not supported"
            );
        }
    }
}
