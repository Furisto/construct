use construct_core::{
    store::*,
    testing::MemoryStore,
    tool::{Tool, ToolContext},
    CancellationToken,
};
use construct_tools::{
    communication::{AskUser, Handoff, SubmitReport},
    filesystem::{EditFile, FindFile, Grep, ReadFile},
    system::ExecuteCommand,
    web::{Fetch, MAX_CONTENT_SIZE},
};
use construct_types::{entity::*, to_go_json};
use serde_json::{json, Value};
use std::{sync::Arc, time::Duration};
use uuid::Uuid;

fn context() -> ToolContext {
    ToolContext {
        workspace_root: "/not-used".into(),
        task_id: None,
        agent_id: None,
        cancellation: CancellationToken::new(),
        events: None,
    }
}

#[tokio::test]
async fn report_validation_and_go_json() {
    assert_eq!(
        SubmitReport
            .execute(&context(), json!({}))
            .await
            .unwrap_err()
            .message,
        "summary is required"
    );
    let result = SubmitReport
        .execute(
            &context(),
            json!({"Summary":"<done> 世界","Completed":true}),
        )
        .await
        .unwrap();
    assert_eq!(
        String::from_utf8(to_go_json(&result).unwrap()).unwrap(),
        r#"{"submit_report":{"summary":"\u003cdone\u003e 世界","completed":true,"deliverables":null,"next_steps":""}}"#
    );
    assert_eq!(
        to_go_json(&AskUser.execute(&context(), Value::Null).await.unwrap()).unwrap(),
        b"{}"
    );
}

#[tokio::test]
async fn command_cancellation_and_wrapper() {
    let ctx = context();
    ctx.cancellation.cancel();
    let result = tokio::time::timeout(
        Duration::from_secs(2),
        ExecuteCommand.execute(&ctx, json!({"Command":"exec sleep 30"})),
    )
    .await
    .unwrap();
    assert_eq!(result.unwrap_err().details["error"], "context canceled");
    let error = ExecuteCommand
        .execute(&context(), json!({"Command":"false; echo unreachable"}))
        .await
        .unwrap_err();
    assert_eq!(error.details["error"], "exit status 1");
    assert_eq!(error.details["output"], "");
    let error = ExecuteCommand
        .execute(
            &context(),
            json!({"Command":"printf '%s' \"$THIS_IS_UNSET_CONSTRUCT\""}),
        )
        .await
        .unwrap_err();
    assert_eq!(error.message, "error executing command");
}

#[tokio::test]
async fn command_cancellation_reaps_child_and_retains_combined_output() {
    let dir = tempfile::tempdir().unwrap();
    let marker = dir.path().join("pid");
    let ctx = context();
    ctx.cancellation.cancel();
    assert!(ExecuteCommand
        .execute(
            &ctx,
            json!({"Command":format!("echo wrong > '{}'",marker.display())})
        )
        .await
        .is_err());
    assert!(!marker.exists());
    let ctx = context();
    let cancellation = ctx.cancellation.clone();
    let command = json!({"Command":format!("printf partial; echo $$ > '{}'; exec sleep 30",marker.display())});
    let task = tokio::spawn(async move { ExecuteCommand.execute(&ctx, command).await });
    tokio::time::timeout(Duration::from_secs(2), async {
        while !marker.exists() {
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
    })
    .await
    .unwrap();
    cancellation.cancel();
    let error = tokio::time::timeout(Duration::from_secs(2), task)
        .await
        .unwrap()
        .unwrap()
        .unwrap_err();
    assert_eq!(error.details["error"], "signal: killed");
    assert_eq!(error.details["output"], "partial");
    #[cfg(target_os = "linux")]
    {
        let pid = std::fs::read_to_string(marker).unwrap();
        assert!(!std::path::Path::new(&format!("/proc/{}", pid.trim())).exists());
    }
}

#[tokio::test]
async fn read_scanner_limits_crlf_and_ranges() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("file");
    std::fs::write(&path, b"a\r\nb\r\n").unwrap();
    let output = ReadFile
        .execute(&context(), json!({"path":path,"start_line":2,"end_line":2}))
        .await
        .unwrap()
        .read_file
        .unwrap();
    assert_eq!(
        output.content,
        "// skipped 1 lines\nb\n// 0 lines remaining"
    );
    assert_eq!(
        ReadFile
            .execute(&context(), json!({"path":path,"start_line":9}))
            .await
            .unwrap()
            .read_file
            .unwrap()
            .content,
        ""
    );
    std::fs::write(&path, vec![b'x'; 65536]).unwrap();
    assert_eq!(
        ReadFile
            .execute(&context(), json!({"path":path}))
            .await
            .unwrap_err()
            .details["error"],
        "bufio.Scanner: token too long"
    );
}

#[tokio::test]
async fn edits_are_atomic_and_warn_on_dependencies() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("file");
    std::fs::write(&path, "one\ntwo\n").unwrap();
    let error = EditFile
        .execute(
            &context(),
            json!({"path":path,"diffs":[{"old":"one","new":"changed"},{"old":"absent","new":"x"}]}),
        )
        .await
        .unwrap_err();
    assert_eq!(error.message, "validation failed");
    assert_eq!(std::fs::read_to_string(&path).unwrap(), "one\ntwo\n");
    let result=EditFile.execute(&context(),json!({"path":path,"diffs":[{"old":"one","new":"changed"},{"old":"changed","new":"final"}]})).await.unwrap().edit_file.unwrap();
    assert_eq!(result.replacements_made, 2);
    assert_eq!(
        result.conflict_warnings.unwrap()[0].conflict_type,
        "dependency"
    );
    assert_eq!(std::fs::read_to_string(&path).unwrap(), "final\ntwo\n");
    assert_eq!(
        result.patch_info.patch,
        "--- a/file\n+++ b/file\n@@ -1,2 +1,2 @@\n-one\n+final\n two\n"
    );
}

#[tokio::test]
async fn edits_preserve_invalid_utf8_and_trim_unicode_space() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("bytes");
    std::fs::write(&path, b"\xffold\xfe\n").unwrap();
    EditFile
        .execute(
            &context(),
            json!({"path":path,"diffs":[{"old":"old","new":"new"}]}),
        )
        .await
        .unwrap();
    assert_eq!(std::fs::read(&path).unwrap(), b"\xffnew\xfe\n");
    std::fs::write(&path, "\u{2003}old\u{2003}\n").unwrap();
    EditFile
        .execute(
            &context(),
            json!({"path":path,"diffs":[{"old":" old ","new":"new\n"}]}),
        )
        .await
        .unwrap();
    assert_eq!(std::fs::read_to_string(&path).unwrap(), "new\n");
    std::fs::write(&path, b"\xffold\xfe\n").unwrap();
    let error = EditFile
        .execute(
            &context(),
            json!({"path":path,"diffs":[{"old":"�old�","new":"wrong"}]}),
        )
        .await
        .unwrap_err();
    assert_eq!(error.message, "validation failed");
    assert_eq!(std::fs::read(&path).unwrap(), b"\xffold\xfe\n");
}

#[test]
fn all_tools_expose_go_names_schemas_and_rendered_descriptions() {
    use construct_tools::filesystem::{CreateFile, ListFiles};
    let tools: Vec<Box<dyn Tool>> = vec![
        Box::new(CreateFile),
        Box::new(EditFile),
        Box::new(ReadFile),
        Box::new(ListFiles),
        Box::new(FindFile),
        Box::new(Grep),
        Box::new(ExecuteCommand),
        Box::new(Fetch::default()),
        Box::new(AskUser),
        Box::new(SubmitReport),
        Box::new(Handoff::new(Arc::new(MemoryStore::default()))),
    ];
    for tool in tools {
        assert!(
            tool.description().contains("# Description"),
            "{}",
            tool.name()
        );
        assert!(!tool.description().contains("%["), "{}", tool.name());
        assert_eq!(tool.input_schema()["type"], "object", "{}", tool.name());
        assert!(
            tool.input_schema()["properties"].is_object(),
            "{}",
            tool.name()
        );
    }
}

#[tokio::test]
async fn find_and_grep_real_processes() {
    let ripgrep = std::process::Command::new("rg")
        .arg("--version")
        .output()
        .is_ok();
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    std::fs::write(root.join("a.rs"), "before\nNeedle\nafter\n").unwrap();
    std::fs::write(root.join(".hidden.rs"), "Needle\n").unwrap();
    let result = FindFile
        .execute(
            &context(),
            json!({"path":root,"pattern":"*.rs","exclude_pattern":"**/.hidden.rs","max_results":10}),
        )
        .await
        .unwrap()
        .find_file
        .unwrap();
    assert_eq!(result.total_files, 1);
    assert_eq!(
        FindFile
            .execute(&context(), json!({"path":root,"pattern":"*.rs"}))
            .await
            .unwrap()
            .find_file
            .unwrap()
            .total_files,
        0
    );
    let result = Grep
        .execute(
            &context(),
            json!({"path":root,"query":"needle","include_pattern":"*.rs","exclude_pattern":".hidden.rs"}),
        )
        .await
        .unwrap()
        .grep
        .unwrap();
    assert_eq!(result.total_matches, 1);
    assert_eq!(
        result.matches.unwrap()[0].value,
        if ripgrep {
            "-1-before\n:2:Needle\n-3-after\n"
        } else {
            "-1-before:2:Needle-3-after"
        }
    );
    assert_eq!(
        Grep.execute(&context(), json!({"path":root,"query":"absent"}))
            .await
            .unwrap()
            .grep
            .unwrap()
            .total_matches,
        0
    );
    assert_eq!(
        Grep.execute(&context(), json!({"path":root,"query":"["}))
            .await
            .unwrap_err()
            .message,
        if ripgrep {
            "ripgrep error"
        } else {
            "grep error"
        }
    );
    assert_eq!(
        FindFile
            .execute(&context(), json!({"path":"relative","pattern":"*"}))
            .await
            .unwrap_err()
            .message,
        "Path is not absolute"
    );
}

#[test]
fn real_search_fallback_without_ripgrep_on_path() {
    let bin = tempfile::tempdir().unwrap();
    let grep = std::env::split_paths(&std::env::var_os("PATH").unwrap())
        .map(|p| p.join("grep"))
        .find(|p| p.is_file())
        .expect("system grep");
    std::fs::copy(grep, bin.path().join("grep")).unwrap();
    let output = std::process::Command::new(std::env::current_exe().unwrap())
        .args(["--exact", "find_and_grep_real_processes", "--nocapture"])
        .env("PATH", bin.path())
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "{}\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
}

async fn store() -> (Arc<MemoryStore>, Uuid, Uuid, Uuid) {
    let store = Arc::new(MemoryStore::default());
    let provider = Uuid::new_v4();
    let model = Uuid::new_v4();
    let source = Uuid::new_v4();
    let target = Uuid::new_v4();
    let task = Uuid::new_v4();
    store
        .create_model_provider(ModelProvider {
            id: provider,
            name: "provider".into(),
            secret: "test".into(),
            ..Default::default()
        })
        .await
        .unwrap();
    store
        .create_model(Model {
            id: model,
            name: "model".into(),
            model_provider_id: provider,
            ..Default::default()
        })
        .await
        .unwrap();
    for (id, name) in [(source, "source"), (target, "target")] {
        store
            .create_agent(Agent {
                id,
                name: name.into(),
                model_id: Some(model),
                ..Default::default()
            })
            .await
            .unwrap();
    }
    store
        .create_task(Task {
            id: task,
            agent_id: Some(source),
            ..Default::default()
        })
        .await
        .unwrap();
    (store, source, target, task)
}

#[tokio::test]
async fn handoff_success_with_and_without_instructions() {
    for handover in ["", "do this next"] {
        let (store, source, target, task) = store().await;
        let tool = Handoff::new(store.clone());
        let ctx = ToolContext {
            task_id: Some(task),
            agent_id: Some(source),
            ..context()
        };
        let result = tool
            .execute(
                &ctx,
                json!({"RequestedAgent":"target","HandoverMessage":handover}),
            )
            .await
            .unwrap();
        assert_eq!(to_go_json(&result).unwrap(), b"{}");
        assert_eq!(store.get_task(task).await.unwrap().agent_id, Some(target));
        let messages = store
            .list_messages(MessageFilter::default(), ListOptions::default())
            .await
            .unwrap();
        assert_eq!(messages.len(), 1);
        assert_eq!(messages[0].agent_id, Some(source));
        assert!(messages[0].model_id.is_some());
        let mut expected =
            "The source agent has performed a handoff to you, the target agent.\n\n".to_owned();
        if !handover.is_empty() {
            expected.push_str(&format!(
                "It has left the following instructions for you:\n{handover}"
            ));
        }
        assert_eq!(
            messages[0]
                .content
                .as_ref()
                .unwrap()
                .blocks
                .as_ref()
                .unwrap()[0]
                .payload,
            expected
        );
    }
}

#[tokio::test]
async fn handoff_errors_leave_no_partial_writes() {
    let (store, source, _, task) = store().await;
    let tool = Handoff::new(store.clone());
    for (task_id, agent, name, expected) in [
        (
            task,
            Uuid::new_v4(),
            "target",
            "failed to get current agent: agent not found",
        ),
        (task, source, "unknown", "agent unknown does not exist"),
        (
            Uuid::new_v4(),
            source,
            "target",
            "failed to get task: task not found",
        ),
        (task, source, "source", "agent cannot handoff to itself"),
    ] {
        let error = tool
            .execute(
                &context(),
                json!({"TaskID":task_id,"CurrentAgentID":agent,"RequestedAgent":name}),
            )
            .await
            .unwrap_err();
        assert_eq!(error.message, expected);
        assert_eq!(store.get_task(task).await.unwrap().agent_id, Some(source));
        assert!(store
            .list_messages(MessageFilter::default(), ListOptions::default())
            .await
            .unwrap()
            .is_empty());
    }
    for (input, expected) in [
        (json!({}), "task_id is required"),
        (json!({"TaskID":task}), "current_agent_id is required"),
        (
            json!({"TaskID":task,"CurrentAgentID":source}),
            "requested_agent is required",
        ),
    ] {
        assert_eq!(
            tool.execute(&context(), input).await.unwrap_err().message,
            expected
        );
    }
    let ctx = ToolContext {
        task_id: Some(task),
        agent_id: Some(source),
        ..context()
    };
    ctx.cancellation.cancel();
    assert_eq!(
        tool.execute(&ctx, json!({"RequestedAgent":"target"}))
            .await
            .unwrap_err()
            .message,
        "context canceled"
    );
    assert_eq!(store.get_task(task).await.unwrap().agent_id, Some(source));
    let mut agent = store.get_agent(source).await.unwrap();
    agent.model_id = None;
    store.update_agent(agent).await.unwrap();
    let error = tool
        .execute(
            &context(),
            json!({"TaskID":task,"CurrentAgentID":source,"RequestedAgent":"target"}),
        )
        .await
        .unwrap_err();
    assert!(error.message.contains("constraint/conflict:"), "{error}");
    assert_eq!(store.get_task(task).await.unwrap().agent_id, Some(source));
    assert!(store
        .list_messages(MessageFilter::default(), ListOptions::default())
        .await
        .unwrap()
        .is_empty());
}

async fn start_server(
    status: u16,
    content_type: &str,
    body: Vec<u8>,
    delay: Duration,
) -> (String, tokio::task::JoinHandle<String>) {
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let url = format!("http://{}", listener.local_addr().unwrap());
    let ct = content_type.to_owned();
    let task = tokio::spawn(async move {
        let (mut socket, _) = listener.accept().await.unwrap();
        let mut request = vec![0; 8192];
        let size = socket.read(&mut request).await.unwrap();
        tokio::time::sleep(delay).await;
        let reason = match status {
            200 => "OK",
            404 => "Not Found",
            500 => "Internal Server Error",
            _ => "Error",
        };
        let header=format!("HTTP/1.1 {status} {reason}\r\nContent-Type: {ct}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",body.len());
        let _ = socket.write_all(header.as_bytes()).await;
        let _ = socket.write_all(&body).await;
        String::from_utf8_lossy(&request[..size]).into_owned()
    });
    (url, task)
}

#[tokio::test]
async fn fetch_go_cases_hermetic() {
    let fetch = Fetch::default();
    for (url, message) in [
        ("", "URL is required"),
        ("ftp://example.com", "Invalid URL scheme"),
        ("example.com/page", "Invalid URL scheme"),
    ] {
        assert_eq!(
            fetch
                .execute(&context(), json!({"url":url}))
                .await
                .unwrap_err()
                .message,
            message
        );
    }
    for status in [404, 500] {
        let (url, server) = start_server(status, "text/html", Vec::new(), Duration::ZERO).await;
        assert_eq!(
            fetch
                .execute(&context(), json!({"url":url}))
                .await
                .unwrap_err()
                .message,
            "HTTP request failed"
        );
        server.await.unwrap();
    }
    let (url, server) = start_server(
        200,
        "application/pdf",
        b"PDF content".to_vec(),
        Duration::ZERO,
    )
    .await;
    assert_eq!(
        fetch
            .execute(&context(), json!({"url":url}))
            .await
            .unwrap_err()
            .message,
        "Unsupported content type"
    );
    server.await.unwrap();
    let (url, server) = start_server(
        200,
        "application/json",
        br#"{"name":"test","values":[1,2,3]}"#.to_vec(),
        Duration::ZERO,
    )
    .await;
    let output = fetch
        .execute(
            &context(),
            json!({"url":url,"headers":{"Authorization":"Bearer test-only","X-Custom":"value"}}),
        )
        .await
        .unwrap()
        .fetch
        .unwrap();
    assert_eq!(output.content, r#"{"name":"test","values":[1,2,3]}"#);
    assert_eq!(output.byte_size, 32);
    assert_eq!(output.content_type, "json");
    let headers = server.await.unwrap().to_lowercase();
    assert!(headers.contains("authorization: bearer test-only"));
    assert!(headers.contains("x-custom: value"));
    assert!(headers.contains("constructbot/1.0"));
    assert!(headers.contains("accept-language: en-us,en;q=0.5"));
    let (url, server) = start_server(200, "application/json", b"{}".to_vec(), Duration::ZERO).await;
    fetch.execute(&context(),json!({"url":url.replacen("http:","HTTP:",1),"headers":{"uSeR-aGeNt":"CustomAgent/2.0"}})).await.unwrap();
    let headers = server.await.unwrap().to_lowercase();
    assert!(headers.contains("user-agent: customagent/2.0"));
    assert!(!headers.contains("constructbot"));
    for ct in ["text/html", "application/xhtml+xml"] {
        let(url,server)=start_server(200,ct,b"<!DOCTYPE html><html><head><title>Test Page</title></head><body><article><h1>Hello World</h1><p>This is a test paragraph.</p></article></body></html>".to_vec(),Duration::ZERO).await;
        let result = fetch
            .execute(&context(), json!({"url":url}))
            .await
            .unwrap()
            .fetch
            .unwrap();
        assert_eq!(result.title, "Test Page");
        assert_eq!(result.content_type, "html");
        assert!(result.content.contains("This is a test paragraph."));
        assert!(!result.truncated);
        server.await.unwrap();
    }
}

#[tokio::test]
async fn fetch_limits_timeouts_and_cancellation() {
    let fetch = Fetch::default();
    let (url, task) = start_server(
        200,
        "application/json",
        vec![b' '; MAX_CONTENT_SIZE + 100],
        Duration::ZERO,
    )
    .await;
    let result = fetch
        .execute(&context(), json!({"url":url}))
        .await
        .unwrap()
        .fetch
        .unwrap();
    assert!(result.truncated);
    assert_eq!(result.byte_size, MAX_CONTENT_SIZE as i64);
    task.await.unwrap();
    let (url, task) = start_server(
        200,
        "application/json",
        b"{}".to_vec(),
        Duration::from_secs(2),
    )
    .await;
    assert_eq!(
        fetch
            .execute(&context(), json!({"url":url,"timeout":1}))
            .await
            .unwrap_err()
            .message,
        "Request timed out"
    );
    task.abort();
    let ctx = context();
    ctx.cancellation.cancel();
    assert_eq!(
        fetch
            .execute(&ctx, json!({"url":"http://127.0.0.1:1"}))
            .await
            .unwrap_err()
            .details["error"],
        "context canceled"
    );
}

#[tokio::test]
async fn go_fetch_json_goldens() {
    let cases: Value = serde_json::from_str(include_str!("fixtures/web.json")).unwrap();
    for case in cases.as_array().unwrap() {
        let (url, task) = start_server(
            200,
            case["type"].as_str().unwrap(),
            case["body"].as_str().unwrap().as_bytes().to_vec(),
            Duration::ZERO,
        )
        .await;
        let result = Fetch::default()
            .execute(&context(), json!({"url":url}))
            .await
            .unwrap()
            .fetch
            .unwrap();
        let actual = String::from_utf8(to_go_json(&result).unwrap()).unwrap();
        let expected = case["result_json"]
            .as_str()
            .unwrap()
            .replace("{{URL}}", &url)
            .replace("{{HOST}}", url.trim_start_matches("http://"));
        assert_eq!(actual, expected, "{}", case["name"]);
        task.await.unwrap();
    }
}

#[test]
fn path_not_directory_hint_matches_go() {
    assert!(construct_core::tool::ToolError::new(
        construct_core::tool::ToolErrorCode::PathIsNotDirectory
    )
    .suggestions
    .contains(&"You must specify a directory path, not a file path.".into()));
}
