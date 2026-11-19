use async_trait::async_trait;
use construct_core::{event::*, model::*, secret::*, store::*, tool::*, CancellationToken};
use construct_types::{
    provider::{Message, ProviderError},
    tools::ToolOutput,
};
use std::sync::Arc;

struct StubTool;
#[async_trait]
impl Tool for StubTool {
    fn name(&self) -> &str {
        "stub"
    }
    fn description(&self) -> &str {
        "stub"
    }
    fn input_schema(&self) -> serde_json::Value {
        serde_json::json!({"type":"object"})
    }
    async fn execute(
        &self,
        ctx: &ToolContext,
        _: serde_json::Value,
    ) -> Result<ToolOutput, ToolError> {
        if ctx.cancellation.is_cancelled() {
            return Err(ToolError::custom("cancelled", vec![]));
        }
        Ok(ToolOutput::default())
    }
}
struct StubProvider;
#[async_trait]
impl ModelProvider for StubProvider {
    async fn invoke_model(
        &self,
        _: &str,
        _: &str,
        _: &[Message],
        _: InvokeOptions,
    ) -> Result<Message, ProviderError> {
        Ok(Message::default())
    }
}

// These type checks fail at compilation if any contract stops being dyn-safe.
#[test]
fn repository_traits_are_dyn_safe() {
    let _: Option<Arc<dyn Store>> = None;
    let _: Option<Arc<dyn AgentStore>> = None;
    let _: Option<Arc<dyn TaskStore>> = None;
    let _: Option<Arc<dyn MessageStore>> = None;
    let _: Option<Arc<dyn ModelStore>> = None;
    let _: Option<Arc<dyn ModelProviderStore>> = None;
    let _: Option<Arc<dyn TokenStore>> = None;
    let _: Option<Box<dyn StoreTransaction>> = None;
    let _: Option<Arc<dyn SecretProvider>> = None;
}

#[tokio::test]
async fn dyn_tool_provider_and_event_contracts() {
    let tool: Arc<dyn Tool> = Arc::new(StubTool);
    let provider: Arc<dyn ModelProvider> = Arc::new(StubProvider);
    let router = Arc::new(EventRouter::default());
    let publisher: Arc<dyn EventPublisher> = router.clone();
    let subscriber: Arc<dyn EventSubscriber> = router;
    let _sub = subscriber.subscribe(Default::default());
    let ctx = ToolContext {
        workspace_root: "/workspace".into(),
        task_id: None,
        agent_id: None,
        cancellation: CancellationToken::new(),
        events: Some(publisher),
    };
    assert_eq!(tool.name(), "stub");
    assert_eq!(tool.description(), "stub");
    assert!(tool.input_schema().is_object());
    assert!(tool.execute(&ctx, serde_json::json!({})).await.is_ok());
    ctx.cancellation.cancel();
    assert!(tool.execute(&ctx, serde_json::json!({})).await.is_err());
    assert!(provider
        .invoke_model("stub", "", &[], InvokeOptions::default())
        .await
        .is_ok());
}

#[test]
fn profile_validation_defaults_and_go_json_tags() {
    let mut profile = ModelProfile::Anthropic(AnthropicProfile {
        enable_prompt_caching: true,
        enable_computer_use: true,
        ..Default::default()
    });
    profile.validate().unwrap();
    profile.validate().unwrap();
    assert_eq!(profile.kind().as_ref(), "anthropic");
    let ModelProfile::Anthropic(p) = profile else {
        unreachable!()
    };
    assert_eq!(p.timeout, 60_000_000_000);
    assert_eq!(p.anthropic_version, "2024-01-01");
    assert_eq!(p.anthropic_beta.len(), 2);
    assert_eq!(
        construct_types::to_go_json(&AnthropicProfile::default()).unwrap(),
        b"{}"
    );
    assert_eq!(
        serde_json::to_value(GeminiProfile {
            default_max_tokens: Some(0),
            ..Default::default()
        })
        .unwrap(),
        serde_json::json!({"default_max_tokens":0})
    );
    let mut p = ModelProfile::OpenAI(OpenAIProfile::default());
    p.validate().unwrap();
    let ModelProfile::OpenAI(p) = p else {
        unreachable!()
    };
    assert_eq!(p.timeout, 30_000_000_000);
    assert_eq!(p.max_retries, 3);
    assert_eq!(p.api_url, "https://api.openai.com/v1");
    for mut p in [
        ModelProfile::Anthropic(AnthropicProfile {
            temperature: 1.1,
            ..Default::default()
        }),
        ModelProfile::Anthropic(AnthropicProfile {
            top_k: -1,
            ..Default::default()
        }),
        ModelProfile::OpenAI(OpenAIProfile {
            frequency_penalty: 3.0,
            ..Default::default()
        }),
        ModelProfile::OpenAI(OpenAIProfile {
            presence_penalty: -3.0,
            ..Default::default()
        }),
        ModelProfile::Gemini(GeminiProfile {
            default_top_p: Some(2.0),
            ..Default::default()
        }),
        ModelProfile::Gemini(GeminiProfile {
            default_max_tokens: Some(-1),
            ..Default::default()
        }),
    ] {
        assert!(p.validate().is_err());
    }
}

#[test]
fn secret_keys_and_go_tool_error_kinds() {
    assert_eq!(
        model_provider_secret_key(uuid::Uuid::nil()),
        "model_provider:00000000-0000-0000-0000-000000000000"
    );
    assert_eq!(ENCRYPTION_KEY_SECRET, "encryption_key");
    assert_eq!(ToolErrorCode::InvalidArgument as i32, 11);
    let error = ToolError::new(ToolErrorCode::FileNotFound);
    assert_eq!(error.message, "File not found");
    assert_eq!(error.suggestions[0], GENERIC_SUGGESTION);
    assert!(error.to_string().contains("Ensure the file exists"));
    let error = ToolError::custom("custom", vec!["retry".into()]);
    assert_eq!(error.suggestions, vec!["retry", GENERIC_SUGGESTION]);
}
