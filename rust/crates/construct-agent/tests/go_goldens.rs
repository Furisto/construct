use base64::{engine::general_purpose::STANDARD, Engine};
use construct_agent::{analytics::*, conv, environment, prompt, provider::ModelProviderFactory};
use construct_core::{store::ModelProviderStore, testing::MemoryStore};
use construct_secret::{Encryption, Keyset};
use construct_types::{entity, memory, provider as model, to_go_json};
use serde::Deserialize;
use std::sync::Arc;

#[derive(Deserialize)]
struct Conversion {
    model: Option<Vec<model::ContentBlock>>,
    explicit_null_args: Option<bool>,
    stored: String,
    back: String,
}

#[test]
fn go_conversion_bytes() {
    let cases: Vec<Conversion> =
        serde_json::from_str(include_str!("fixtures/conversion.json")).unwrap();
    for mut case in cases {
        if case.explicit_null_args == Some(true) {
            if let model::ContentBlock::ToolCall(call) = &mut case.model.as_mut().unwrap()[0] {
                call.args = Some(serde_json::value::RawValue::from_string("null".into()).unwrap());
            }
        }
        let stored =
            conv::model_blocks_to_memory(case.model.as_deref().unwrap_or_default(), "anthropic")
                .unwrap();
        assert_eq!(to_go_json(&stored).unwrap(), case.stored.as_bytes());
        let from_go: memory::MessageContent = serde_json::from_str(&case.stored).unwrap();
        let back =
            conv::memory_blocks_to_model(from_go.blocks.as_deref().unwrap_or_default()).unwrap();
        assert_eq!(to_go_json(&back).unwrap(), case.back.as_bytes());
        let message = entity::Message {
            source: "assistant".into(),
            content: Some(from_go),
            ..Default::default()
        };
        let model = conv::memory_message_to_model(&message).unwrap();
        assert_eq!(model.source.as_ref(), "model");
        assert_eq!(to_go_json(&model.content).unwrap(), case.back.as_bytes());
        let memory = conv::model_message_to_memory(&model).unwrap();
        assert_eq!(memory.source.as_ref(), "assistant");
    }
}

#[test]
fn go_tool_results_stderr_and_parsed_calls() {
    #[derive(Deserialize)]
    struct Fixture {
        blocks: Vec<memory::MessageBlock>,
        model: Vec<model::ContentBlock>,
    }
    let fixture: Fixture = serde_json::from_str(include_str!("fixtures/tool-result.json")).unwrap();
    let model = conv::memory_blocks_to_model(&fixture.blocks)
        .unwrap()
        .unwrap();
    assert_eq!(
        to_go_json(&model).unwrap(),
        to_go_json(&fixture.model).unwrap()
    );
    let (calls, results) = conv::memory_blocks_to_agent(&fixture.blocks).unwrap();
    assert!(calls.is_empty());
    assert_eq!(results.len(), 1);
    match &model[0] {
        model::ContentBlock::ToolResult(r) => assert_eq!(r.result, "stdout\nfailure"),
        _ => panic!("wrong block"),
    }
}

#[test]
fn go_system_prompt_bytes() {
    let expected: serde_json::Value =
        serde_json::from_str(include_str!("fixtures/prompts.json")).unwrap();
    for full in [false, true] {
        let mut context = prompt::PromptContext {
            working_directory: "/workspace/<demo>".into(),
            operating_system: "linux".into(),
            default_shell: "bash".into(),
            project_structure: "|_Cargo.toml\n|_src\n".into(),
            ..Default::default()
        };
        if full {
            context
                .tools
                .push(("read_file".into(), "Read a file".into()));
            context.dev_tools =
                environment::available_dev_tools_with(|s| ["git", "gh", "cargo"].contains(&s));
        }
        let suffix = if full { "full" } else { "empty" };
        for (name, source) in [("edit", prompt::EDIT), ("plan", prompt::PLAN)] {
            assert_eq!(
                prompt::assemble_system_prompt(source, &context).unwrap(),
                expected[format!("{name}-{suffix}")].as_str().unwrap()
            );
        }
        let skills = [construct_skill::Skill {
            name: "test".into(),
            description: "Description".into(),
            location: "/skills/test/SKILL.md".into(),
            scope: None,
        }];
        context.skills = skills.to_vec();
        assert_eq!(
            prompt::assemble_system_prompt("{{ .AvailableSkills }}", &context).unwrap(),
            construct_skill::format_available_skills(&skills)
        );
        // Go's embedded edit/plan templates do not currently interpolate skills.
        assert!(!prompt::assemble_system_prompt(prompt::EDIT, &context)
            .unwrap()
            .contains("<available_skills>"));
    }
}

#[tokio::test]
async fn every_provider_decrypts_go_secret() {
    #[derive(Deserialize)]
    struct Fixture {
        id: uuid::Uuid,
        keyset: serde_json::Value,
        ciphertext: String,
    }
    let fixture: Fixture = serde_json::from_str(include_str!("fixtures/provider.json")).unwrap();
    let encryption =
        Arc::new(Encryption::new(Keyset::from_json(&fixture.keyset.to_string()).unwrap()).unwrap());
    for kind in ["anthropic", "openai", "xai", "gemini"] {
        let store = Arc::new(MemoryStore::default());
        let mut record = entity::ModelProvider {
            id: fixture.id,
            name: "test".into(),
            provider_type: kind.into(),
            secret: STANDARD.decode(&fixture.ciphertext).unwrap(),
            ..Default::default()
        };
        store.create_model_provider(record.clone()).await.unwrap();
        let factory = ModelProviderFactory::new(store, encryption.clone());
        assert!(factory.create_client(fixture.id).await.is_ok(), "{kind}");
        record.id = uuid::Uuid::from_u128(2);
        assert!(
            factory.create_from_record(&record).is_err(),
            "associated data must bind provider ID"
        );
    }
}

#[test]
fn every_analytics_emitter_matches_go() {
    let client = InMemory::default();
    emit_agent_created(&client, "a", "edit", "claude");
    emit_agent_updated(&client, "a", "edit", Some(&["name".into()]));
    emit_model_provider_created(&client, "p", "provider", "anthropic");
    emit_model_created(&client, "m", "claude", "p", 100);
    emit_task_created(&client, "t", "a");
    emit_task_updated(&client, "t", None);
    emit_agent_deleted(&client, "a", "edit");
    emit_model_provider_deleted(&client, "p", "provider", "anthropic");
    emit_model_deleted(&client, "m", "claude", "p");
    emit_task_deleted(&client, "t", "a");
    emit_message_created(&client, "msg", "t", "user");
    emit_message_deleted(&client, "msg", "t", "user");
    emit_session_started(&client, "new", "edit", "/workspace", true);
    emit_conversation_resumed(&client, "t", 2, 3);
    emit_quick_ask(&client, "edit", 4, 5, true, 6);
    emit_tool_executed(&client, "t", "read_file", 42, false, "missing");
    emit_code_interpreter_session(&client, "t", 3, 1, 20);
    emit_agent_handoff(&client, "a", "b", "t", "specialization", 2);
    let go: Vec<serde_json::Value> =
        serde_json::from_str(include_str!("fixtures/analytics.json")).unwrap();
    let rust = client.events();
    assert_eq!(go.len(), rust.len());
    for (go, rust) in go.iter().zip(&rust) {
        assert_eq!(go["DistinctId"], rust.distinct_id);
        assert_eq!(go["Event"], rust.event);
        assert_eq!(go["Properties"], rust.properties);
    }
}
