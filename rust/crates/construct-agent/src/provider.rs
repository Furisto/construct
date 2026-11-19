//! Store-backed model provider factory using Go's Tink associated data.
use crate::Result;
use construct_core::{model::ModelProvider, store::ModelProviderStore};
use construct_model::{
    anthropic::AnthropicProvider, gemini::GeminiProvider, openai::OpenAICompletionProvider,
    ProviderOptions,
};
use construct_secret::{model_provider_associated, Encryption};
use std::sync::Arc;
use uuid::Uuid;

/// Injectable factory for provider invocations, including deterministic scripts.
#[construct_core::async_trait]
pub trait ProviderFactory: Send + Sync {
    async fn create(&self, id: Uuid) -> Result<Arc<dyn ModelProvider>>;
}

#[construct_core::async_trait]
impl ProviderFactory for ModelProviderFactory {
    async fn create(&self, id: Uuid) -> Result<Arc<dyn ModelProvider>> {
        self.create_client(id).await.map(Arc::from)
    }
}

pub struct ModelProviderFactory {
    pub store: Arc<dyn ModelProviderStore>,
    pub encryption: Arc<Encryption>,
}

impl ModelProviderFactory {
    pub fn new(store: Arc<dyn ModelProviderStore>, encryption: Arc<Encryption>) -> Self {
        Self { store, encryption }
    }

    pub async fn create_client(&self, id: Uuid) -> Result<Box<dyn ModelProvider>> {
        let record = self
            .store
            .get_model_provider(id)
            .await
            .map_err(|e| format!("failed to fetch model provider: {e}"))?;
        self.create_from_record(&record)
    }

    pub fn create_from_record(
        &self,
        record: &construct_types::entity::ModelProvider,
    ) -> Result<Box<dyn ModelProvider>> {
        let plaintext = self
            .encryption
            .decrypt(&record.secret, &model_provider_associated(record.id))
            .map_err(|e| format!("failed to decrypt model provider secret: {e}"))?;
        #[derive(Default, serde::Deserialize)]
        #[serde(default)]
        struct Auth {
            #[serde(rename = "apiKey")]
            api_key: Option<String>,
        }
        let auth: Option<Auth> = serde_json::from_slice(&plaintext)
            .map_err(|e| format!("failed to unmarshal model provider auth: {e}"))?;
        let key = auth.unwrap_or_default().api_key.unwrap_or_default();
        let options = |name| {
            let mut options = ProviderOptions::new(name);
            options.url = record.url.clone();
            options
        };
        let provider: std::result::Result<Box<dyn ModelProvider>, _> = match record
            .provider_type
            .as_ref()
        {
            "anthropic" => {
                AnthropicProvider::new(key, options("anthropic")).map(|p| Box::new(p) as _)
            }
            "openai" => {
                OpenAICompletionProvider::new(key, options("openai")).map(|p| Box::new(p) as _)
            }
            "gemini" => {
                GeminiProvider::with_options(key, options("gemini")).map(|p| Box::new(p) as _)
            }
            "xai" => {
                let mut options = options("openai");
                if options.url.is_empty() {
                    options.url = "https://api.xai.com/v1".into();
                }
                OpenAICompletionProvider::new(key, options).map(|p| Box::new(p) as _)
            }
            _ => {
                return Err(format!("unknown model provider type: {}", record.provider_type).into())
            }
        };
        provider.map_err(|e| format!("failed to create Anthropic provider: {e}").into())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::{extract::OriginalUri, http::StatusCode, routing::post, Router};
    use construct_core::{model::InvokeOptions, store::ModelProviderStore, testing::MemoryStore};
    use construct_secret::Keyset;
    use construct_types::{
        entity::ModelProvider as Record,
        provider::{ContentBlock, Message, TextBlock},
    };
    use std::sync::Mutex;

    #[tokio::test]
    async fn persisted_provider_urls_are_used_for_requests() {
        let paths = Arc::new(Mutex::new(Vec::new()));
        let recorded = paths.clone();
        let app = Router::new().fallback(post(move |uri: OriginalUri| {
            let recorded = recorded.clone();
            async move {
                recorded.lock().unwrap().push(uri.path().to_owned());
                (StatusCode::UNAUTHORIZED, "fixture rejects credentials")
            }
        }));
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let address = listener.local_addr().unwrap();
        let server = tokio::spawn(async move { axum::serve(listener, app).await.unwrap() });
        let encryption = Arc::new(Encryption::new(Keyset::generate()).unwrap());
        let factory =
            ModelProviderFactory::new(Arc::new(MemoryStore::default()), encryption.clone());
        let id = Uuid::new_v4();
        let secret = encryption
            .encrypt(br#"{"apiKey":"fixture"}"#, &model_provider_associated(id))
            .unwrap();
        for (kind, prefix, expected_path) in [
            ("anthropic", "", "/v1/messages"),
            ("openai", "/v1", "/v1/chat/completions"),
            ("xai", "/v1", "/v1/chat/completions"),
            ("gemini", "", "/v1beta/models/fixture:streamGenerateContent"),
        ] {
            let record = Record {
                id,
                secret: secret.clone(),
                provider_type: kind.into(),
                url: format!("http://{address}{prefix}"),
                ..Default::default()
            };
            let result = factory
                .create_from_record(&record)
                .unwrap()
                .invoke_model(
                    "fixture",
                    "system",
                    &[Message {
                        source: "user".into(),
                        content: Some(vec![ContentBlock::Text(TextBlock {
                            text: "hello".into(),
                        })]),
                        ..Default::default()
                    }],
                    InvokeOptions::default(),
                )
                .await;
            assert!(result.is_err());
            assert_eq!(
                paths.lock().unwrap().pop().as_deref(),
                Some(expected_path),
                "{kind}"
            );
        }
        server.abort();
    }
    #[tokio::test]
    async fn store_and_failures() {
        let store = Arc::new(MemoryStore::default());
        let encryption = Arc::new(Encryption::new(Keyset::generate()).unwrap());
        let factory = ModelProviderFactory::new(store.clone(), encryption.clone());
        assert!(factory
            .create_client(Uuid::nil())
            .await
            .unwrap_err_string()
            .contains("failed to fetch model provider"));
        let mut record = Record {
            id: Uuid::from_u128(1),
            name: "test".into(),
            provider_type: "anthropic".into(),
            ..Default::default()
        };
        assert!(factory.create_from_record(&record).is_err());
        for (plain, expected) in [
            ("{", "failed to unmarshal"),
            ("{}", "failed to create Anthropic"),
            (r#"{"apiKey":"test"}"#, "unknown model provider type"),
        ] {
            record.secret = encryption
                .encrypt(plain.as_bytes(), &model_provider_associated(record.id))
                .unwrap();
            if plain.contains("test") {
                record.provider_type = "unsupported".into();
            }
            assert!(factory
                .create_from_record(&record)
                .unwrap_err_string()
                .contains(expected));
        }
        record.provider_type = "anthropic".into();
        store.create_model_provider(record.clone()).await.unwrap();
        assert!(factory.create_client(record.id).await.is_ok());
    }
    trait ErrorString {
        fn unwrap_err_string(self) -> String;
    }
    impl<T> ErrorString for Result<T> {
        fn unwrap_err_string(self) -> String {
            match self {
                Err(e) => e.to_string(),
                Ok(_) => panic!("expected error"),
            }
        }
    }
}
