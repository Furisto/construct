//! Anthropic-only conversation title generation, independent of runtime scheduling.
use crate::{conv, Result};
use construct_core::{
    model::{InvokeOptions, ModelProvider},
    store::{ListOptions, MessageFilter, SortField, SortOrder, Store},
};
use construct_types::{
    entity,
    provider::{ContentBlock, Message, TextBlock},
};
use uuid::Uuid;

pub const MAX_TITLE_LENGTH: usize = 80;
pub const MAX_RETRIES: usize = 2;
pub const SYSTEM_PROMPT: &str = r#"You are a title generator for development tasks and conversations. Generate a concise, descriptive title based on the user's request and conversation context.

CRITICAL RULES:
- Maximum 8 words
- Maximum 80 characters
- Start immediately with the title (no preamble)
- Use action verbs when describing tasks
- Be specific about technology/domain when relevant
- NO quotes, markdown, or extra punctuation
- NO meta-commentary or explanations

GOOD EXAMPLES:
- "Implement JWT authentication for API"
- "Fix memory leak in worker pool"
- "Add TypeScript support to build"
- "Debug race condition in cache"
- "Refactor database connection pooling"

BAD EXAMPLES (too vague):
- "Help with code"
- "Fix issue"
- "Question about app"

BAD EXAMPLES (too long):
- "Implement a comprehensive user authentication system with JWT tokens and refresh functionality"

For the given conversation, generate ONLY the title starting with a quote:"#;

fn text_message(source: &str, text: &str) -> Message {
    Message {
        source: source.into(),
        content: Some(vec![ContentBlock::Text(TextBlock { text: text.into() })]),
        ..Default::default()
    }
}

pub async fn generate_title(
    provider: &dyn ModelProvider,
    provider_kind: &str,
    messages: &[entity::Message],
) -> Result<Option<String>> {
    let messages = &messages[..messages.len().min(5)];
    if !messages.iter().any(|m| m.source.as_ref() == "user") {
        return Ok(None);
    }
    let mut history = messages
        .iter()
        .filter(|m| ["user", "assistant"].contains(&m.source.as_ref()))
        .map(conv::memory_message_to_model)
        .collect::<Result<Vec<_>>>()?;
    if provider_kind != "anthropic" {
        return Err("provider is not an Anthropic provider".into());
    }
    for attempt in 0..=MAX_RETRIES {
        let mut request = history.clone();
        request.push(text_message("model", "\""));
        let response = provider
            .invoke_model(
                construct_model::catalog::anthropic::BUDGET_MODEL,
                SYSTEM_PROMPT,
                &request,
                InvokeOptions::default(),
            )
            .await?;
        let title = extract_title(response.content.as_deref().unwrap_or_default());
        if title.is_empty() {
            return Err("model returned empty title".into());
        }
        if title.len() <= MAX_TITLE_LENGTH {
            return Ok(Some(title));
        }
        if attempt == MAX_RETRIES {
            let mut end = MAX_TITLE_LENGTH - 3;
            while !title.is_char_boundary(end) {
                end -= 1;
            }
            return Ok(Some(title[..end].to_owned() + "..."));
        }
        history.push(text_message("model", &title));
        history.push(text_message("user", "That title is too long. Please provide a shorter version (max 8 words, ~80 characters)."));
    }
    unreachable!()
}

pub fn extract_title(blocks: &[ContentBlock]) -> String {
    blocks
        .iter()
        .find_map(|b| match b {
            ContentBlock::Text(t) => Some(
                t.text
                    .trim_matches(['"', ' ', '\n', '\t', '\r', '\'', '`'])
                    .trim()
                    .to_owned(),
            ),
            _ => None,
        })
        .unwrap_or_default()
}

pub struct TitleGenerator {
    pub store: std::sync::Arc<dyn Store>,
    pub provider_factory: std::sync::Arc<crate::provider::ModelProviderFactory>,
}
impl TitleGenerator {
    pub async fn generate_title(&self, task_id: Uuid) -> Result<()> {
        let task = self
            .store
            .get_task(task_id)
            .await
            .map_err(|e| format!("failed to fetch task: {e}"))?;
        let agent_id = task.agent_id.ok_or_else(|| {
            format!("failed to fetch task: no agent associated with task: {task_id}")
        })?;
        let agent = self
            .store
            .get_agent(agent_id)
            .await
            .map_err(|e| format!("failed to fetch task: {e}"))?;
        let model = self
            .store
            .get_model(agent.model_id.ok_or("agent has no model")?)
            .await
            .map_err(|e| format!("failed to fetch task: {e}"))?;
        let messages = self
            .store
            .list_messages(
                MessageFilter {
                    task_id: Some(task_id),
                    ..Default::default()
                },
                ListOptions {
                    limit: Some(5),
                    sort_field: SortField::CreatedAt,
                    sort_order: SortOrder::Ascending,
                    ..Default::default()
                },
            )
            .await
            .map_err(|e| format!("failed to fetch messages: {e}"))?;
        if !messages.iter().any(|m| m.source.as_ref() == "user") {
            return Ok(());
        }
        let record = self
            .store
            .get_model_provider(model.model_provider_id)
            .await?;
        let provider = self
            .provider_factory
            .create_client(model.model_provider_id)
            .await
            .map_err(|e| format!("failed to create model provider: {e}"))?;
        if let Some(title) =
            generate_title(provider.as_ref(), record.provider_type.as_ref(), &messages)
                .await
                .map_err(|e| format!("failed to generate title: {e}"))?
        {
            let transaction = self.store.begin().await?;
            let mut task = transaction.get_task(task_id).await?;
            task.description = title;
            task.update_time = construct_types::Timestamp(std::time::SystemTime::now().into());
            transaction
                .update_task(task)
                .await
                .map_err(|e| format!("failed to save title: {e}"))?;
            transaction
                .commit()
                .await
                .map_err(|e| format!("failed to save title: {e}"))?;
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use construct_core::testing::{ModelStep, ScriptedModelProvider};
    fn scripted(titles: &[&str]) -> ScriptedModelProvider {
        ScriptedModelProvider::new(titles.iter().map(|t| ModelStep {
            expected_model: Some(construct_model::catalog::anthropic::BUDGET_MODEL.into()),
            chunks: vec![],
            retries: vec![],
            result: Ok(text_message("model", t)),
        }))
    }
    fn user() -> entity::Message {
        entity::Message {
            source: "user".into(),
            ..Default::default()
        }
    }
    #[tokio::test]
    async fn title_retry_and_truncation() {
        let long = "a".repeat(81);
        let provider = scripted(&[&long, " 'Fix Rust'\n"]);
        assert_eq!(
            generate_title(&provider, "anthropic", &[user()])
                .await
                .unwrap()
                .unwrap(),
            "Fix Rust"
        );
        let calls = provider.calls();
        assert_eq!(calls.len(), 2);
        assert_eq!(calls[0].system_prompt, SYSTEM_PROMPT);
        assert_eq!(
            extract_title(calls[0].messages.last().unwrap().content.as_ref().unwrap()),
            ""
        );
        assert_eq!(calls[1].messages.len(), calls[0].messages.len() + 2);
        let provider = scripted(&[&long, &long, &long]);
        assert_eq!(
            generate_title(&provider, "anthropic", &[user()])
                .await
                .unwrap()
                .unwrap(),
            "a".repeat(77) + "..."
        );
        assert_eq!(provider.calls().len(), 3);
        let unicode = "é".repeat(50);
        let provider = scripted(&[&unicode, &unicode, &unicode]);
        let title = generate_title(&provider, "anthropic", &[user()])
            .await
            .unwrap()
            .unwrap();
        assert_eq!(title, "é".repeat(38) + "...");
        assert!(title.len() <= MAX_TITLE_LENGTH);
    }
    #[tokio::test]
    async fn empty_missing_user_wrong_provider_and_error() {
        let provider = scripted(&[]);
        assert!(generate_title(&provider, "anthropic", &[])
            .await
            .unwrap()
            .is_none());
        let mut messages = vec![entity::Message::default(); 5];
        messages.push(user());
        assert!(generate_title(&provider, "anthropic", &messages)
            .await
            .unwrap()
            .is_none());
        assert!(generate_title(&provider, "openai", &[user()])
            .await
            .unwrap_err()
            .to_string()
            .contains("not an Anthropic"));
        assert!(
            generate_title(&scripted(&["\"\" "]), "anthropic", &[user()])
                .await
                .unwrap_err()
                .to_string()
                .contains("empty title")
        );
        assert!(generate_title(&provider, "anthropic", &[user()])
            .await
            .is_err());
    }
}
