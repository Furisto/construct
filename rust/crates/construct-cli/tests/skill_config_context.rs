mod common;
use common::Harness;
use construct::{
    config::ConfigStore,
    context::{AuthConfig, ContextManager, EndpointContext, EndpointContexts},
    credentials::MemoryProvider,
};
use construct_connect::{Code, ConnectError};
use construct_proto::construct::v1::*;
use serde_json::{json, Value};
use std::sync::Arc;

fn endpoint(address: &str) -> EndpointContext {
    EndpointContext {
        address: address.into(),
        kind: if address.starts_with('/') {
            "unix"
        } else {
            "http"
        }
        .into(),
        auth: None,
    }
}
fn setup(h: &mut Harness, current: &str, previous: &str, names: &[&str]) {
    h.app.contexts = ContextManager::new(&h.app.user, Arc::new(MemoryProvider::default()));
    let contexts = EndpointContexts {
        current: current.into(),
        previous: previous.into(),
        contexts: names
            .iter()
            .map(|name| ((*name).into(), endpoint(&format!("/tmp/{name}.sock"))))
            .collect(),
    };
    h.app.contexts.save(&contexts).unwrap();
}

#[tokio::test]
async fn context_setup_code_exchanges_at_new_endpoint_and_stores_only_keyring_reference() {
    let mut h = Harness::new().await;
    let address = h
        .app
        .contexts
        .load()
        .unwrap()
        .current()
        .unwrap()
        .address
        .clone();
    setup(&mut h, "", "", &[]);
    h.fake.expect(
        "exchange_setup_code",
        ExchangeSetupCodeRequest {
            setup_code: "ABCD-1234".into(),
        },
        ExchangeSetupCodeResponse {
            token: "ctk_setup-token".into(),
            name: "different-token-name".into(),
            ..Default::default()
        },
    );
    h.run(
        "context",
        &[
            "add",
            "remote",
            "--endpoint",
            &address,
            "--setup-code",
            "ABCD-1234",
            "--set-current",
        ],
    )
    .await
    .unwrap();
    let contexts = h.app.contexts.load().unwrap();
    assert_eq!(contexts.current, "remote");
    let context = contexts.current().unwrap();
    assert_eq!(context.address, address);
    assert_eq!(context.kind, "http");
    let auth = context.auth.as_ref().unwrap();
    assert_eq!(auth.auth_type, "token");
    assert_eq!(auth.token_ref, "keyring://construct/remote");
    assert!(auth.token.is_empty());
    assert_eq!(
        h.app.contexts.resolve_token(auth).unwrap(),
        "ctk_setup-token"
    );
    assert_eq!(
        h.stdout(),
        "Context \"remote\" created and set as current\n"
    );
    let yaml = std::fs::read_to_string(h.app.user.config.join("context.yaml")).unwrap();
    for secret in ["ctk_setup-token", "ABCD-1234"] {
        assert!(!yaml.contains(secret));
        assert!(!h.stdout().contains(secret));
    }
    h.fake.done();
}

#[tokio::test]
async fn context_setup_code_errors_do_not_save_context_or_overwrite_credentials() {
    for (code, message) in [
        ("BAD1-CODE", "invalid setup code"),
        ("OLD1-CODE", "expired setup code"),
    ] {
        let mut h = Harness::new().await;
        let address = h
            .app
            .contexts
            .load()
            .unwrap()
            .current()
            .unwrap()
            .address
            .clone();
        setup(&mut h, "remote", "", &["remote"]);
        h.app
            .contexts
            .store_token("remote", "original-token")
            .unwrap();
        let before = std::fs::read(h.app.user.config.join("context.yaml")).unwrap();
        h.fake.fail_with(
            "exchange_setup_code",
            ExchangeSetupCodeRequest {
                setup_code: code.into(),
            },
            ConnectError::new(Code::PermissionDenied, message),
        );
        assert_eq!(
            h.run(
                "context",
                &[
                    "add",
                    "remote",
                    "--endpoint",
                    &address,
                    "--setup-code",
                    code,
                    "--set-current"
                ]
            )
            .await
            .unwrap_err()
            .to_string(),
            format!("failed to exchange setup code: permission_denied: {message}")
        );
        assert_eq!(
            std::fs::read(h.app.user.config.join("context.yaml")).unwrap(),
            before
        );
        assert_eq!(
            h.app
                .contexts
                .resolve_token(&AuthConfig {
                    auth_type: "token".into(),
                    token_ref: "keyring://construct/remote".into(),
                    ..Default::default()
                })
                .unwrap(),
            "original-token"
        );
        assert!(h.stdout().is_empty());
        h.fake.done();
    }
    let mut h = Harness::new().await;
    let address = h
        .app
        .contexts
        .load()
        .unwrap()
        .current()
        .unwrap()
        .address
        .clone();
    setup(&mut h, "", "", &[]);
    for args in [
        vec!["add", "remote", "--endpoint", &address, "--setup-code", ""],
        vec![
            "add",
            "remote",
            "--endpoint",
            &address,
            "--setup-code",
            "ABCD-1234",
            "--auth-token",
        ],
    ] {
        assert!(h.run("context", &args).await.is_err());
    }
    assert!(h.fake.requests().is_empty());
    h.fake.expect(
        "exchange_setup_code",
        ExchangeSetupCodeRequest {
            setup_code: "ABCD-1234".into(),
        },
        ExchangeSetupCodeResponse::default(),
    );
    assert_eq!(
        h.run(
            "context",
            &[
                "add",
                "remote",
                "--endpoint",
                &address,
                "--setup-code",
                "ABCD-1234"
            ]
        )
        .await
        .unwrap_err()
        .to_string(),
        "setup code exchange returned an empty token"
    );
    assert!(h.app.contexts.load().unwrap().contexts.is_empty());
    assert!(h.app.contexts.provider.get("remote").is_err());
    h.fake.done();
}

#[tokio::test]
async fn context_add_go_scenarios_and_validation() {
    for (name, address, kind, current, exists) in [
        (
            "local",
            "/home/user/.construct/construct.sock",
            "",
            false,
            false,
        ),
        (
            "production",
            "https://construct.prod.example.com:8443",
            "",
            false,
            false,
        ),
        ("dev", "https://localhost:8443", "http", false, false),
        (
            "staging",
            "https://staging.example.com:8443",
            "",
            true,
            false,
        ),
        ("local", "/tmp/construct.sock", "", false, true),
    ] {
        let mut h = Harness::new().await;
        setup(
            &mut h,
            if exists { "local" } else { "" },
            "",
            if exists { &["local"] } else { &[] },
        );
        let mut args = vec!["add", name, "--endpoint", address];
        if !kind.is_empty() {
            args.extend(["--kind", kind]);
        }
        if current {
            args.push("--set-current");
        }
        h.run("context", &args).await.unwrap();
        assert_eq!(
            h.stdout(),
            format!(
                "Context {name:?} {}{}\n",
                if exists { "updated" } else { "created" },
                if current { " and set as current" } else { "" }
            )
        );
        let saved = h.app.contexts.load().unwrap();
        assert_eq!(saved.contexts[name].address, address);
        assert_eq!(
            saved.contexts[name].kind,
            if address.starts_with('/') {
                "unix"
            } else {
                "http"
            }
        );
        assert_eq!(saved.current, if current || exists { name } else { "" });
        let bytes = std::fs::read_to_string(h.app.user.config.join("context.yaml")).unwrap();
        assert!(!bytes.contains("auth:"));
        h.fake.done();
    }
    for (args, error) in [
        (
            vec!["add", "mycontext"],
            "required flag(s) \"endpoint\" not set",
        ),
        (
            vec![
                "add",
                "bad",
                "--endpoint",
                "relative.sock",
                "--kind",
                "unix",
            ],
            "unix address must be an absolute path: relative.sock",
        ),
        (
            vec![
                "add",
                "bad",
                "--endpoint",
                "https://example.com",
                "--kind",
                "tcp",
            ],
            "invalid kind: tcp",
        ),
    ] {
        let mut h = Harness::new().await;
        assert_eq!(
            h.run("context", &args).await.unwrap_err().to_string(),
            error
        );
        h.fake.done();
    }
}
#[tokio::test]
async fn context_auth_foundation_matches_go_auth_test_and_secure_prompt_error() {
    let mut h = Harness::new().await;
    setup(&mut h, "", "", &[]);
    h.app
        .contexts
        .upsert(
            "production",
            EndpointContext {
                auth: Some(AuthConfig {
                    auth_type: "token".into(),
                    token_ref: "keyring://construct/production".into(),
                    ..Default::default()
                }),
                ..endpoint("https://construct.prod.example.com:8443")
            },
            false,
        )
        .unwrap();
    h.app
        .contexts
        .store_token("production", "test-token")
        .unwrap();
    let context = h.app.contexts.get("production").unwrap();
    assert_eq!(
        context.auth.as_ref().unwrap().token_ref,
        "keyring://construct/production"
    );
    assert_eq!(
        h.app
            .contexts
            .resolve_token(context.auth.as_ref().unwrap())
            .unwrap(),
        "test-token"
    );
    h.run("context", &["rm", "production", "-f"]).await.unwrap();
    assert!(h.app.contexts.retrieve_token("production").is_err());
    let mut h = Harness::new().await;
    let error = h
        .run(
            "context",
            &[
                "add",
                "production",
                "--endpoint",
                "https://example.com",
                "--auth-token",
            ],
        )
        .await
        .unwrap_err();
    assert!(error.to_string().starts_with("failed to read token:"));
    assert_eq!(h.stdout(), "Enter authentication token: \n");
    h.fake.done();
}
#[tokio::test]
async fn context_current_use_previous_missing_and_empty() {
    for (args, current, previous, error, expected) in [
        (vec!["current"], "local", "", None, "local\n"),
        (vec!["current"], "", "", Some("no current context set"), ""),
        (
            vec!["use", "staging"],
            "local",
            "",
            None,
            "Switched to context \"staging\"\n",
        ),
        (
            vec!["use", "nonexistent"],
            "local",
            "",
            Some("context nonexistent not found"),
            "",
        ),
        (
            vec!["use"],
            "local",
            "",
            Some("accepts 1 arg(s), received 0"),
            "",
        ),
        (
            vec!["use", "-"],
            "local",
            "staging",
            None,
            "Switched to context \"staging\"\n",
        ),
        (
            vec!["use", "-"],
            "local",
            "",
            Some("no previous context found"),
            "",
        ),
    ] {
        let mut h = Harness::new().await;
        setup(&mut h, current, previous, &["local", "staging"]);
        let result = h.run("context", &args).await;
        if let Some(error) = error {
            assert_eq!(result.unwrap_err().to_string(), error);
        } else {
            result.unwrap();
        }
        assert_eq!(h.stdout(), expected);
        if args[0] == "use" && error.is_none() {
            let contexts = h.app.contexts.load().unwrap();
            assert_eq!(contexts.current, "staging");
            assert_eq!(contexts.previous, "local");
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn context_list_sorted_auth_current_json_and_empty() {
    for format in ["table", "json", "yaml", "empty"] {
        let mut h = Harness::new().await;
        setup(
            &mut h,
            if format == "empty" { "" } else { "local" },
            "",
            if format == "empty" {
                &[]
            } else {
                &["staging", "local", "ec2-dev"]
            },
        );
        if format != "empty" {
            let mut contexts = h.app.contexts.load().unwrap();
            contexts.contexts.get_mut("staging").unwrap().auth = Some(AuthConfig {
                auth_type: "token".into(),
                token: "test-inline-token".into(),
                ..Default::default()
            });
            contexts.contexts.get_mut("ec2-dev").unwrap().auth = Some(AuthConfig {
                auth_type: "token".into(),
                token_ref: "keyring://construct/ec2-dev".into(),
                ..Default::default()
            });
            h.app.contexts.save(&contexts).unwrap();
        }
        h.run(
            "context",
            &["ls", "-o", if format == "empty" { "table" } else { format }],
        )
        .await
        .unwrap();
        if format == "empty" {
            assert_eq!(h.stdout(), "");
        } else if format == "table" {
            let out = h.stdout();
            assert!(out.find("ec2-dev").unwrap() < out.find("local").unwrap());
            assert!(out.contains("token (inline)"));
            assert!(out.contains("keyring://construct/ec2-dev"));
            assert!(!out.contains("test-inline-token"));
        } else {
            let value: Value = if format == "json" {
                serde_json::from_str(&h.stdout()).unwrap()
            } else {
                serde_yaml::from_str(&h.stdout()).unwrap()
            };
            assert_eq!(
                value,
                json!([
                    {"name":"ec2-dev","endpoint":"/tmp/ec2-dev.sock","kind":"unix","auth":"keyring://construct/ec2-dev","current":false},
                    {"name":"local","endpoint":"/tmp/local.sock","kind":"unix","auth":"none","current":true},
                    {"name":"staging","endpoint":"/tmp/staging.sock","kind":"unix","auth":"token (inline)","current":false}
                ])
            );
        }
        h.fake.done();
    }
    let mut h = Harness::new().await;
    setup(&mut h, "local", "", &["local"]);
    h.run("context", &["list", "-o", "json"]).await.unwrap();
    let value: Value = serde_json::from_str(&h.stdout()).unwrap();
    assert_eq!(value.as_array().unwrap().len(), 1);
    assert_eq!(value[0]["current"], true);
    h.fake.done();
}
#[tokio::test]
async fn context_remove_single_multiple_confirm_cancel_missing_and_file_errors() {
    for case in ["single", "multiple", "confirm", "cancel", "missing"] {
        let mut h = Harness::new().await;
        setup(&mut h, "staging", "", &["staging", "dev"]);
        h.stdin(if case == "cancel" { "n\n" } else { "y\n" });
        let names = if case == "multiple" {
            vec!["staging", "dev"]
        } else if case == "missing" {
            vec!["nonexistent"]
        } else {
            vec!["staging"]
        };
        let force = !matches!(case, "confirm" | "cancel");
        let mut args = vec!["remove"];
        args.extend(&names);
        if force {
            args.push("--force");
        }
        let result = h.run("context", &args).await;
        if case == "missing" {
            assert_eq!(
                result.unwrap_err().to_string(),
                "context \"nonexistent\" not found"
            );
        } else {
            result.unwrap();
            let expected = if force {
                String::new()
            } else {
                "Are you sure you want to delete context staging? (y/n): ".into()
            };
            let removed = if case == "cancel" {
                String::new()
            } else {
                names
                    .iter()
                    .map(|n| format!("Context {n:?} removed\n"))
                    .collect::<String>()
            };
            assert_eq!(h.stdout(), format!("{expected}{removed}"));
            assert_eq!(
                h.app
                    .contexts
                    .load()
                    .unwrap()
                    .contexts
                    .contains_key("staging"),
                case == "cancel"
            );
            if case != "cancel" {
                assert_eq!(h.app.contexts.load().unwrap().current, "");
            }
        }
        h.fake.done();
    }
    for args in [
        vec!["current"],
        vec!["list"],
        vec!["use", "-"],
        vec!["add", "x", "--endpoint", "/tmp/x.sock"],
        vec!["remove", "x", "--force"],
    ] {
        let mut h = Harness::new().await;
        let path = h.app.user.config.join("context.yaml");
        std::fs::write(&path, "contexts: [").unwrap();
        let error = h.run("context", &args).await.unwrap_err().to_string();
        assert!(!error.is_empty());
        if matches!(args[0], "current" | "list" | "use") {
            assert!(error.starts_with("failed to load contexts:"));
        }
        h.fake.done();
    }
}

#[tokio::test]
async fn config_set_get_types_persistence_sections_and_errors() {
    for (input, expected) in [
        ("coder", "coder\n"),
        ("true", "true\n"),
        ("42", "42\n"),
        ("2.5", "2.5\n"),
    ] {
        let mut h = Harness::new().await;
        h.run("config", &["set", "cmd.new.agent", input])
            .await
            .unwrap();
        assert_eq!(h.stdout(), "");
        h.app.config = ConfigStore::load(&h.app.user).unwrap();
        h.run("config", &["get", "cmd.new.agent"]).await.unwrap();
        assert_eq!(h.stdout(), expected);
        let bytes = std::fs::read_to_string(h.app.user.config.join("config.yaml")).unwrap();
        assert!(bytes.starts_with("cmd:\n    new:\n        agent:"));
        h.fake.done();
    }
    let mut h = Harness::new().await;
    h.run("config", &["get", "editor"]).await.unwrap();
    assert_eq!(h.stdout(), "");
    h.run("config", &["set", "log.level", "debug"])
        .await
        .unwrap();
    h.run("config", &["set", "log.format", "json"])
        .await
        .unwrap();
    h.run("config", &["get", "log"]).await.unwrap();
    assert_eq!(h.stdout(), "log.format: json\nlog.level: debug\n");
    h.fake.done();
    for args in [
        vec!["set", "nonsense", "v"],
        vec!["get", "nonsense"],
        vec!["unset", "nonsense"],
    ] {
        let mut h = Harness::new().await;
        assert_eq!(
            h.run("config", &args).await.unwrap_err().to_string(),
            "unsupported configuration key: 'nonsense'"
        );
        h.fake.done();
    }
    let mut h = Harness::new().await;
    assert_eq!(h.run("config",&["set","output","json"]).await.unwrap_err().to_string(),"'output' is a configuration section, not a single value.\nYou can only set a specific key within a section.\n\nAvailable keys under 'output' are:\n - format\n - no-headers\n - wide\n\nExample: construct config set output.format value");
    let error = h
        .run("config", &["get", "cmdnwagent"])
        .await
        .unwrap_err()
        .to_string();
    assert!(error.contains("Did you mean one of these?"));
    assert!(error.contains("cmd.new.agent"));
    h.fake.done();
    let mut h = Harness::new().await;
    std::fs::create_dir(h.app.user.config.join("config.yaml")).unwrap();
    assert!(h.run("config", &["set", "editor", "vim"]).await.is_err());
    h.fake.done();
}
#[tokio::test]
async fn config_unset_leaf_section_force_confirm_cancel_missing_and_flush_error() {
    for case in ["leaf", "section", "cancel", "force", "missing", "error"] {
        let mut h = Harness::new().await;
        h.app
            .config
            .set("output.format", serde_yaml::Value::String("json".into()))
            .unwrap();
        h.app.config.flush().unwrap();
        h.stdin(if case == "cancel" { "n\n" } else { "y\n" });
        let key = if case == "leaf" || case == "error" {
            "output.format"
        } else if case == "missing" {
            "editor"
        } else {
            "output"
        };
        let mut args = vec!["unset", key];
        if case == "force" {
            args.push("--force");
        }
        if case == "error" {
            std::fs::remove_file(h.app.user.config.join("config.yaml")).unwrap();
            std::fs::create_dir(h.app.user.config.join("config.yaml")).unwrap();
            assert!(h.run("config", &args).await.is_err());
        } else {
            h.run("config", &args).await.unwrap();
            assert_eq!(
                h.app.config.get("output.format").is_some(),
                matches!(case, "cancel" | "missing")
            );
            if matches!(case, "section" | "cancel") {
                assert_eq!(h.stdout(),"You are about to remove the entire 'output' section and all its children:\n - format\n - no-headers\n - wide\n\nAre you sure? (y/n): ");
            } else {
                assert_eq!(h.stdout(), "");
            }
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn config_explain_list_and_command_help_aliases() {
    let mut h = Harness::new().await;
    h.run("config", &["explain", "defaults.agent"])
        .await
        .unwrap();
    assert_eq!(h.stdout(),"defaults.agent\n\n  Specifies the default agent to use when running `construct new` without the\n  --agent flag. This allows you to set a preferred agent for new conversations.\n\n  Type                         Default\n  String (Agent Name or ID)    (none)\n\n  Example: construct config set defaults.agent \"my-favorite-agent\"\n");
    assert_eq!(
        h.run("config", &["explain", "cmd.new.agent"])
            .await
            .unwrap_err()
            .to_string(),
        "unknown configuration key: cmd.new.agent"
    );
    h.fake.done();
    let mut h = Harness::new().await;
    h.app
        .config
        .set("editor", serde_yaml::Value::String("vim".into()))
        .unwrap();
    h.run("config", &["ls"]).await.unwrap();
    assert_eq!(h.stdout(), "");
    assert!(h.run("config", &["list", "--invalid"]).await.is_err());
    h.fake.done();
    for group in ["config", "context", "skill"] {
        let mut h = Harness::new().await;
        h.run(group, &[]).await.unwrap();
        assert!(!h.stdout().is_empty());
        h.fake.done();
    }
    let mut root = clap::Command::new("construct")
        .subcommand(construct::commands::context::command())
        .subcommand(construct::commands::skill::command());
    for alias in ["ctx", "skills"] {
        assert!(root
            .try_get_matches_from_mut(["construct", alias, "ls"])
            .is_ok());
    }
}

fn skill(source: Option<skill::Source>) -> Skill {
    Skill {
        name: "review".into(),
        description: "Review changes".into(),
        source,
        ..Default::default()
    }
}
#[tokio::test]
async fn skill_install_list_update_delete_main_empty_and_rpc_errors() {
    for command in ["install", "list", "update", "delete"] {
        for case in ["normal", "empty", "error"] {
            let mut h = Harness::new().await;
            let fixture = h.dir.path().join("fixture");
            std::fs::create_dir(&fixture).unwrap();
            let path = fixture.join("SKILL.md");
            std::fs::write(
                &path,
                "---\nname: review\ndescription: Review changes\n---\nReview the changes.\n",
            )
            .unwrap();
            let source = path.to_string_lossy().into_owned();
            let mut args = vec![command];
            let items = if case == "empty" {
                vec![]
            } else {
                vec![skill(None)]
            };
            match command {
                "install" => {
                    args.extend([&source, "-f", "-s", "review", "-s", "commit", "-o", "json"]);
                    let req = InstallSkillRequest {
                        source: source.clone(),
                        force: true,
                        skill_names: vec!["review".into(), "commit".into()],
                    };
                    if case == "error" {
                        h.fake.fail("install_skill", req);
                    } else {
                        h.fake.expect(
                            "install_skill",
                            req,
                            InstallSkillResponse {
                                installed_skills: items,
                            },
                        );
                    }
                }
                "list" => {
                    args.extend(["-o", "json"]);
                    if case == "error" {
                        h.fake.fail("list_skills", ListSkillsRequest {});
                    } else {
                        h.fake.expect(
                            "list_skills",
                            ListSkillsRequest {},
                            ListSkillsResponse { skills: items },
                        );
                    }
                }
                "update" => {
                    args.extend(["review", "-o", "json"]);
                    let req = UpdateSkillRequest {
                        name: Some("review".into()),
                    };
                    if case == "error" {
                        h.fake.fail("update_skill", req);
                    } else {
                        h.fake.expect(
                            "update_skill",
                            req,
                            UpdateSkillResponse {
                                updated_skills: items,
                            },
                        );
                    }
                }
                _ => {
                    args.extend(["review", "-f"]);
                    if case == "error" {
                        h.fake.fail(
                            "delete_skill",
                            DeleteSkillRequest {
                                name: "review".into(),
                            },
                        );
                    } else {
                        h.fake.expect(
                            "delete_skill",
                            DeleteSkillRequest {
                                name: "review".into(),
                            },
                            DeleteSkillResponse {},
                        );
                    }
                }
            }
            let result = h.run("skill", &args).await;
            if case == "error" {
                assert_eq!(result.unwrap_err().to_string(), "internal");
            } else {
                result.unwrap();
                match (command, case) {
                    ("install", "empty") => assert_eq!(h.stdout(), "No skills installed\n"),
                    ("update", "empty") => assert_eq!(h.stdout(), "No skills updated\n"),
                    ("delete", _) => assert_eq!(h.stdout(), "Skill \"review\" deleted\n"),
                    ("list", "empty") => assert_eq!(h.stdout(), "[]\n"),
                    _ => assert_eq!(
                        serde_json::from_str::<Value>(&h.stdout()).unwrap(),
                        json!([{"name":"review","description":"Review changes","source":"local","installed_at":"","updated_at":""}])
                    ),
                }
            }
            h.fake.done();
        }
    }
}
#[tokio::test]
async fn skill_sources_dates_rendering_confirm_cancel_update_all_and_transformed_error() {
    for (source, expected) in [
        (None, "local"),
        (
            Some(skill::Source::Url(UrlSource {
                url: "http://127.0.0.1/SKILL.md".into(),
            })),
            "url",
        ),
        (
            Some(skill::Source::Git(GitSource {
                provider: GitProvider::Github as i32,
                ..Default::default()
            })),
            "github",
        ),
        (
            Some(skill::Source::Git(GitSource {
                provider: GitProvider::Gitlab as i32,
                ..Default::default()
            })),
            "gitlab",
        ),
        (Some(skill::Source::Git(GitSource::default())), "git"),
    ] {
        let mut h = Harness::new().await;
        h.fake.expect(
            "list_skills",
            ListSkillsRequest {},
            ListSkillsResponse {
                skills: vec![skill(source)],
            },
        );
        h.run("skill", &["ls", "-o", "yaml"]).await.unwrap();
        let value: Value = serde_yaml::from_str(&h.stdout()).unwrap();
        assert_eq!(value[0]["source"], expected);
        h.fake.done();
    }
    for format in ["table", "card", "markdown"] {
        let mut h = Harness::new().await;
        let mut item = skill(None);
        item.installed_at = Some(
            serde_json::from_value(json!(
                (chrono::Utc::now() - chrono::Duration::days(3)).to_rfc3339()
            ))
            .unwrap(),
        );
        item.updated_at = Some(
            serde_json::from_value(json!(
                (chrono::Utc::now() + chrono::Duration::hours(3)).to_rfc3339()
            ))
            .unwrap(),
        );
        h.fake.expect(
            "list_skills",
            ListSkillsRequest {},
            ListSkillsResponse { skills: vec![item] },
        );
        h.run("skill", &["list", "-o", format, "--wide"])
            .await
            .unwrap();
        assert!(h.stdout().contains("3 days ago"));
        assert!(h.stdout().contains("2 hours from now"));
        assert!(h.stdout().contains("Review changes"));
        h.fake.done();
    }
    for yes in [true, false] {
        let mut h = Harness::new().await;
        h.stdin(if yes { "y\n" } else { "n\n" });
        if yes {
            h.fake.expect(
                "delete_skill",
                DeleteSkillRequest {
                    name: "review".into(),
                },
                DeleteSkillResponse {},
            );
        }
        h.run("skill", &["rm", "review"]).await.unwrap();
        assert_eq!(
            h.stdout(),
            format!(
                "Are you sure you want to delete skill review? (y/n): {}",
                if yes {
                    "Skill \"review\" deleted\n"
                } else {
                    ""
                }
            )
        );
        h.fake.done();
    }
    let mut h = Harness::new().await;
    h.fake.expect(
        "update_skill",
        UpdateSkillRequest { name: None },
        UpdateSkillResponse::default(),
    );
    h.run("skill", &["update"]).await.unwrap();
    assert_eq!(h.stdout(), "No skills updated\n");
    h.fake.done();
    let mut h = Harness::new().await;
    h.fake.fail_with(
        "list_skills",
        ListSkillsRequest {},
        ConnectError::new(Code::Unavailable, "offline"),
    );
    let error = h.run("skill", &["list"]).await.unwrap_err();
    assert!(error
        .to_string()
        .contains("Agent runtime unavailable. Retrying..."));
    h.fake.done();
    for args in [
        vec!["install"],
        vec!["delete"],
        vec!["update", "a", "b"],
        vec!["list", "--invalid"],
    ] {
        let mut h = Harness::new().await;
        assert!(h.run("skill", &args).await.is_err());
        h.fake.done();
    }
}
