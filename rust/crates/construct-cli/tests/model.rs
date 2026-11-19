mod common;
use common::{Fake, Harness};
use construct_connect::{Code, ConnectError};
use construct_proto::construct::v1::*;
use serde_json::{json, Value};

const P: &str = "01974c1d-0be8-70e1-88b4-ad9462fff251";
const Q: &str = "01974c1d-0be8-70e1-88b4-ad9462fff252";
const M: &str = "01974c1d-0be8-70e1-88b4-ad9462fff253";
const N: &str = "01974c1d-0be8-70e1-88b4-ad9462fff254";

fn provider(id: &str, name: &str, kind: ModelProviderType) -> ModelProvider {
    ModelProvider {
        metadata: Some(ModelProviderMetadata {
            id: id.into(),
            provider_type: kind as i32,
            ..Default::default()
        }),
        spec: Some(ModelProviderSpec {
            name: name.into(),
            enabled: true,
        }),
    }
}
fn model(id: &str, name: &str, provider: &str, window: i64, enabled: bool) -> Model {
    Model {
        metadata: Some(ModelMetadata {
            id: id.into(),
            model_provider_id: provider.into(),
            ..Default::default()
        }),
        spec: Some(ModelSpec {
            name: name.into(),
            context_window: window,
            enabled,
            capabilities: vec![ModelCapability::Image as i32],
            ..Default::default()
        }),
    }
}
fn model_lookup_request(name: &str) -> ListModelsRequest {
    ListModelsRequest {
        filter: Some(list_models_request::Filter {
            names: vec![name.into()],
            ..Default::default()
        }),
        ..Default::default()
    }
}
fn model_list_request(provider: Option<&str>, enabled: Option<bool>) -> ListModelsRequest {
    ListModelsRequest {
        filter: Some(list_models_request::Filter {
            model_provider_id: provider.map(str::to_owned),
            enabled,
            ..Default::default()
        }),
        ..Default::default()
    }
}
fn provider_list_request(types: &[ModelProviderType]) -> ListModelProvidersRequest {
    ListModelProvidersRequest {
        filter: Some(list_model_providers_request::Filter {
            provider_types: types.iter().map(|t| *t as i32).collect(),
            ..Default::default()
        }),
        ..Default::default()
    }
}
fn provider_lookup(fake: &Fake, name: &str, id: &str) {
    fake.expect(
        "list_model_providers",
        ListModelProvidersRequest::default(),
        ListModelProvidersResponse {
            model_providers: vec![provider(id, name, ModelProviderType::Openai)],
            ..Default::default()
        },
    );
}
fn model_lookup(fake: &Fake, name: &str, id: &str) {
    fake.expect(
        "list_models",
        model_lookup_request(name),
        ListModelsResponse {
            models: vec![model(id, name, P, 8192, true)],
            ..Default::default()
        },
    );
}
fn model_get(fake: &Fake, item: Model) {
    fake.expect(
        "get_model",
        GetModelRequest {
            id: item.metadata.as_ref().unwrap().id.clone(),
        },
        GetModelResponse { model: Some(item) },
    );
}
fn provider_get(fake: &Fake, item: ModelProvider) {
    fake.expect(
        "get_model_provider",
        GetModelProviderRequest {
            id: item.metadata.as_ref().unwrap().id.clone(),
        },
        GetModelProviderResponse {
            model_provider: Some(item),
        },
    );
}
fn model_display(
    id: &str,
    name: &str,
    provider: &str,
    window: i64,
    enabled: bool,
    yaml: bool,
) -> Value {
    if yaml {
        json!({"id": id, "name": name, "modelprovider": provider, "contextwindow": window, "enabled": enabled, "capabilities": ["MODEL_CAPABILITY_IMAGE"]})
    } else {
        json!({"id": id, "name": name, "model_provider": provider, "context_window": window, "enabled": enabled, "capabilities": ["MODEL_CAPABILITY_IMAGE"]})
    }
}
fn provider_display(id: &str, name: &str, kind: &str, yaml: bool) -> Value {
    if yaml {
        json!({"id":id,"name":name,"providertype":kind,"enabled":true})
    } else {
        json!({"id":id,"name":name,"provider_type":kind,"enabled":true})
    }
}

#[tokio::test]
async fn model_create_by_provider_name_id_and_short_flags() {
    for (name, provider_name, window, short) in [
        ("gpt-4", "openai-dev", 8192, false),
        ("claude-3-5-sonnet", P, 200000, false),
        ("llama-3.1-8b", "ollama-local", 32768, true),
    ] {
        let mut h = Harness::new().await;
        if provider_name != P {
            provider_lookup(&h.fake, provider_name, P);
        }
        h.fake.expect(
            "create_model",
            CreateModelRequest {
                name: name.into(),
                model_provider_id: P.into(),
                context_window: window,
                ..Default::default()
            },
            CreateModelResponse {
                model: Some(model(M, name, P, window, true)),
            },
        );
        h.run(
            "model",
            &[
                "create",
                name,
                if short { "-p" } else { "--provider" },
                provider_name,
                if short { "-w" } else { "--context-window" },
                &window.to_string(),
            ],
        )
        .await
        .unwrap();
        assert_eq!(h.stdout(), format!("{M}\n"));
        h.fake.done();
    }
}
#[tokio::test]
async fn model_create_errors() {
    for stage in ["missing", "create", "lookup"] {
        let mut h = Harness::new().await;
        let provider_name = match stage {
            "missing" => "nonexistent",
            "lookup" => "openai-dev",
            _ => P,
        };
        let error = match stage {
            "missing" => {
                h.fake.expect(
                    "list_model_providers",
                    ListModelProvidersRequest::default(),
                    ListModelProvidersResponse::default(),
                );
                "failed to resolve model provider nonexistent: model provider nonexistent not found"
            }
            "lookup" => {
                h.fake
                    .fail("list_model_providers", ListModelProvidersRequest::default());
                "failed to resolve model provider openai-dev: failed to list model providers: internal"
            }
            _ => {
                h.fake.fail(
                    "create_model",
                    CreateModelRequest {
                        name: "test-model".into(),
                        model_provider_id: P.into(),
                        context_window: 4096,
                        ..Default::default()
                    },
                );
                "failed to create model: internal"
            }
        };
        assert_eq!(
            h.run(
                "model",
                &[
                    "create",
                    "test-model",
                    "--provider",
                    provider_name,
                    "--context-window",
                    "4096"
                ]
            )
            .await
            .unwrap_err()
            .to_string(),
            error
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn model_get_name_id_json_yaml() {
    for (input, name, window, enabled, format) in [
        ("gpt-4", "gpt-4", 8192, true, "card"),
        (M, "claude-3-5-sonnet", 200000, false, "card"),
        ("llama-3.1-8b", "llama-3.1-8b", 32768, true, "json"),
        (
            "claude-3-5-sonnet",
            "claude-3-5-sonnet",
            200000,
            true,
            "yaml",
        ),
    ] {
        let mut h = Harness::new().await;
        if input != M {
            model_lookup(&h.fake, input, M);
        }
        model_get(&h.fake, model(M, name, P, window, enabled));
        let args = if format == "card" {
            vec!["get", input]
        } else {
            vec!["get", input, "--output", format]
        };
        h.run("model", &args).await.unwrap();
        match format {
            "json" => assert_eq!(
                serde_json::from_str::<Value>(&h.stdout()).unwrap(),
                model_display(M, name, P, window, enabled, false)
            ),
            "yaml" => assert_eq!(
                serde_yaml::from_str::<Value>(&h.stdout()).unwrap(),
                model_display(M, name, P, window, enabled, true)
            ),
            _ => {
                assert_eq!(h.stdout(), format!("Id:            {M}\nName:          {name}\nModelProvider: {P}\nContextWindow: {window}\n"));
            }
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn model_get_errors() {
    for stage in ["missing", "get", "lookup"] {
        let mut h = Harness::new().await;
        let (name, error) = match stage {
            "missing" => {
                h.fake.expect(
                    "list_models",
                    model_lookup_request("nonexistent"),
                    ListModelsResponse::default(),
                );
                (
                    "nonexistent",
                    "failed to resolve model nonexistent: model nonexistent not found".into(),
                )
            }
            "get" => {
                h.fake.fail_with(
                    "get_model",
                    GetModelRequest { id: M.into() },
                    ConnectError::new(Code::NotFound, ""),
                );
                (M, format!("failed to get model {M}: not_found"))
            }
            _ => {
                h.fake.fail("list_models", model_lookup_request("gpt-4"));
                (
                    "gpt-4",
                    "failed to resolve model gpt-4: failed to list models: internal".into(),
                )
            }
        };
        assert_eq!(
            h.run("model", &["get", name])
                .await
                .unwrap_err()
                .to_string(),
            error
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn model_list_enabled_provider_all_json_short_yaml_empty() {
    for case in [
        "enabled",
        "provider-name",
        "provider-id",
        "all",
        "json",
        "short",
        "empty",
    ] {
        let mut h = Harness::new().await;
        let mut args = vec!["list"];
        let provider_filter = match case {
            "provider-name" => {
                args.extend(["--provider", "openai-dev"]);
                provider_lookup(&h.fake, "openai-dev", P);
                Some(P)
            }
            "provider-id" => {
                args.extend(["--provider", P]);
                Some(P)
            }
            "short" => {
                args.extend(["-p", "anthropic-dev", "-a", "-o", "yaml"]);
                provider_lookup(&h.fake, "anthropic-dev", Q);
                Some(Q)
            }
            _ => None,
        };
        if case == "all" {
            args.push("--all");
        }
        if case == "json" {
            args.extend(["--output", "json"]);
        }
        let items = match case {
            "empty" => vec![],
            "all" => vec![
                model(M, "gpt-4", P, 8192, true),
                model(N, "claude-3-5-sonnet", Q, 200000, false),
            ],
            "short" => vec![model(N, "claude-3-5-sonnet", Q, 200000, true)],
            _ => vec![model(M, "gpt-4", P, 8192, true)],
        };
        h.fake.expect(
            "list_models",
            model_list_request(
                provider_filter,
                if matches!(case, "all" | "short") {
                    None
                } else {
                    Some(true)
                },
            ),
            ListModelsResponse {
                models: items,
                ..Default::default()
            },
        );
        h.run("model", &args).await.unwrap();
        match case {
            "empty" => assert_eq!(h.stdout(), ""),
            "short" => assert_eq!(
                serde_yaml::from_str::<Value>(&h.stdout()).unwrap(),
                json!([model_display(N, "claude-3-5-sonnet", Q, 200000, true, true)])
            ),
            "json" => assert_eq!(
                serde_json::from_str::<Value>(&h.stdout()).unwrap(),
                json!([model_display(M, "gpt-4", P, 8192, true, false)])
            ),
            _ => {
                assert!(h.stdout().contains("ContextWindow"));
                assert!(h.stdout().contains("gpt-4"));
                if case == "all" {
                    assert!(h.stdout().contains("claude-3-5-sonnet"));
                }
            }
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn model_list_errors() {
    let mut h = Harness::new().await;
    h.fake.expect(
        "list_model_providers",
        ListModelProvidersRequest::default(),
        ListModelProvidersResponse::default(),
    );
    assert_eq!(
        h.run("model", &["list", "--provider", "nonexistent"])
            .await
            .unwrap_err()
            .to_string(),
        "failed to resolve model provider nonexistent: model provider nonexistent not found"
    );
    h.fake.done();
    let mut h = Harness::new().await;
    h.fake
        .fail("list_models", model_list_request(None, Some(true)));
    assert_eq!(
        h.run("model", &["list"]).await.unwrap_err().to_string(),
        "failed to list models: internal"
    );
    h.fake.done();
}
#[tokio::test]
async fn model_delete_names_ids_mixed_force_confirm_cancel() {
    for (names, force, input, confirmed) in [
        (vec!["gpt-4"], true, "", true),
        (vec![M], true, "", true),
        (vec!["gpt-4", "claude-3-5-sonnet"], true, "", true),
        (vec![M, "llama-3.1-8b"], true, "", true),
        (vec!["gpt-4"], false, "y\n", true),
        (vec!["gpt-4"], false, "n\n", false),
        (vec!["gpt-4", "claude-3-5-sonnet"], false, "y\n", true),
    ] {
        let mut h = Harness::new().await;
        h.stdin(input);
        for (index, name) in names.iter().enumerate() {
            if *name != M {
                model_lookup(&h.fake, name, if index == 0 { M } else { N });
            }
        }
        if confirmed {
            for index in 0..names.len() {
                h.fake.expect(
                    "delete_model",
                    DeleteModelRequest {
                        id: if index == 0 { M } else { N }.into(),
                    },
                    DeleteModelResponse {},
                );
            }
        }
        let mut args = vec!["delete"];
        if force {
            args.push("--force");
        }
        args.extend(&names);
        h.run("model", &args).await.unwrap();
        assert_eq!(
            h.stdout(),
            if force {
                String::new()
            } else {
                format!(
                    "Are you sure you want to delete model{} {}? (y/n): ",
                    if names.len() > 1 { "s" } else { "" },
                    names.join(" ")
                )
            }
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn model_delete_errors() {
    for stage in ["missing", "delete", "second", "lookup"] {
        let mut h = Harness::new().await;
        let (names, error) = match stage {
            "missing" => {
                h.fake.expect(
                    "list_models",
                    model_lookup_request("nonexistent"),
                    ListModelsResponse::default(),
                );
                (
                    vec!["nonexistent"],
                    "failed to resolve model nonexistent: model nonexistent not found".into(),
                )
            }
            "lookup" => {
                h.fake.fail("list_models", model_lookup_request("gpt-4"));
                (
                    vec!["gpt-4"],
                    "failed to resolve model gpt-4: failed to list models: internal".into(),
                )
            }
            "second" => {
                model_lookup(&h.fake, "gpt-4", M);
                model_lookup(&h.fake, "claude-3-5-sonnet", N);
                h.fake.expect(
                    "delete_model",
                    DeleteModelRequest { id: M.into() },
                    DeleteModelResponse {},
                );
                h.fake
                    .fail("delete_model", DeleteModelRequest { id: N.into() });
                (
                    vec!["gpt-4", "claude-3-5-sonnet"],
                    "failed to delete model claude-3-5-sonnet: internal".into(),
                )
            }
            _ => {
                h.fake.fail_with(
                    "delete_model",
                    DeleteModelRequest { id: M.into() },
                    ConnectError::new(Code::NotFound, ""),
                );
                (vec![M], format!("failed to delete model {M}: not_found"))
            }
        };
        let mut args = vec!["delete", "--force"];
        args.extend(names);
        assert_eq!(h.run("model", &args).await.unwrap_err().to_string(), error);
        h.fake.done();
    }
}

fn create_provider_request(
    name: &str,
    kind: ModelProviderType,
    key: &str,
) -> CreateModelProviderRequest {
    CreateModelProviderRequest {
        name: name.into(),
        provider_type: kind as i32,
        authentication: Some(create_model_provider_request::Authentication::ApiKey(
            key.into(),
        )),
        ..Default::default()
    }
}
struct Environment(&'static str, Option<std::ffi::OsString>);
impl Environment {
    fn set(name: &'static str, value: &str) -> Self {
        let old = std::env::var_os(name);
        std::env::set_var(name, value);
        Self(name, old)
    }
}
impl Drop for Environment {
    fn drop(&mut self) {
        if let Some(value) = &self.1 {
            std::env::set_var(self.0, value);
        } else {
            std::env::remove_var(self.0);
        }
    }
}
#[tokio::test]
async fn provider_create_flag_environment_precedence_all_types_and_noninteractive_prompt() {
    for (kind, kind_name, env, display) in [
        (
            ModelProviderType::Openai,
            "openai",
            "OPENAI_API_KEY",
            "OpenAI",
        ),
        (
            ModelProviderType::Anthropic,
            "anthropic",
            "ANTHROPIC_API_KEY",
            "Anthropic",
        ),
        (
            ModelProviderType::Gemini,
            "gemini",
            "GEMINI_API_KEY",
            "Gemini",
        ),
        (ModelProviderType::Xai, "xai", "XAI_API_KEY", "xAI"),
    ] {
        for source in ["flag", "environment", "precedence", "noninteractive"] {
            let environment = Environment::set(
                env,
                if matches!(source, "environment" | "precedence") {
                    "test-environment-key"
                } else {
                    ""
                },
            );
            let mut h = Harness::new().await;
            let mut args = vec!["create", "test-provider", "--type", kind_name];
            if matches!(source, "flag" | "precedence") {
                args.extend(["--api-key", "test-flag-key"]);
            }
            if source == "noninteractive" {
                assert_eq!(h.run("modelprovider", &args).await.unwrap_err().to_string(), "failed to read API key: cannot prompt for API key in non-interactive terminal\n\nPlease use --api-key flag or set environment variable");
                assert_eq!(
                    h.stdout(),
                    format!("Enter {display} API key for test-provider: ")
                );
            } else {
                h.fake.expect(
                    "create_model_provider",
                    create_provider_request(
                        "test-provider",
                        kind,
                        if source == "environment" {
                            "test-environment-key"
                        } else {
                            "test-flag-key"
                        },
                    ),
                    CreateModelProviderResponse {
                        model_provider: Some(provider(P, "test-provider", kind)),
                    },
                );
                h.run("modelprovider", &args).await.unwrap();
                assert_eq!(h.stdout(), format!("{P}\n"));
            }
            h.fake.done();
            drop(environment);
        }
    }
}
#[tokio::test]
async fn provider_create_validation_and_rpc_errors() {
    for (args, error) in [
        (vec!["create", "my-provider"], "required flag(s) \"type\" not set"),
        (vec!["create", "--type", "openai"], "accepts 1 arg(s), received 0"),
        (vec!["create", "my-provider", "--type", "invalid"], "invalid argument \"invalid\" for \"-t, --type\" flag: must be one of \"openai\",\"anthropic\",\"gemini\",\"xai\""),
    ] {
        let mut h = Harness::new().await;
        assert_eq!(h.run("modelprovider", &args).await.unwrap_err().to_string(), error); h.fake.done();
    }
    let mut h = Harness::new().await;
    h.fake.fail_with(
        "create_model_provider",
        create_provider_request("my-openai", ModelProviderType::Openai, "test-key"),
        ConnectError::new(Code::Unknown, "provider name already exists"),
    );
    assert_eq!(
        h.run(
            "modelprovider",
            &[
                "create",
                "my-openai",
                "--type",
                "openai",
                "--api-key",
                "test-key"
            ]
        )
        .await
        .unwrap_err()
        .to_string(),
        "failed to create model provider: unknown: provider name already exists"
    );
    h.fake.done();
}
#[tokio::test]
async fn provider_get_name_id_yaml() {
    for (input, name, kind, format) in [
        (
            "anthropic-dev",
            "anthropic-dev",
            ModelProviderType::Anthropic,
            "card",
        ),
        (P, "openai-prod", ModelProviderType::Openai, "card"),
        (
            "openai-prod",
            "openai-prod",
            ModelProviderType::Openai,
            "yaml",
        ),
    ] {
        let mut h = Harness::new().await;
        if input != P {
            provider_lookup(&h.fake, input, P);
        }
        let mut item = provider(P, name, kind);
        if format == "yaml" {
            item.spec.as_mut().unwrap().enabled = false;
        }
        provider_get(&h.fake, item);
        let args = if format == "card" {
            vec!["get", input]
        } else {
            vec!["get", input, "--output", format]
        };
        h.run("modelprovider", &args).await.unwrap();
        let kind = if kind == ModelProviderType::Openai {
            "openai"
        } else {
            "anthropic"
        };
        if format == "yaml" {
            let mut expected = provider_display(P, name, kind, true);
            expected["enabled"] = json!(false);
            assert_eq!(
                serde_yaml::from_str::<Value>(&h.stdout()).unwrap(),
                expected
            );
        } else {
            assert_eq!(
                h.stdout(),
                format!("Id:           {P}\nName:         {name}\nProviderType: {kind}\n")
            );
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn provider_get_missing_duplicate_and_rpc_errors() {
    for stage in ["missing", "duplicate", "get", "lookup"] {
        let mut h = Harness::new().await;
        let (name, error) = match stage {
            "missing" => {
                h.fake.expect(
                    "list_model_providers",
                    ListModelProvidersRequest::default(),
                    ListModelProvidersResponse::default(),
                );
                ("nonexistent", "failed to resolve model provider nonexistent: model provider nonexistent not found".into())
            }
            "duplicate" => {
                h.fake.expect(
                    "list_model_providers",
                    ListModelProvidersRequest::default(),
                    ListModelProvidersResponse {
                        model_providers: vec![
                            provider(P, "duplicate", ModelProviderType::Anthropic),
                            provider(Q, "duplicate", ModelProviderType::Openai),
                        ],
                        ..Default::default()
                    },
                );
                ("duplicate", "failed to resolve model provider duplicate: multiple model providers found for duplicate".into())
            }
            "get" => {
                h.fake.fail(
                    "get_model_provider",
                    GetModelProviderRequest { id: P.into() },
                );
                (P, format!("failed to get model provider {P}: internal"))
            }
            _ => {
                h.fake
                    .fail("list_model_providers", ListModelProvidersRequest::default());
                ("openai", "failed to resolve model provider openai: failed to list model providers: internal".into())
            }
        };
        assert_eq!(
            h.run("modelprovider", &["get", name])
                .await
                .unwrap_err()
                .to_string(),
            error
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn provider_list_all_single_multiple_comma_short_json_empty() {
    for case in [
        "all", "single", "multiple", "comma", "short", "json", "empty",
    ] {
        let mut h = Harness::new().await;
        let mut args = vec!["list"];
        let types = match case {
            "single" => {
                args.extend(["--provider-type", "anthropic"]);
                vec![ModelProviderType::Anthropic]
            }
            "multiple" => {
                args.extend(["--provider-type", "anthropic", "--provider-type", "openai"]);
                vec![ModelProviderType::Anthropic, ModelProviderType::Openai]
            }
            "comma" => {
                args.extend(["--provider-type", "anthropic, openai"]);
                vec![ModelProviderType::Anthropic, ModelProviderType::Openai]
            }
            "short" => {
                args.extend(["-t", "openai"]);
                vec![ModelProviderType::Openai]
            }
            _ => vec![],
        };
        if case == "json" {
            args.extend(["--output", "json"]);
        }
        let mut disabled = provider(N, "anthropic-disabled", ModelProviderType::Anthropic);
        disabled.spec.as_mut().unwrap().enabled = false;
        let items = match case {
            "empty" => vec![],
            "single" => vec![
                provider(P, "anthropic-dev", ModelProviderType::Anthropic),
                disabled,
            ],
            "short" | "json" => vec![provider(Q, "openai-prod", ModelProviderType::Openai)],
            _ => vec![
                provider(P, "anthropic-dev", ModelProviderType::Anthropic),
                provider(Q, "openai-prod", ModelProviderType::Openai),
                disabled,
            ],
        };
        h.fake.expect(
            "list_model_providers",
            provider_list_request(&types),
            ListModelProvidersResponse {
                model_providers: items,
                ..Default::default()
            },
        );
        h.run("modelprovider", &args).await.unwrap();
        match case {
            "empty" => assert_eq!(h.stdout(), ""),
            "json" => assert_eq!(
                serde_json::from_str::<Value>(&h.stdout()).unwrap(),
                json!([provider_display(Q, "openai-prod", "openai", false)])
            ),
            "short" => {
                assert!(h.stdout().contains("openai-prod"));
                assert!(!h.stdout().contains("anthropic-dev"));
            }
            "single" => {
                assert!(h.stdout().contains("anthropic-dev"));
                assert!(h.stdout().contains("anthropic-disabled"));
                assert!(!h.stdout().contains("openai-prod"));
            }
            _ => {
                for name in ["anthropic-dev", "openai-prod", "anthropic-disabled"] {
                    assert!(h.stdout().contains(name));
                }
            }
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn provider_list_validation_and_rpc_errors() {
    let mut h = Harness::new().await;
    assert_eq!(h.run("modelprovider", &["list", "--provider-type", "invalid"]).await.unwrap_err().to_string(), "invalid argument \"invalid\" for \"-t, --provider-type\" flag: must be one of \"openai\",\"anthropic\",\"gemini\",\"xai\"");
    h.fake.done();
    let mut h = Harness::new().await;
    h.fake
        .fail("list_model_providers", provider_list_request(&[]));
    assert_eq!(
        h.run("modelprovider", &["list"])
            .await
            .unwrap_err()
            .to_string(),
        "failed to list model providers: internal"
    );
    h.fake.done();
}
#[tokio::test]
async fn provider_delete_cascades_names_ids_mixed_force_confirm_cancel_empty() {
    for (names, force, input, confirmed, empty) in [
        (vec!["openai-dev"], true, "", true, false),
        (vec![P], true, "", true, false),
        (vec!["openai-dev", "anthropic-dev"], true, "", true, false),
        (vec![P, "anthropic-dev"], true, "", true, false),
        (vec!["empty-provider"], true, "", true, true),
        (vec!["openai-dev"], false, "y\n", true, false),
        (vec!["openai-dev"], false, "n\n", false, false),
        (
            vec!["openai-dev", "anthropic-dev"],
            false,
            "y\n",
            true,
            false,
        ),
    ] {
        let mut h = Harness::new().await;
        h.stdin(input);
        for (index, name) in names.iter().enumerate() {
            if *name != P {
                provider_lookup(&h.fake, name, if index == 0 { P } else { Q });
            }
        }
        if confirmed {
            for index in 0..names.len() {
                let id = if index == 0 { P } else { Q };
                let models = if empty {
                    vec![]
                } else if index == 0 {
                    vec![
                        model(M, "gpt-4", id, 8192, true),
                        model(N, "gpt-3.5-turbo", id, 16384, true),
                    ]
                } else {
                    vec![model(N, "claude-3-5-sonnet", id, 200000, true)]
                };
                let ids: Vec<_> = models
                    .iter()
                    .map(|m| m.metadata.as_ref().unwrap().id.clone())
                    .collect();
                h.fake.expect(
                    "list_models",
                    model_list_request(Some(id), None),
                    ListModelsResponse {
                        models,
                        ..Default::default()
                    },
                );
                for id in ids {
                    h.fake.expect(
                        "delete_model",
                        DeleteModelRequest { id },
                        DeleteModelResponse {},
                    );
                }
                h.fake.expect(
                    "delete_model_provider",
                    DeleteModelProviderRequest { id: id.into() },
                    DeleteModelProviderResponse {},
                );
            }
        }
        let mut args = vec!["delete"];
        if force {
            args.push("--force");
        }
        args.extend(&names);
        h.run("modelprovider", &args).await.unwrap();
        assert_eq!(
            h.stdout(),
            if force {
                String::new()
            } else {
                format!(
                    "Are you sure you want to delete model-provider{} {}? (y/n): ",
                    if names.len() > 1 { "s" } else { "" },
                    names.join(" ")
                )
            }
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn provider_delete_failures_stop_before_later_deletions() {
    for stage in ["missing", "model", "provider", "list", "lookup"] {
        let mut h = Harness::new().await;
        let name = if stage == "missing" {
            "nonexistent"
        } else {
            "openai-dev"
        };
        let error = match stage {
            "missing" => {
                h.fake.expect(
                    "list_model_providers",
                    ListModelProvidersRequest::default(),
                    ListModelProvidersResponse::default(),
                );
                "failed to resolve model provider nonexistent: model provider nonexistent not found"
            }
            "lookup" => {
                h.fake
                    .fail("list_model_providers", ListModelProvidersRequest::default());
                "failed to resolve model provider openai-dev: failed to list model providers: internal"
            }
            _ => {
                provider_lookup(&h.fake, name, P);
                if stage == "list" {
                    h.fake
                        .fail("list_models", model_list_request(Some(P), None));
                    "failed to list models for model provider openai-dev: internal"
                } else {
                    h.fake.expect(
                        "list_models",
                        model_list_request(Some(P), None),
                        ListModelsResponse {
                            models: vec![model(M, "gpt-4", P, 8192, true)],
                            ..Default::default()
                        },
                    );
                    if stage == "model" {
                        h.fake
                            .fail("delete_model", DeleteModelRequest { id: M.into() });
                        "failed to delete model gpt-4 for model provider openai-dev: internal"
                    } else {
                        h.fake.expect(
                            "delete_model",
                            DeleteModelRequest { id: M.into() },
                            DeleteModelResponse {},
                        );
                        h.fake.fail(
                            "delete_model_provider",
                            DeleteModelProviderRequest { id: P.into() },
                        );
                        "failed to delete model provider openai-dev: internal"
                    }
                }
            }
        };
        assert_eq!(
            h.run("modelprovider", &["delete", "--force", name])
                .await
                .unwrap_err()
                .to_string(),
            error
        );
        h.fake.done();
    }
}

#[tokio::test]
async fn aliases_wide_no_headers_markdown_and_help() {
    for group in ["model", "modelprovider"] {
        for (flag, expectation) in [
            ("--wide", "Enabled"),
            ("--no-headers", ""),
            ("--output=md", "**Id:**"),
        ] {
            let mut h = Harness::new().await;
            if group == "model" {
                h.fake.expect(
                    "list_models",
                    model_list_request(None, Some(true)),
                    ListModelsResponse {
                        models: vec![model(M, "gpt-4", P, 8192, true)],
                        ..Default::default()
                    },
                );
            } else {
                h.fake.expect(
                    "list_model_providers",
                    provider_list_request(&[]),
                    ListModelProvidersResponse {
                        model_providers: vec![provider(P, "openai", ModelProviderType::Openai)],
                        ..Default::default()
                    },
                );
            }
            h.run(group, &["ls", flag]).await.unwrap();
            if flag == "--no-headers" {
                assert!(!h.stdout().contains("Name"));
            } else {
                assert!(h.stdout().contains(expectation));
            }
            h.fake.done();
        }
        let mut h = Harness::new().await;
        if group == "model" {
            h.fake.expect(
                "delete_model",
                DeleteModelRequest { id: M.into() },
                DeleteModelResponse {},
            );
        } else {
            h.fake.expect(
                "list_models",
                model_list_request(Some(M), None),
                ListModelsResponse::default(),
            );
            h.fake.expect(
                "delete_model_provider",
                DeleteModelProviderRequest { id: M.into() },
                DeleteModelProviderResponse {},
            );
        }
        h.run(group, &["rm", "-f", M]).await.unwrap();
        assert_eq!(h.stdout(), "");
        h.fake.done();
        let mut h = Harness::new().await;
        h.run(group, &[]).await.unwrap();
        for subcommand in ["create", "get", "list", "delete"] {
            assert!(h.stdout().contains(subcommand));
        }
        h.fake.done();
    }
    let mut root = clap::Command::new("construct")
        .subcommand(construct::commands::model::command())
        .subcommand(construct::commands::modelprovider::command());
    for alias in ["models", "modelproviders", "mp"] {
        assert!(root
            .try_get_matches_from_mut(["construct", alias, "ls"])
            .is_ok());
    }
}
