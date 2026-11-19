mod common;
use common::{Fake, Harness};
use construct_connect::{Code, ConnectError};
use construct_proto::construct::v1::*;
use serde_json::{json, Value};

const T: &str = "01974c1d-0be8-70e1-88b4-ad9462fff251";
const U: &str = "01974c1d-0be8-70e1-88b4-ad9462fff252";
const A: &str = "01974c1d-0be8-70e1-88b4-ad9462fff253";
const M: &str = "01974c1d-0be8-70e1-88b4-ad9462fff254";
const N: &str = "01974c1d-0be8-70e1-88b4-ad9462fff255";
const MODEL: &str = "01974c1d-0be8-70e1-88b4-ad9462fff256";
const CREATED: &str = "2025-06-07T12:00:00Z";
const UPDATED: &str = "2025-06-07T13:00:00Z";

fn task(id: &str, agent: &str) -> Task {
    serde_json::from_value(json!({"metadata":{"id":id,"createdAt":CREATED,"updatedAt":UPDATED},"spec":{"agentId":agent},"status":{"usage":{"inputTokens":1000,"outputTokens":500,"cacheWriteTokens":100,"cacheReadTokens":50,"cost":0.05}}})).unwrap()
}
fn message(id: &str, role: MessageRole, content: &str) -> Message {
    serde_json::from_value(json!({"metadata":{"id":id,"taskId":T,"agentId":A,"modelId":MODEL,"role":role.as_str_name(),"createdAt":CREATED,"updatedAt":UPDATED},"spec":{"content":[{"text":{"content":content}}]},"status":{"usage":{"inputTokens":100,"outputTokens":50,"cacheWriteTokens":10,"cacheReadTokens":5,"cost":0.01}}})).unwrap()
}
fn agent_request(name: &str) -> ListAgentsRequest {
    ListAgentsRequest {
        filter: Some(list_agents_request::Filter {
            names: vec![name.into()],
        }),
        ..Default::default()
    }
}
fn lookup(fake: &Fake, name: &str) {
    fake.expect(
        "list_agents",
        agent_request(name),
        ListAgentsResponse {
            agents: vec![Agent {
                metadata: Some(AgentMetadata {
                    id: A.into(),
                    ..Default::default()
                }),
                spec: Some(AgentSpec {
                    name: name.into(),
                    ..Default::default()
                }),
            }],
            ..Default::default()
        },
    );
}
fn tasks_request(agent: Option<&str>, limit: i32) -> ListTasksRequest {
    ListTasksRequest {
        filter: Some(list_tasks_request::Filter {
            agent_id: agent.map(str::to_owned),
            ..Default::default()
        }),
        page_size: (limit != 0).then_some(limit),
        ..Default::default()
    }
}
fn messages_request(
    task: Option<&str>,
    agent: Option<&str>,
    role: Option<MessageRole>,
) -> ListMessagesRequest {
    ListMessagesRequest {
        filter: Some(list_messages_request::Filter {
            task_ids: task.map(str::to_owned),
            agent_ids: agent.map(str::to_owned),
            roles: role.map(|r| r as i32),
        }),
        ..Default::default()
    }
}
fn task_display(id: &str, agent: &str, yaml: bool) -> Value {
    json!({"id":id,"agent_id":agent,"workspace":"","created_at":CREATED,"updated_at":UPDATED,"usage":{"input_tokens":1000,"output_tokens":500,"cache_write_tokens":100,"cache_read_tokens":50,"cost":0.05,"tool_uses":if yaml {json!({})} else {Value::Null}}})
}
fn message_display(id: &str, role: &str, content: &str) -> Value {
    json!({"id":id,"task_id":T,"agent":A,"model":MODEL,"role":role,"content":content,"created_at":CREATED,"updated_at":UPDATED,"usage":{"input_tokens":100,"output_tokens":50,"cache_write_tokens":10,"cache_read_tokens":5,"cost":0.01}})
}

#[tokio::test]
async fn task_create_name_id_workspace_and_short_flags() {
    for case in ["name", "id", "workspace", "short"] {
        let mut h = Harness::new().await;
        let workspace = h.dir.path().to_str().unwrap().to_owned();
        let name = match case {
            "id" => A,
            "workspace" => "sql-expert",
            "short" => "reviewer",
            _ => "coder",
        };
        if case != "id" {
            lookup(&h.fake, name);
        }
        let use_workspace = matches!(case, "workspace" | "short");
        h.fake.expect(
            "create_task",
            CreateTaskRequest {
                agent_id: A.into(),
                project_directory: if use_workspace {
                    workspace.clone()
                } else {
                    String::new()
                },
                ..Default::default()
            },
            CreateTaskResponse {
                task: Some(task(T, A)),
            },
        );
        let mut args = vec![
            "create",
            if case == "short" { "-a" } else { "--agent" },
            name,
        ];
        if use_workspace {
            args.extend([
                if case == "short" { "-w" } else { "--workspace" },
                &workspace,
            ]);
        }
        h.run("task", &args).await.unwrap();
        assert_eq!(h.stdout(), format!("{T}\n"));
        h.fake.done();
    }
}
#[tokio::test]
async fn task_create_missing_agent_workspace_lookup_and_rpc_errors() {
    for case in ["required", "missing", "workspace", "create", "lookup"] {
        let mut h = Harness::new().await;
        let missing = h.dir.path().join("missing");
        let missing = missing.to_str().unwrap();
        let workspace = h.dir.path().to_str().unwrap().to_owned();
        let mut args = vec!["create"];
        let error = match case {
            "required" => {
                args.extend(["-w", missing]);
                "required flag(s) \"agent\" not set".into()
            }
            "workspace" => {
                args.extend(["--agent", "coder", "--workspace", missing]);
                format!("workspace directory {missing} does not exist")
            }
            "missing" => {
                args.extend(["--agent", "nonexistent"]);
                h.fake.expect(
                    "list_agents",
                    agent_request("nonexistent"),
                    ListAgentsResponse::default(),
                );
                "failed to resolve agent nonexistent: agent nonexistent not found".into()
            }
            "lookup" => {
                args.extend(["--agent", "coder"]);
                h.fake.fail("list_agents", agent_request("coder"));
                "failed to resolve agent coder: failed to list agents: internal".into()
            }
            _ => {
                args.extend(["--agent", "coder", "--workspace", &workspace]);
                lookup(&h.fake, "coder");
                h.fake.fail_with(
                    "create_task",
                    CreateTaskRequest {
                        agent_id: A.into(),
                        project_directory: workspace.clone(),
                        ..Default::default()
                    },
                    ConnectError::new(Code::AlreadyExists, ""),
                );
                "failed to create task: already_exists".into()
            }
        };
        assert_eq!(h.run("task", &args).await.unwrap_err().to_string(), error);
        h.fake.done();
    }
}
#[tokio::test]
async fn task_get_card_json_yaml_and_not_found() {
    for format in ["card", "json", "yaml", "error"] {
        let mut h = Harness::new().await;
        if format == "error" {
            h.fake.fail_with(
                "get_task",
                GetTaskRequest { id: T.into() },
                ConnectError::new(Code::NotFound, ""),
            );
            assert_eq!(
                h.run("task", &["get", T]).await.unwrap_err().to_string(),
                format!("failed to get task {T}: not_found")
            );
        } else {
            h.fake.expect(
                "get_task",
                GetTaskRequest { id: T.into() },
                GetTaskResponse {
                    task: Some(task(T, A)),
                },
            );
            let args = if format == "card" {
                vec!["get", T]
            } else {
                vec!["get", T, "--output", format]
            };
            h.run("task", &args).await.unwrap();
            match format {
                "json" => assert_eq!(
                    serde_json::from_str::<Value>(&h.stdout()).unwrap(),
                    task_display(T, A, false)
                ),
                "yaml" => assert_eq!(
                    serde_yaml::from_str::<Value>(&h.stdout()).unwrap(),
                    task_display(T, A, true)
                ),
                _ => assert_eq!(
                    h.stdout(),
                    format!("Id:          {T}\nDescription: \nAgentId:     {A}\nWorkspace:   \n")
                ),
            }
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn task_list_all_agent_name_id_json_empty_limit_and_errors() {
    for case in [
        "all", "name", "id", "json", "empty", "missing", "error", "limit",
    ] {
        let mut h = Harness::new().await;
        let mut args = vec!["list"];
        let agent = match case {
            "name" => {
                lookup(&h.fake, "coder");
                args.extend(["--agent", "coder"]);
                Some(A)
            }
            "id" => {
                args.extend(["--agent", A]);
                Some(A)
            }
            _ => None,
        };
        if case == "json" {
            args.extend(["--output", "json"]);
        }
        if case == "limit" {
            args.extend(["-l", "12"]);
        }
        if case == "missing" {
            args.extend(["--agent", "nonexistent"]);
            h.fake.expect(
                "list_agents",
                agent_request("nonexistent"),
                ListAgentsResponse::default(),
            );
            assert_eq!(
                h.run("task", &args).await.unwrap_err().to_string(),
                "failed to resolve agent nonexistent: agent nonexistent not found"
            );
        } else if case == "error" {
            h.fake.fail("list_tasks", tasks_request(None, 0));
            assert_eq!(
                h.run("task", &args).await.unwrap_err().to_string(),
                "failed to list tasks: internal"
            );
        } else {
            let items = match case {
                "empty" => vec![],
                "all" => vec![task(T, A), task(U, MODEL)],
                _ => vec![task(T, A)],
            };
            h.fake.expect(
                "list_tasks",
                tasks_request(agent, if case == "limit" { 12 } else { 0 }),
                ListTasksResponse {
                    tasks: items,
                    ..Default::default()
                },
            );
            h.run("task", &args).await.unwrap();
            match case {
                "empty" => assert_eq!(h.stdout(), ""),
                "json" => assert_eq!(
                    serde_json::from_str::<Value>(&h.stdout()).unwrap(),
                    json!([task_display(T, A, false)])
                ),
                _ => {
                    assert!(h.stdout().contains(T));
                    assert!(h.stdout().contains("AgentId"));
                    if case == "all" {
                        assert!(h.stdout().contains(U));
                    }
                }
            }
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn task_and_message_delete_force_multiple_confirm_cancel_and_errors() {
    for group in ["task", "message"] {
        for case in [
            "single",
            "multiple",
            "confirm",
            "cancel",
            "confirm-multiple",
            "first-error",
            "second-error",
        ] {
            let mut h = Harness::new().await;
            let force = matches!(case, "single" | "multiple" | "first-error" | "second-error");
            let multiple = matches!(case, "multiple" | "confirm-multiple" | "second-error")
                || (group == "message" && case == "first-error");
            let ids = if multiple { vec![T, U] } else { vec![T] };
            h.stdin(if case == "cancel" { "n\n" } else { "y\n" });
            if case != "cancel" {
                for (index, id) in ids.iter().enumerate() {
                    let error = if case == "first-error" && index == 0 {
                        Some(Code::NotFound)
                    } else if case == "second-error" && index == 1 {
                        Some(Code::Internal)
                    } else {
                        None
                    };
                    if group == "task" {
                        if let Some(code) = error {
                            h.fake.fail_with(
                                "delete_task",
                                DeleteTaskRequest { id: (*id).into() },
                                ConnectError::new(code, ""),
                            );
                        } else {
                            h.fake.expect(
                                "delete_task",
                                DeleteTaskRequest { id: (*id).into() },
                                DeleteTaskResponse {},
                            );
                        }
                    } else if let Some(code) = error {
                        h.fake.fail_with(
                            "delete_message",
                            DeleteMessageRequest { id: (*id).into() },
                            ConnectError::new(code, ""),
                        );
                    } else {
                        h.fake.expect(
                            "delete_message",
                            DeleteMessageRequest { id: (*id).into() },
                            DeleteMessageResponse {},
                        );
                    }
                    if error.is_some() {
                        break;
                    }
                }
            }
            let mut args = vec!["delete"];
            if force {
                args.push("--force");
            }
            args.extend(&ids);
            let result = h.run(group, &args).await;
            match case {
                "first-error" => assert_eq!(
                    result.unwrap_err().to_string(),
                    format!("failed to delete {group} {T}: not_found")
                ),
                "second-error" => assert_eq!(
                    result.unwrap_err().to_string(),
                    format!("failed to delete {group} {U}: internal")
                ),
                _ => result.unwrap(),
            }
            assert_eq!(
                h.stdout(),
                if force {
                    String::new()
                } else {
                    format!(
                        "Are you sure you want to delete {group}{} {}? (y/n): ",
                        if multiple { "s" } else { "" },
                        ids.join(" ")
                    )
                }
            );
            h.fake.done();
        }
    }
}

#[tokio::test]
async fn message_create_and_rpc_error() {
    for fail in [false, true] {
        let mut h = Harness::new().await;
        let content = if fail {
            "Test message"
        } else {
            "Please implement a hello world function"
        };
        let request = CreateMessageRequest {
            task_id: T.into(),
            content: vec![MessagePart {
                data: Some(message_part::Data::Text(message_part::Text {
                    content: content.into(),
                })),
            }],
        };
        if fail {
            h.fake.fail("create_message", request);
            assert_eq!(
                h.run("message", &["create", T, content])
                    .await
                    .unwrap_err()
                    .to_string(),
                "failed to create message: internal"
            );
        } else {
            h.fake.expect(
                "create_message",
                request,
                CreateMessageResponse {
                    message: Some(message(M, MessageRole::User, content)),
                },
            );
            h.run("message", &["create", T, content]).await.unwrap();
            assert_eq!(h.stdout(), format!("{M}\n"));
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn message_get_card_assistant_json_yaml_and_not_found() {
    for (format, role, content) in [
        ("card", MessageRole::User, "Test message content"),
        ("json", MessageRole::Assistant, "Assistant response"),
        ("yaml", MessageRole::User, "YAML test message"),
        ("error", MessageRole::User, ""),
    ] {
        let mut h = Harness::new().await;
        if format == "error" {
            h.fake.fail_with(
                "get_message",
                GetMessageRequest { id: M.into() },
                ConnectError::new(Code::NotFound, ""),
            );
            assert_eq!(
                h.run("message", &["get", M]).await.unwrap_err().to_string(),
                format!("failed to get message {M}: not_found")
            );
        } else {
            h.fake.expect(
                "get_message",
                GetMessageRequest { id: M.into() },
                GetMessageResponse {
                    message: Some(message(M, role, content)),
                },
            );
            let args = if format == "card" {
                vec!["get", M]
            } else {
                vec!["get", M, "--output", format]
            };
            h.run("message", &args).await.unwrap();
            match format {
                "json" => assert_eq!(
                    serde_json::from_str::<Value>(&h.stdout()).unwrap(),
                    message_display(M, "assistant", content)
                ),
                "yaml" => assert_eq!(
                    serde_yaml::from_str::<Value>(&h.stdout()).unwrap(),
                    message_display(M, "user", content)
                ),
                _ => assert_eq!(
                    h.stdout(),
                    format!(
                        "ID:     {M}\nTaskID: {T}\nAgent:  {A}\nModel:  {MODEL}\nRole:   user\n"
                    )
                ),
            }
        }
        h.fake.done();
    }
}
#[tokio::test]
async fn message_list_all_agent_name_id_task_role_json_short_yaml_empty_errors() {
    for case in [
        "all",
        "name",
        "id",
        "task",
        "role",
        "json",
        "short",
        "empty",
        "missing",
        "error",
        "invalid-role",
    ] {
        let mut h = Harness::new().await;
        let mut args = vec!["list"];
        let agent = match case {
            "name" => {
                lookup(&h.fake, "coder");
                args.extend(["--agent", "coder"]);
                Some(A)
            }
            "id" => {
                args.extend(["--agent", A]);
                Some(A)
            }
            "short" => {
                lookup(&h.fake, "sql-expert");
                args.extend(["-a", "sql-expert", "-r", "assistant", "-o", "yaml"]);
                Some(A)
            }
            _ => None,
        };
        let task = if case == "task" {
            args.extend(["--task", T]);
            Some(T)
        } else {
            None
        };
        let role = match case {
            "role" => {
                args.extend(["--role", "user"]);
                Some(MessageRole::User)
            }
            "short" => Some(MessageRole::Assistant),
            _ => None,
        };
        if case == "json" {
            args.extend(["--output", "json"]);
        }
        let error = match case {
            "missing" => {
                args.extend(["--agent", "nonexistent"]);
                h.fake.expect(
                    "list_agents",
                    agent_request("nonexistent"),
                    ListAgentsResponse::default(),
                );
                Some("failed to resolve agent nonexistent: agent nonexistent not found")
            }
            "error" => {
                h.fake
                    .fail("list_messages", messages_request(None, None, None));
                Some("failed to list messages: internal")
            }
            "invalid-role" => {
                args.extend(["-r", "tool"]);
                Some("invalid role tool: must be 'user' or 'assistant'")
            }
            _ => None,
        };
        if let Some(error) = error {
            assert_eq!(
                h.run("message", &args).await.unwrap_err().to_string(),
                error
            );
        } else {
            let (r, text) = if case == "short" {
                (MessageRole::Assistant, "Assistant response")
            } else {
                (MessageRole::User, "User message")
            };
            let items = match case {
                "empty" => vec![],
                "all" => vec![
                    message(M, MessageRole::User, "User message"),
                    message(N, MessageRole::Assistant, "Assistant response"),
                ],
                _ => vec![message(M, r, text)],
            };
            h.fake.expect(
                "list_messages",
                messages_request(task, agent, role),
                ListMessagesResponse {
                    messages: items,
                    ..Default::default()
                },
            );
            h.run("message", &args).await.unwrap();
            match case {
                "empty" => assert_eq!(h.stdout(), ""),
                "json" => assert_eq!(
                    serde_json::from_str::<Value>(&h.stdout()).unwrap(),
                    json!([message_display(M, "user", text)])
                ),
                "short" => assert_eq!(
                    serde_yaml::from_str::<Value>(&h.stdout()).unwrap(),
                    json!([message_display(M, "assistant", text)])
                ),
                _ => {
                    assert!(h.stdout().contains(M));
                    assert!(h.stdout().contains("TaskID"));
                    if case == "all" {
                        assert!(h.stdout().contains(N));
                        assert!(h.stdout().contains("assistant"));
                    }
                }
            }
        }
        h.fake.done();
    }
}

#[tokio::test]
async fn non_text_parts_render_through_task_filtered_list_and_get() {
    for action in ["list", "get"] {
        for format in ["default", "json", "yaml"] {
            let mut h = Harness::new().await;
            let mut item = message(M, MessageRole::Assistant, "Before ");
            item.spec.as_mut().unwrap().content.extend([
                MessagePart {
                    data: Some(message_part::Data::ToolCall(ToolCall {
                        id: "call-1".into(),
                        tool_name: "execute_command".into(),
                        input: Some(tool_call::Input::ExecuteCommand(
                            tool_call::ExecuteCommandInput {
                                command: "pwd".into(),
                            },
                        )),
                    })),
                },
                MessagePart {
                    data: Some(message_part::Data::ToolResult(ToolResult {
                        id: "call-1".into(),
                        tool_name: "execute_command".into(),
                        result: Some(tool_result::Result::ExecuteCommand(
                            tool_result::ExecuteCommandResult {
                                stdout: "/workspace".into(),
                                ..Default::default()
                            },
                        )),
                        ..Default::default()
                    })),
                },
                MessagePart {
                    data: Some(message_part::Data::Error(message_part::Error {
                        message: "tool unavailable".into(),
                    })),
                },
                MessagePart::default(),
                MessagePart {
                    data: Some(message_part::Data::Text(message_part::Text {
                        content: " After".into(),
                    })),
                },
            ]);
            let mut args = if action == "list" {
                h.fake.expect(
                    "list_messages",
                    messages_request(Some(T), None, None),
                    ListMessagesResponse {
                        messages: vec![message(N, MessageRole::User, "User request"), item],
                        ..Default::default()
                    },
                );
                vec!["list", "--task", T]
            } else {
                h.fake.expect(
                    "get_message",
                    GetMessageRequest { id: M.into() },
                    GetMessageResponse {
                        message: Some(item),
                    },
                );
                vec!["get", M]
            };
            if format != "default" {
                args.extend(["--output", format]);
            }
            h.run("message", &args).await.unwrap();
            let content = if format == "default" {
                h.stdout()
            } else {
                let value: Value = if format == "json" {
                    serde_json::from_str(&h.stdout()).unwrap()
                } else {
                    serde_yaml::from_str(&h.stdout()).unwrap()
                };
                let item = if action == "list" { &value[1] } else { &value };
                item["content"].as_str().unwrap().to_owned()
            };
            if format == "default" {
                assert!(!content.contains("Content"));
                assert!(!content.contains("[tool"));
            } else {
                assert_eq!(content, "Before  After");
            }
            h.fake.done();
        }
    }
}

#[tokio::test]
async fn empty_content_missing_usage_optional_fields_and_task_tool_counts() {
    for group in ["task", "message"] {
        for format in ["json", "yaml"] {
            let mut h = Harness::new().await;
            if group == "task" {
                let item = Task {
                    metadata: Some(TaskMetadata {
                        id: T.into(),
                        ..Default::default()
                    }),
                    spec: Some(TaskSpec::default()),
                    status: None,
                };
                h.fake.expect(
                    "get_task",
                    GetTaskRequest { id: T.into() },
                    GetTaskResponse { task: Some(item) },
                );
            } else {
                let item = Message {
                    metadata: Some(MessageMetadata {
                        id: T.into(),
                        ..Default::default()
                    }),
                    spec: Some(MessageSpec::default()),
                    status: None,
                };
                h.fake.expect(
                    "get_message",
                    GetMessageRequest { id: T.into() },
                    GetMessageResponse {
                        message: Some(item),
                    },
                );
            }
            h.run(group, &["get", T, "-o", format]).await.unwrap();
            let value: Value = if format == "json" {
                serde_json::from_str(&h.stdout()).unwrap()
            } else {
                serde_yaml::from_str(&h.stdout()).unwrap()
            };
            assert_eq!(value["created_at"], "1970-01-01T00:00:00Z");
            assert_eq!(value["updated_at"], "1970-01-01T00:00:00Z");
            for key in [
                "input_tokens",
                "output_tokens",
                "cache_read_tokens",
                "cache_write_tokens",
            ] {
                assert_eq!(value["usage"][key], 0);
            }
            if group == "task" {
                assert!(value.get("description").is_none());
                assert_eq!(value["agent_id"], "");
                assert_eq!(
                    value["usage"]["tool_uses"],
                    if format == "json" {
                        Value::Null
                    } else {
                        json!({})
                    }
                );
            } else {
                assert_eq!(value["role"], "unknown");
                assert_eq!(value["agent"], "");
                assert_eq!(value["model"], "");
                assert_eq!(value["content"], "");
            }
            h.fake.done();
        }
    }
    let mut h = Harness::new().await;
    let mut item = task(T, A);
    item.spec.as_mut().unwrap().description = "A task description".into();
    item.status
        .as_mut()
        .unwrap()
        .usage
        .as_mut()
        .unwrap()
        .tool_uses
        .insert("execute_command".into(), 3);
    item.metadata.as_mut().unwrap().created_at =
        Some(serde_json::from_value(json!("2025-06-07T12:00:00.123400Z")).unwrap());
    h.fake.expect(
        "get_task",
        GetTaskRequest { id: T.into() },
        GetTaskResponse { task: Some(item) },
    );
    h.run("task", &["get", T, "-o", "json"]).await.unwrap();
    let value: Value = serde_json::from_str(&h.stdout()).unwrap();
    assert_eq!(value["usage"]["tool_uses"], json!({"execute_command":3}));
    assert_eq!(value["description"], "A task description");
    assert_eq!(value["created_at"], "2025-06-07T12:00:00.1234Z");
    h.fake.done();
}

#[tokio::test]
async fn aliases_render_options_help_workspace_normalization_and_text_parts() {
    for group in ["task", "message"] {
        for format in ["--wide", "--no-headers", "--output=md"] {
            let mut h = Harness::new().await;
            if group == "task" {
                h.fake.expect(
                    "list_tasks",
                    tasks_request(None, 0),
                    ListTasksResponse {
                        tasks: vec![task(T, A)],
                        ..Default::default()
                    },
                );
            } else {
                h.fake.expect(
                    "list_messages",
                    messages_request(None, None, None),
                    ListMessagesResponse {
                        messages: vec![message(M, MessageRole::User, "text")],
                        ..Default::default()
                    },
                );
            }
            h.run(group, &["ls", format]).await.unwrap();
            if format == "--no-headers" {
                assert!(!h.stdout().contains("Id"));
                assert!(!h.stdout().contains("ID"));
            } else if format == "--wide" && group == "message" {
                assert!(h.stdout().contains("2025-06-07 12:00:00 +0000 UTC"));
            } else if format == "--output=md" {
                assert!(h.stdout().contains("**"));
            }
            h.fake.done();
        }
        let mut h = Harness::new().await;
        if group == "task" {
            h.fake.expect(
                "delete_task",
                DeleteTaskRequest { id: T.into() },
                DeleteTaskResponse {},
            );
        } else {
            h.fake.expect(
                "delete_message",
                DeleteMessageRequest { id: T.into() },
                DeleteMessageResponse {},
            );
        }
        h.run(group, &["rm", "-f", T]).await.unwrap();
        h.fake.done();
        let mut h = Harness::new().await;
        h.run(group, &[]).await.unwrap();
        for command in ["create", "get", "list", "delete"] {
            assert!(h.stdout().contains(command));
        }
        h.fake.done();
    }
    let mut root = clap::Command::new("construct")
        .subcommand(construct::commands::task::command())
        .subcommand(construct::commands::message::command());
    for alias in ["tasks", "messages", "msg"] {
        assert!(root
            .try_get_matches_from_mut(["construct", alias, "ls"])
            .is_ok());
    }
    let mut h = Harness::new().await;
    let nested = h.dir.path().join("nested");
    std::fs::create_dir(&nested).unwrap();
    let workspace = nested.join("..");
    let expected = h.dir.path().to_str().unwrap().to_owned();
    h.fake.expect(
        "create_task",
        CreateTaskRequest {
            agent_id: A.into(),
            project_directory: expected,
            ..Default::default()
        },
        CreateTaskResponse {
            task: Some(task(T, A)),
        },
    );
    h.run(
        "task",
        &["create", "-a", A, "-w", workspace.to_str().unwrap()],
    )
    .await
    .unwrap();
    h.fake.done();
    let mut h = Harness::new().await;
    let mut item = message(M, MessageRole::User, "one");
    item.spec.as_mut().unwrap().content.push(MessagePart {
        data: Some(message_part::Data::Text(message_part::Text {
            content: "two".into(),
        })),
    });
    h.fake.expect(
        "get_message",
        GetMessageRequest { id: M.into() },
        GetMessageResponse {
            message: Some(item),
        },
    );
    h.run("message", &["get", M, "-o", "json"]).await.unwrap();
    let value: Value = serde_json::from_str(&h.stdout()).unwrap();
    assert_eq!(value["content"], "onetwo");
    h.fake.done();
    let mut h = Harness::new().await;
    let mut item = message(M, MessageRole::User, "fractional timestamp");
    item.metadata.as_mut().unwrap().created_at =
        Some(serde_json::from_value(json!("2025-06-07T12:00:00.123400Z")).unwrap());
    h.fake.expect(
        "get_message",
        GetMessageRequest { id: M.into() },
        GetMessageResponse {
            message: Some(item),
        },
    );
    h.run("message", &["get", M, "--wide"]).await.unwrap();
    assert!(h.stdout().contains("2025-06-07 12:00:00.1234 +0000 UTC"));
    h.fake.done();
}
