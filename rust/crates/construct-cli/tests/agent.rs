mod common;
use common::{Fake, Harness};
use construct_proto::construct::v1::*;
use serde_json::{json, Value};

const A: &str = "01974c1d-0be8-70e1-88b4-ad9462fff251";
const B: &str = "01974c1d-0be8-70e1-88b4-ad9462fff252";
const C: &str = "01974c1d-0be8-70e1-88b4-ad9462fff253";
const M: &str = "01974c1d-0be8-70e1-88b4-ad9462fff254";
const N: &str = "01974c1d-0be8-70e1-88b4-ad9462fff255";
fn agent(id: &str, name: &str, model: &str) -> Agent {
    Agent {
        metadata: Some(AgentMetadata {
            id: id.into(),
            ..Default::default()
        }),
        spec: Some(AgentSpec {
            name: name.into(),
            description: format!("Description for {name}"),
            instructions: "A helpful coding assistant".into(),
            model_id: model.into(),
        }),
    }
}
fn model(id: &str, name: &str) -> Model {
    Model {
        metadata: Some(ModelMetadata {
            id: id.into(),
            ..Default::default()
        }),
        spec: Some(ModelSpec {
            name: name.into(),
            ..Default::default()
        }),
    }
}
fn list_request(names: &[&str]) -> ListAgentsRequest {
    ListAgentsRequest {
        filter: Some(list_agents_request::Filter {
            names: names.iter().map(|s| (*s).into()).collect(),
        }),
        ..Default::default()
    }
}
fn models_request(name: &str) -> ListModelsRequest {
    ListModelsRequest {
        filter: Some(list_models_request::Filter {
            names: vec![name.into()],
            ..Default::default()
        }),
        ..Default::default()
    }
}
fn lookup(fake: &Fake, name: &str, id: &str) {
    fake.expect(
        "list_agents",
        list_request(&[name]),
        ListAgentsResponse {
            agents: vec![agent(id, name, M)],
            ..Default::default()
        },
    );
}
fn model_lookup(fake: &Fake, name: &str) {
    fake.expect(
        "list_models",
        models_request(name),
        ListModelsResponse {
            models: vec![model(M, name)],
            ..Default::default()
        },
    );
}
fn model_get(fake: &Fake, id: &str, name: &str) {
    fake.expect(
        "get_model",
        GetModelRequest { id: id.into() },
        GetModelResponse {
            model: Some(model(id, name)),
        },
    );
}
fn get(fake: &Fake, id: &str, name: &str) {
    fake.expect(
        "get_agent",
        GetAgentRequest { id: id.into() },
        GetAgentResponse {
            agent: Some(agent(id, name, M)),
        },
    );
}
fn expected_display(id: &str, name: &str, model: &str) -> Value {
    json!({"id":id,"name":name,"description":format!("Description for {name}"),"instructions":"A helpful coding assistant","model":model,"created_at":"1970-01-01 00:00:00"})
}

#[tokio::test]
async fn create_success_prompt_sources_description_and_model_id() {
    for source in ["inline", "description", "id", "stdin", "file"] {
        let mut h = Harness::new().await;
        let file = h.dir.path().join("prompt.txt");
        std::fs::write(&file, "A helpful coding assistant").unwrap();
        let path = file.to_str().unwrap();
        let mut args = vec![
            "create",
            "coder",
            "--model",
            match source {
                "id" => M,
                "description" => "claude-4",
                _ => "gpt-4",
            },
        ];
        match source {
            "stdin" => {
                h.stdin("A helpful coding assistant");
                args.push("--prompt-stdin");
            }
            "file" => args.extend(["--prompt-file", path]),
            _ => args.extend(["--prompt", "A helpful coding assistant"]),
        }
        let description = if source == "description" {
            args.extend(["--description", "An agent that helps with coding tasks"]);
            "An agent that helps with coding tasks"
        } else {
            ""
        };
        if source != "id" {
            model_lookup(
                &h.fake,
                if source == "description" {
                    "claude-4"
                } else {
                    "gpt-4"
                },
            );
        }
        h.fake.expect(
            "create_agent",
            CreateAgentRequest {
                name: "coder".into(),
                description: description.into(),
                instructions: "A helpful coding assistant".into(),
                model_id: M.into(),
            },
            CreateAgentResponse {
                agent: Some(agent(A, "coder", M)),
            },
        );
        h.agent(&args).await.unwrap();
        assert_eq!(h.stdout(), format!("{A}\n"), "{source}");
        assert_eq!(h.fake.requests().last().unwrap().0, "create_agent");
        h.fake.done();
    }
}
#[tokio::test]
async fn create_prompt_validation() {
    for (flags, input, error) in [
        (
            vec![],
            "",
            "system prompt is required (use --prompt, --prompt-file, or --prompt-stdin)",
        ),
        (
            vec!["--prompt", "hello", "--prompt-stdin"],
            "hi",
            "only one prompt source can be specified (--prompt, --prompt-file, or --prompt-stdin)",
        ),
        (
            vec!["--prompt-stdin"],
            "",
            "no prompt content received from stdin",
        ),
        (
            vec!["--prompt-file", "nonexistent.txt"],
            "",
            "failed to read prompt file nonexistent.txt: open nonexistent.txt: file does not exist",
        ),
    ] {
        let mut h = Harness::new().await;
        h.stdin(input);
        let mut args = vec!["create", "coder", "--model", "gpt-4"];
        args.extend(flags);
        assert_eq!(h.agent(&args).await.unwrap_err().to_string(), error);
        h.fake.done();
    }
    let mut h = Harness::new().await;
    let file = h.dir.path().join("empty.txt");
    std::fs::write(&file, "").unwrap();
    assert_eq!(
        h.agent(&[
            "create",
            "coder",
            "--model",
            M,
            "--prompt-file",
            file.to_str().unwrap()
        ])
        .await
        .unwrap_err()
        .to_string(),
        format!("prompt file {} is empty", file.display())
    );
    h.fake.done();
}
#[tokio::test]
async fn create_model_lookup_errors() {
    for (name, count, error) in [
        ("nonexistent-model", 0, "model nonexistent-model not found"),
        ("gpt", 2, "multiple models found for gpt"),
    ] {
        let mut h = Harness::new().await;
        h.fake.expect(
            "list_models",
            models_request(name),
            ListModelsResponse {
                models: (0..count).map(|_| model(M, name)).collect(),
                ..Default::default()
            },
        );
        assert_eq!(
            h.agent(&["create", "coder", "--model", name, "--prompt", "hello"])
                .await
                .unwrap_err()
                .to_string(),
            error
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn get_by_name_id_json_yaml_and_default_card() {
    for (name, format) in [
        ("coder", "card"),
        (A, "card"),
        ("coder", "json"),
        ("coder", "yaml"),
    ] {
        let mut h = Harness::new().await;
        if name != A {
            lookup(&h.fake, name, A);
        }
        get(&h.fake, A, "coder");
        model_get(&h.fake, M, "gpt-4");
        let args = if format == "card" {
            vec!["get", name]
        } else {
            vec!["get", name, "--output", format]
        };
        h.agent(&args).await.unwrap();
        match format {
            "json" => assert_eq!(
                serde_json::from_str::<Value>(&h.stdout()).unwrap(),
                expected_display(A, "coder", "gpt-4")
            ),
            "yaml" => assert_eq!(
                serde_yaml::from_str::<Value>(&h.stdout()).unwrap(),
                expected_display(A, "coder", "gpt-4")
            ),
            _ => {
                assert!(h.stdout().contains("Name:"));
                assert!(h.stdout().contains("coder"));
                assert!(h.stdout().contains("gpt-4"));
            }
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn get_lookup_errors() {
    for (name, count, error) in [
        (
            "nonexistent",
            0,
            "failed to resolve agent nonexistent: agent nonexistent not found",
        ),
        (
            "duplicate",
            2,
            "failed to resolve agent duplicate: multiple agents found for duplicate",
        ),
    ] {
        let mut h = Harness::new().await;
        h.fake.expect(
            "list_agents",
            list_request(&[name]),
            ListAgentsResponse {
                agents: (0..count).map(|_| agent(A, name, M)).collect(),
                ..Default::default()
            },
        );
        assert_eq!(
            h.agent(&["get", name]).await.unwrap_err().to_string(),
            error
        );
        h.fake.done();
    }
    let mut h = Harness::new().await;
    h.fake.fail("get_agent", GetAgentRequest { id: A.into() });
    assert_eq!(
        h.agent(&["get", A]).await.unwrap_err().to_string(),
        format!("failed to get agent {A}: internal")
    );
    h.fake.done();
    let mut h = Harness::new().await;
    h.fake.fail("list_agents", list_request(&["coder"]));
    assert_eq!(
        h.agent(&["get", "coder"]).await.unwrap_err().to_string(),
        "failed to resolve agent coder: failed to list agents: internal"
    );
    h.fake.done();
}
#[tokio::test]
async fn delete_by_name_id_multiple_and_mixed() {
    for names in [
        vec!["coder"],
        vec![A],
        vec!["coder", "architect"],
        vec![A, B],
        vec![A, "architect", C],
    ] {
        let mut h = Harness::new().await;
        for name in &names {
            if *name == "coder" {
                lookup(&h.fake, name, A);
            } else if *name == "architect" {
                lookup(&h.fake, name, B);
            }
        }
        for name in &names {
            let id = match *name {
                "coder" => A,
                "architect" => B,
                other => other,
            };
            h.fake.expect(
                "delete_agent",
                DeleteAgentRequest { id: id.into() },
                DeleteAgentResponse {},
            );
        }
        let mut args = vec!["delete", "--force"];
        args.extend(names);
        h.agent(&args).await.unwrap();
        assert_eq!(h.stdout(), "");
        h.fake.done();
    }
}
#[tokio::test]
async fn delete_confirm_cancel_and_multiple() {
    for (names, input, confirmed) in [
        (vec!["coder"], "y\n", true),
        (vec!["coder"], "n\n", false),
        (vec!["coder", "architect"], "yes\n", true),
    ] {
        let mut h = Harness::new().await;
        h.stdin(input);
        lookup(&h.fake, "coder", A);
        if names.len() > 1 {
            lookup(&h.fake, "architect", B);
        }
        if confirmed {
            h.fake.expect(
                "delete_agent",
                DeleteAgentRequest { id: A.into() },
                DeleteAgentResponse {},
            );
            if names.len() > 1 {
                h.fake.expect(
                    "delete_agent",
                    DeleteAgentRequest { id: B.into() },
                    DeleteAgentResponse {},
                );
            }
        }
        let mut args = vec!["delete"];
        args.extend(&names);
        h.agent(&args).await.unwrap();
        assert_eq!(
            h.stdout(),
            format!(
                "Are you sure you want to delete agent{} {}? (y/n): ",
                if names.len() > 1 { "s" } else { "" },
                names.join(" ")
            )
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn delete_missing_name_and_atomic_lookup_failure() {
    for first in [false, true] {
        let mut h = Harness::new().await;
        if first {
            lookup(&h.fake, "coder", A);
        }
        h.fake.expect(
            "list_agents",
            list_request(&["nonexistent"]),
            ListAgentsResponse::default(),
        );
        let args = if first {
            vec!["delete", "--force", "coder", "nonexistent"]
        } else {
            vec!["delete", "--force", "nonexistent"]
        };
        assert_eq!(
            h.agent(&args).await.unwrap_err().to_string(),
            "agent nonexistent not found"
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn list_all_json_model_filters_names_combined_and_empty() {
    for case in [
        "all",
        "json",
        "model-id",
        "model-name",
        "models",
        "name",
        "names",
        "combined",
        "empty",
        "limit",
    ] {
        let mut h = Harness::new().await;
        let mut args = vec!["list", "-o", "json"];
        let names = match case {
            "name" | "combined" => vec!["coder"],
            "names" => vec!["coder", "reviewer"],
            _ => vec![],
        };
        for name in &names {
            args.extend(["--name", name]);
        }
        match case {
            "model-id" | "combined" => args.extend(["--model", M]),
            "model-name" => {
                args.extend(["--model", "claude-4"]);
                model_lookup(&h.fake, "claude-4");
            }
            "models" => {
                args.extend(["--model", M, "--model", "claude-4"]);
                h.fake.expect(
                    "list_models",
                    models_request("claude-4"),
                    ListModelsResponse {
                        models: vec![model(N, "claude-4")],
                        ..Default::default()
                    },
                );
            }
            "limit" => args.extend(["--limit", "1"]),
            _ => {}
        }
        let agents = if case == "empty" {
            vec![]
        } else if matches!(case, "name" | "combined") {
            vec![agent(A, "coder", M)]
        } else {
            vec![agent(A, "coder", M), agent(B, "reviewer", N)]
        };
        h.fake.expect(
            "list_agents",
            list_request(&names),
            ListAgentsResponse {
                agents,
                ..Default::default()
            },
        );
        let mut expected = Vec::new();
        if case != "empty" {
            let model = if case == "model-name" {
                "claude-4"
            } else {
                "gpt-4"
            };
            model_get(&h.fake, M, model);
            expected.push(expected_display(A, "coder", model));
            if matches!(case, "all" | "json" | "models" | "names") {
                model_get(&h.fake, N, "claude-4");
                expected.push(expected_display(B, "reviewer", "claude-4"));
            }
        }
        h.agent(&args).await.unwrap();
        assert_eq!(
            serde_json::from_str::<Value>(&h.stdout()).unwrap(),
            json!(expected),
            "{case}"
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn list_rpc_and_model_resolution_errors() {
    let mut h = Harness::new().await;
    h.fake.fail("list_agents", list_request(&[]));
    assert_eq!(
        h.agent(&["list"]).await.unwrap_err().to_string(),
        "failed to list agents: internal"
    );
    h.fake.done();
    for (name, count, error) in [
        (
            "nonexistent-model",
            0,
            "failed to resolve model nonexistent-model: model nonexistent-model not found",
        ),
        (
            "duplicate-model",
            2,
            "failed to resolve model duplicate-model: multiple models found for duplicate-model",
        ),
    ] {
        let mut h = Harness::new().await;
        h.fake.expect(
            "list_models",
            models_request(name),
            ListModelsResponse {
                models: (0..count).map(|_| model(M, name)).collect(),
                ..Default::default()
            },
        );
        assert_eq!(
            h.agent(&["list", "--model", name])
                .await
                .unwrap_err()
                .to_string(),
            error
        );
        h.fake.done();
    }
    let mut h = Harness::new().await;
    h.fake.fail("list_models", models_request("claude-4"));
    assert_eq!(
        h.agent(&["list", "--model", "claude-4"])
            .await
            .unwrap_err()
            .to_string(),
        "failed to resolve model claude-4: failed to list models: internal"
    );
    h.fake.done();
}

#[tokio::test]
async fn apply_create_update_and_no_changes() {
    for mode in ["create", "update", "unchanged"] {
        let mut h = Harness::new().await;
        let path = h.dir.path().join("agent.yaml");
        let spec = if mode == "unchanged" {
            json!({"id":A,"name":"coder","description":"Description for coder","instructions":"A helpful coding assistant","model":M})
        } else {
            json!({"id":if mode=="create" { "" } else { A },"name":"coder","description":"new description","instructions":"new instructions","model":"gpt-4"})
        };
        std::fs::write(&path, serde_yaml::to_string(&spec).unwrap()).unwrap();
        if mode != "create" {
            get(&h.fake, A, "coder");
        }
        if mode != "unchanged" {
            model_lookup(&h.fake, "gpt-4");
        }
        if mode == "create" {
            h.fake.expect(
                "create_agent",
                CreateAgentRequest {
                    name: "coder".into(),
                    description: "new description".into(),
                    instructions: "new instructions".into(),
                    model_id: M.into(),
                },
                CreateAgentResponse {
                    agent: Some(agent(A, "coder", M)),
                },
            );
        } else {
            h.fake.expect(
                "update_agent",
                UpdateAgentRequest {
                    id: A.into(),
                    description: (mode == "update").then(|| "new description".into()),
                    instructions: (mode == "update").then(|| "new instructions".into()),
                    ..Default::default()
                },
                UpdateAgentResponse::default(),
            );
        }
        h.agent(&["apply", "-f", path.to_str().unwrap()])
            .await
            .unwrap();
        assert_eq!(
            h.stdout(),
            if mode == "create" {
                format!("agent.construct.ai/coder created\nID: {A}\n")
            } else {
                "agent.construct.ai/coder configured\n".into()
            }
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn apply_invalid_empty_and_missing_fields() {
    for (content, error) in [
        ("{}", "name is required"),
        ("name: coder", "instructions are required"),
        ("name: coder\ninstructions: hi", "model is required"),
        (
            "id: invalid\nname: coder\ninstructions: hi\nmodel: gpt-4",
            "invalid agent ID format: invalid",
        ),
    ] {
        let mut h = Harness::new().await;
        let path = h.dir.path().join("agent.yaml");
        std::fs::write(&path, content).unwrap();
        let expected = if error.starts_with("invalid agent ID") {
            error.to_string()
        } else {
            format!("failed to parse agent spec file: {error}")
        };
        assert_eq!(
            h.agent(&["apply", "-f", path.to_str().unwrap()])
                .await
                .unwrap_err()
                .to_string(),
            expected
        );
        h.fake.done();
    }
    for content in ["", "[invalid"] {
        let mut h = Harness::new().await;
        let path = h.dir.path().join("agent.yaml");
        std::fs::write(&path, content).unwrap();
        assert!(h
            .agent(&["apply", "-f", path.to_str().unwrap()])
            .await
            .unwrap_err()
            .to_string()
            .starts_with("failed to parse agent spec file:"));
        h.fake.done();
    }
    let mut h = Harness::new().await;
    assert_eq!(
        h.agent(&["apply", "-f", "missing.yaml"])
            .await
            .unwrap_err()
            .to_string(),
        "failed to parse agent spec file: failed to read file: open missing.yaml: file does not exist"
    );
    h.fake.done();
}
#[tokio::test]
async fn additional_rpc_errors() {
    let mut h = Harness::new().await;
    h.fake.fail(
        "create_agent",
        CreateAgentRequest {
            name: "coder".into(),
            instructions: "hi".into(),
            model_id: M.into(),
            ..Default::default()
        },
    );
    assert_eq!(
        h.agent(&["create", "coder", "--model", M, "--prompt", "hi"])
            .await
            .unwrap_err()
            .to_string(),
        "failed to create agent: internal"
    );
    h.fake.done();
    let mut h = Harness::new().await;
    h.fake
        .fail("delete_agent", DeleteAgentRequest { id: A.into() });
    assert_eq!(
        h.agent(&["delete", A, "-f"]).await.unwrap_err().to_string(),
        format!("failed to delete agent {A}: internal")
    );
    h.fake.done();
    for action in ["get", "list"] {
        let mut h = Harness::new().await;
        if action == "get" {
            get(&h.fake, A, "coder");
        } else {
            h.fake.expect(
                "list_agents",
                list_request(&[]),
                ListAgentsResponse {
                    agents: vec![agent(A, "coder", M)],
                    ..Default::default()
                },
            );
        }
        h.fake.fail("get_model", GetModelRequest { id: M.into() });
        let args = if action == "get" {
            vec!["get", A]
        } else {
            vec!["list"]
        };
        assert_eq!(
            h.agent(&args).await.unwrap_err().to_string(),
            format!("failed to get model {M}: internal")
        );
        h.fake.done();
    }
}

#[tokio::test]
async fn apply_rpc_errors_and_model_lookup_failure() {
    for stage in ["create", "get", "update", "resolve"] {
        let mut h = Harness::new().await;
        let path = h.dir.path().join("agent.yaml");
        std::fs::write(
            &path,
            serde_yaml::to_string(&json!({
                "id": if stage == "create" { "" } else { A },
                "name": "coder", "description": "Description for coder",
                "instructions": "A helpful coding assistant",
                "model": if stage == "resolve" { "missing" } else { M }
            }))
            .unwrap(),
        )
        .unwrap();
        let error = match stage {
            "create" => {
                h.fake.fail(
                    "create_agent",
                    CreateAgentRequest {
                        name: "coder".into(),
                        description: "Description for coder".into(),
                        instructions: "A helpful coding assistant".into(),
                        model_id: M.into(),
                    },
                );
                "failed to create agent: internal"
            }
            "get" => {
                h.fake.fail("get_agent", GetAgentRequest { id: A.into() });
                "failed to get existing agent: internal"
            }
            "resolve" => {
                get(&h.fake, A, "coder");
                h.fake.expect(
                    "list_models",
                    models_request("missing"),
                    ListModelsResponse::default(),
                );
                "failed to resolve model missing: model missing not found"
            }
            _ => {
                get(&h.fake, A, "coder");
                h.fake.fail(
                    "update_agent",
                    UpdateAgentRequest {
                        id: A.into(),
                        ..Default::default()
                    },
                );
                "failed to update agent: internal"
            }
        };
        assert_eq!(
            h.agent(&["apply", "-f", path.to_str().unwrap()])
                .await
                .unwrap_err()
                .to_string(),
            error
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn aliases_rendering_options_and_agent_without_model() {
    for args in [
        vec!["ls"],
        vec!["list", "--wide"],
        vec!["list", "--no-headers"],
        vec!["list", "-o", "markdown"],
    ] {
        let mut h = Harness::new().await;
        h.fake.expect(
            "list_agents",
            list_request(&[]),
            ListAgentsResponse {
                agents: vec![agent(A, "coder", M)],
                ..Default::default()
            },
        );
        model_get(&h.fake, M, "gpt-4");
        h.agent(&args).await.unwrap();
        assert!(h.stdout().contains("coder"));
        assert_eq!(h.stdout().contains("CreatedAt"), args.contains(&"--wide"));
        if args.contains(&"--no-headers") {
            assert!(!h.stdout().contains("Name"));
        }
        h.fake.done();
    }
    let mut h = Harness::new().await;
    h.fake.expect(
        "get_agent",
        GetAgentRequest { id: A.into() },
        GetAgentResponse {
            agent: Some(agent(A, "coder", "")),
        },
    );
    h.agent(&["get", A, "-o", "json"]).await.unwrap();
    assert_eq!(
        serde_json::from_str::<Value>(&h.stdout()).unwrap()["model"],
        "none"
    );
    h.fake.done();
    let mut h = Harness::new().await;
    h.fake.expect(
        "delete_agent",
        DeleteAgentRequest { id: A.into() },
        DeleteAgentResponse {},
    );
    h.agent(&["rm", A, "-f"]).await.unwrap();
    h.fake.done();
}
#[cfg(unix)]
#[tokio::test]
async fn edit_no_changes_changed_fields_invalid_and_editor_failure() {
    use std::os::unix::fs::PermissionsExt;
    struct Restore(Option<std::ffi::OsString>);
    impl Drop for Restore {
        fn drop(&mut self) {
            match &self.0 {
                Some(s) => std::env::set_var("EDITOR", s),
                None => std::env::remove_var("EDITOR"),
            }
        }
    }
    let _restore = Restore(std::env::var_os("EDITOR"));
    for mode in ["unchanged", "changed", "invalid", "fail"] {
        let mut h = Harness::new().await;
        get(&h.fake, A, "coder");
        model_get(&h.fake, M, "gpt-4");
        let editor = h.dir.path().join("test editor");
        let script=match mode {
            "changed"=>format!("#!/bin/sh\nprintf 'name: changed\\ndescription: edited\\ninstructions: new\\nmodel: {N}\\n' > \"$1\"\n"),
            "invalid"=>"#!/bin/sh\nprintf 'name: coder\\n' > \"$1\"\n".into(),
            "fail"=>"#!/bin/sh\nexit 1\n".into(),
            _=>"#!/bin/sh\nexit 0\n".into(),
        };
        std::fs::write(&editor, script).unwrap();
        std::fs::set_permissions(&editor, std::fs::Permissions::from_mode(0o700)).unwrap();
        std::env::set_var("EDITOR", &editor);
        if mode == "changed" {
            h.fake.expect(
                "update_agent",
                UpdateAgentRequest {
                    id: A.into(),
                    name: Some("changed".into()),
                    description: Some("edited".into()),
                    instructions: Some("new".into()),
                    model_id: Some(N.into()),
                },
                UpdateAgentResponse::default(),
            );
        }
        let result = h.agent(&["edit", A]).await;
        match mode {
            "fail" => assert_eq!(
                result.unwrap_err().to_string(),
                "failed to open editor: exit status 1"
            ),
            "invalid" => assert_eq!(
                result.unwrap_err().to_string(),
                "failed to parse edited content: instructions are required"
            ),
            _ => {
                result.unwrap();
                assert_eq!(
                    h.stdout(),
                    if mode == "unchanged" {
                        "No changes made.\n"
                    } else {
                        ""
                    }
                );
            }
        }
        h.fake.done();
    }
}
