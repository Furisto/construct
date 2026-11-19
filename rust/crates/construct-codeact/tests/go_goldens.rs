use construct_codeact::{production_interceptors, FunctionCallState, Interpreter, Task};
use construct_core::{
    event::{Event, EventPublisher},
    tool::{Tool, ToolContext, ToolError},
    CancellationToken,
};
use construct_types::{
    to_go_json,
    tools::{InterpreterInput, InterpreterOutput, ToolCallEvent, ToolOutput, ToolResultEvent},
};
use serde_json::{json, Value};
use std::{
    fs,
    io::{Read, Write},
    net::{TcpListener, TcpStream},
    sync::atomic::{AtomicBool, AtomicUsize, Ordering},
    sync::{Arc, Mutex},
    thread,
    time::Duration,
};
use uuid::Uuid;

#[derive(Default)]
struct RecordingPublisher(Mutex<Vec<Value>>);

struct LoopbackServer {
    port: u16,
    stop: Arc<AtomicBool>,
    worker: Option<thread::JoinHandle<()>>,
}

impl LoopbackServer {
    fn new(responses: Value) -> Self {
        let listener = TcpListener::bind(("127.0.0.1", 0)).unwrap();
        let port = listener.local_addr().unwrap().port();
        listener.set_nonblocking(true).unwrap();
        let stop = Arc::new(AtomicBool::new(false));
        let stopped = stop.clone();
        let worker = thread::spawn(move || {
            while !stopped.load(Ordering::Relaxed) {
                let (mut stream, _) = match listener.accept() {
                    Ok(connection) => connection,
                    Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                        thread::sleep(Duration::from_millis(1));
                        continue;
                    }
                    Err(e) => panic!("loopback accept: {e}"),
                };
                // BSD sockets inherit O_NONBLOCK from the listener; Linux does not.
                stream.set_nonblocking(false).unwrap();
                stream
                    .set_read_timeout(Some(Duration::from_secs(2)))
                    .unwrap();
                let mut request = Vec::new();
                let mut buffer = [0; 1024];
                while !request.ends_with(b"\r\n\r\n") {
                    match stream.read(&mut buffer) {
                        Ok(0) => break,
                        Ok(n) => request.extend_from_slice(&buffer[..n]),
                        Err(_) => break,
                    }
                }
                if request.is_empty() {
                    continue;
                }
                let request = String::from_utf8(request).unwrap();
                let path = request.split_whitespace().nth(1).unwrap();
                let response = &responses[path];
                let status = response["status"].as_u64().unwrap();
                let reason = match status {
                    200 => "OK",
                    503 => "Service Unavailable",
                    _ => panic!("unexpected fixture status"),
                };
                let body = response["body"].as_str().unwrap();
                write!(stream, "HTTP/1.1 {status} {reason}\r\nContent-Type: {}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}", response["content_type"].as_str().unwrap(), body.len()).unwrap();
            }
        });
        Self {
            port,
            stop,
            worker: Some(worker),
        }
    }

    fn normalize(&self, text: &str) -> String {
        text.replace(&format!(":{}", self.port), ":{{PORT}}")
    }
}

impl Drop for LoopbackServer {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Relaxed);
        let _ = TcpStream::connect(("127.0.0.1", self.port));
        self.worker.take().unwrap().join().unwrap();
    }
}
impl EventPublisher for RecordingPublisher {
    fn publish(&self, event: Event) {
        let (tool, payload_json) = match event.r#type.as_str() {
            "tool.called" => {
                let call = event
                    .payload
                    .downcast_ref::<ToolCallEvent>()
                    .expect("typed call payload");
                (
                    call.tool.clone(),
                    String::from_utf8(to_go_json(call).unwrap()).unwrap(),
                )
            }
            "tool.result" => {
                let result = event
                    .payload
                    .downcast_ref::<ToolResultEvent>()
                    .expect("typed result payload");
                (
                    result.tool.clone(),
                    String::from_utf8(to_go_json(result).unwrap()).unwrap(),
                )
            }
            name => panic!("unexpected event type {name}"),
        };
        self.0.lock().unwrap().push(json!({"type": event.r#type, "task_id": event.task_id, "action": event.action, "tool": tool, "payload_json": payload_json}));
    }
}

// Go ranges over error detail maps in randomized order. Only sort contiguous
// detail lines; do not reorder suggestions, ordinary console lines or payloads.
fn normalize_details(text: &str) -> String {
    let mut lines = text.split('\n').peekable();
    let mut output = Vec::new();
    while let Some(line) = lines.next() {
        output.push(line);
        if line == "Details:" {
            let mut details = Vec::new();
            while lines.peek().is_some_and(|line| line.starts_with("- ")) {
                details.push(lines.next().unwrap());
            }
            details.sort_unstable();
            output.extend(details);
        }
    }
    output.join("\n")
}

fn normalize_error(text: Option<&str>) -> Option<String> {
    text.map(|text| {
        let message = text
            .rsplit_once(" at <eval>:")
            .filter(|(_, location)| {
                let Some((line, rest)) = location.split_once(':') else {
                    return false;
                };
                let Some((column, pc)) = rest.split_once('(') else {
                    return false;
                };
                let Some(pc) = pc.strip_suffix(')') else {
                    return false;
                };
                [line, column, pc]
                    .iter()
                    .all(|number| !number.is_empty() && number.bytes().all(|b| b.is_ascii_digit()))
            })
            .map_or(text, |(message, _)| message);
        normalize_details(message)
    })
}

#[test]
fn normalization_preserves_semantic_and_encoding_differences() {
    assert_eq!(
        normalize_error(Some("TypeError: bad input at <eval>:2:24(6)")),
        Some("TypeError: bad input".into())
    );
    assert_eq!(
        normalize_error(Some("Error: text at <eval>:not a location")),
        Some("Error: text at <eval>:not a location".into())
    );
    assert_ne!(
        normalize_error(Some("TypeError: a")),
        normalize_error(Some("TypeError: b"))
    );
    assert_eq!(
        normalize_details("Suggestions:\n- z\n- a\n\nDetails:\n- z: 2\n- a: 1\nend"),
        "Suggestions:\n- z\n- a\n\nDetails:\n- a: 1\n- z: 2\nend"
    );
    let original = r#"{"console_output":"","function_calls":[],"tool_stats":{"z":1,"a":2}}"#;
    assert_eq!(normalize_output(original), original);
}

fn normalize_output(text: &str) -> String {
    let output: Value = serde_json::from_str(text).unwrap();
    let console = output["console_output"].as_str().unwrap();
    let original = String::from_utf8(to_go_json(console).unwrap()).unwrap();
    let normalized = String::from_utf8(to_go_json(&normalize_details(console)).unwrap()).unwrap();
    let prefix = format!("{{\"console_output\":{original},");
    assert!(
        text.starts_with(&prefix),
        "Go declaration order and console encoding"
    );
    text.replacen(&prefix, &format!("{{\"console_output\":{normalized},"), 1)
}

/// macOS launch constraints kill copied platform binaries, so link instead.
#[cfg(unix)]
fn link_executable(source: &std::path::Path, target: &std::path::Path) {
    std::os::unix::fs::symlink(source, target).unwrap();
}

#[cfg(unix)]
#[test]
fn go_goldens_without_ripgrep() {
    let bin = tempfile::tempdir().unwrap();
    for name in ["grep", "sh"] {
        let executable = std::env::split_paths(&std::env::var_os("PATH").unwrap())
            .map(|path| path.join(name))
            .find(|path| path.is_file())
            .unwrap_or_else(|| panic!("system {name}"));
        link_executable(&executable, &bin.path().join(name));
    }
    let output = std::process::Command::new(std::env::current_exe().unwrap())
        .args(["--exact", "real_go_interpreter_goldens", "--nocapture"])
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

#[tokio::test]
async fn real_go_interpreter_goldens() {
    let cases: Vec<Value> = serde_json::from_str(include_str!("fixtures/go.json")).unwrap();
    let divergences: Value = serde_json::from_str(include_str!("fixtures/once.json")).unwrap();
    for (name, expected) in divergences.as_object().unwrap() {
        let case = cases.iter().find(|case| case["name"] == *name).unwrap();
        assert!(expected["known_divergence"]
            .as_str()
            .is_some_and(|reason| !reason.is_empty()));
        assert_ne!(
            expected["output_json"], case["output_json"],
            "{name} must remain a Go divergence"
        );
    }
    let mut failures = Vec::new();
    let windows_path = regex::Regex::new(r"\{\{ROOT\}\}(?:\\+[\w.-]+)+").unwrap();
    let backslashes = regex::Regex::new(r"\\+").unwrap();
    for case in cases {
        let name = case["name"].as_str().unwrap();
        if cfg!(windows) && matches!(name, "command_success" | "nested_side_effect") {
            continue;
        }
        let expected = divergences.get(name).unwrap_or(&case);
        let dir = tempfile::tempdir().unwrap();
        for (path, content) in case["files"].as_object().unwrap() {
            let path = dir.path().join(path);
            fs::create_dir_all(path.parent().unwrap()).unwrap();
            fs::write(path, content.as_str().unwrap()).unwrap();
        }
        let root = fixture_root(dir.path());
        let root = root.as_str();
        let server = LoopbackServer::new(case["http_responses"].clone());
        let normalize = |text: &str| {
            let text = server.normalize(&text.replace(root, "{{ROOT}}"));
            if cfg!(windows) {
                windows_path
                    .replace_all(&text, |m: &regex::Captures<'_>| {
                        backslashes.replace_all(&m[0], "/").into_owned()
                    })
                    .into_owned()
            } else {
                text
            }
        };
        let events = Arc::new(RecordingPublisher::default());
        let interpreter = Interpreter::new(
            vec![
                construct_codeact::create_file(),
                construct_codeact::read_file(),
                construct_codeact::edit_file(),
                construct_codeact::list_files(),
                construct_codeact::grep(),
                construct_codeact::find_file(),
                construct_codeact::execute_command(),
                construct_codeact::fetch(),
                construct_codeact::print(),
            ],
            production_interceptors(events.clone()),
        );
        let task = Task {
            id: Uuid::from_u128(1),
            project_directory: root.into(),
        };
        let ctx = ToolContext {
            workspace_root: root.into(),
            task_id: Some(task.id),
            agent_id: None,
            cancellation: CancellationToken::new(),
            events: None,
        };
        let input = InterpreterInput {
            script: case["script"]
                .as_str()
                .unwrap()
                .replace("{{ROOT}}", root)
                .replace("{{PORT}}", &server.port.to_string()),
        };
        let (output, error) = interpreter.interpret(ctx, input, task).await;
        if name == "nested_side_effect" {
            assert_eq!(
                fs::read_to_string(dir.path().join("effects.txt")).unwrap(),
                "x"
            );
        }
        let actual_output = normalize(&String::from_utf8(to_go_json(&output).unwrap()).unwrap());
        let actual_error = error.map(|error| normalize(&error.to_string()));
        let actual_events: Value = serde_json::from_str(&normalize(
            &serde_json::to_string(&*events.0.lock().unwrap()).unwrap(),
        ))
        .unwrap();
        let comparisons = [
            (
                "console_output",
                json!(normalize_details(&normalize(&output.console_output))),
                json!(normalize_details(
                    expected["console_output"].as_str().unwrap()
                )),
            ),
            (
                "error",
                json!(normalize_error(actual_error.as_deref())),
                json!(normalize_error(expected["error"].as_str())),
            ),
            (
                "output_json",
                json!(normalize_output(&actual_output)),
                json!(normalize_output(expected["output_json"].as_str().unwrap())),
            ),
            ("events", actual_events, expected["events"].clone()),
        ];
        for (field, actual, expected) in comparisons {
            if actual != expected {
                failures.push(format!(
                    "{name}.{field}\nactual:   {actual}\nexpected: {expected}"
                ));
            }
        }
    }
    assert!(failures.is_empty(), "{}", failures.join("\n\n"));
}

struct RejectExecution {
    name: &'static str,
    attempts: Arc<AtomicUsize>,
}

#[construct_core::async_trait]
impl Tool for RejectExecution {
    fn name(&self) -> &str {
        self.name
    }
    fn description(&self) -> &str {
        "execution must be replayed"
    }
    fn input_schema(&self) -> Value {
        json!({})
    }
    async fn execute(&self, _ctx: &ToolContext, _input: Value) -> Result<ToolOutput, ToolError> {
        self.attempts.fetch_add(1, Ordering::SeqCst);
        Err(ToolError::custom("unexpected host execution", vec![]))
    }
}

fn scope(root: &std::path::Path) -> (ToolContext, Task) {
    let task = Task {
        id: Uuid::from_u128(1),
        project_directory: root.into(),
    };
    let ctx = ToolContext {
        workspace_root: root.into(),
        task_id: Some(task.id),
        agent_id: None,
        cancellation: CancellationToken::new(),
        events: None,
    };
    (ctx, task)
}

fn fixture_root(path: &std::path::Path) -> String {
    let root = path.to_str().unwrap();
    if cfg!(windows) {
        root.replace('\\', "/")
    } else {
        root.to_owned()
    }
}

#[tokio::test]
async fn resume_real_go_duplicate_records_without_executing_hosts() {
    let cases: Vec<Value> = serde_json::from_str(include_str!("fixtures/go.json")).unwrap();
    let expected: Value = serde_json::from_str(include_str!("fixtures/once.json")).unwrap();
    for (source, script, expectation) in [
        (
            "nested_coercion_fresh",
            "nested_coercion_fresh",
            "nested_coercion_fresh",
        ),
        (
            "nested_coercion_existing",
            "nested_coercion_existing",
            "nested_coercion_existing",
        ),
        (
            "nested_late_existing",
            "nested_late_existing",
            "nested_late_existing",
        ),
        (
            "nested_late_existing",
            "nested_coercion_existing",
            "nested_coercion_existing",
        ),
        (
            "nested_late_fresh",
            "nested_late_fresh",
            "nested_late_fresh",
        ),
    ] {
        let dir = tempfile::tempdir().unwrap();
        let root = fixture_root(dir.path());
        let root = root.as_str();
        let source = cases.iter().find(|case| case["name"] == source).unwrap();
        let stored: InterpreterOutput = serde_json::from_str(
            &source["output_json"]
                .as_str()
                .unwrap()
                .replace("{{ROOT}}", root),
        )
        .unwrap();
        let calls = stored.function_calls.unwrap();
        if source["name"] == "nested_coercion_fresh" {
            assert_eq!(
                calls.iter().map(|call| call.index).collect::<Vec<_>>(),
                [0, 1, 2, 1]
            );
        } else if source["name"] == "nested_late_existing" {
            assert_eq!(
                calls.iter().map(|call| call.index).collect::<Vec<_>>(),
                [0, 1, 1]
            );
        }
        let state = FunctionCallState {
            index: calls.len() as i64,
            calls,
        };
        let script = cases.iter().find(|case| case["name"] == script).unwrap()["script"]
            .as_str()
            .unwrap()
            .replace("{{ROOT}}", root);
        let attempts = Arc::new(AtomicUsize::new(0));
        let events = Arc::new(RecordingPublisher::default());
        let mut tools = ["read_file", "list_files"]
            .into_iter()
            .map(|name| {
                construct_codeact::host_tool(Arc::new(RejectExecution {
                    name,
                    attempts: attempts.clone(),
                }))
            })
            .collect::<Vec<_>>();
        tools.push(construct_codeact::print());
        let interpreter = Interpreter::new(tools, production_interceptors(events.clone()));
        let (ctx, task) = scope(dir.path());
        let (output, error) = interpreter
            .resume(ctx, InterpreterInput { script }, task, state)
            .await;
        assert_eq!(error, None, "{}", source["name"]);
        assert_eq!(attempts.load(Ordering::SeqCst), 0);
        let actual = String::from_utf8(to_go_json(&output).unwrap())
            .unwrap()
            .replace(root, "{{ROOT}}");
        assert_eq!(
            actual,
            expected[expectation]["output_json"].as_str().unwrap()
        );
        let actual: Value = serde_json::from_str(
            &serde_json::to_string(&*events.0.lock().unwrap())
                .unwrap()
                .replace(root, "{{ROOT}}"),
        )
        .unwrap();
        assert_eq!(actual, expected[expectation]["events"]);
    }
}

#[tokio::test]
async fn resume_empty_exhausted_mismatched_and_invalid_records() {
    let dir = tempfile::tempdir().unwrap();
    let marker = dir.path().join("effects.txt");
    let append = |content| {
        let path = marker.display();
        let command = if cfg!(windows) {
            format!("[IO.File]::AppendAllText('{path}', '{content}')")
        } else {
            format!("printf {content} >> '{path}'")
        };
        format!("execute_command({});", json!(command))
    };
    let first = append("a");
    let second = append("b");
    let events = Arc::new(RecordingPublisher::default());
    let interpreter = Interpreter::new(
        vec![construct_codeact::execute_command()],
        production_interceptors(events.clone()),
    );
    let (ctx, task) = scope(dir.path());
    let (output, error) = interpreter
        .resume(
            ctx,
            InterpreterInput {
                script: first.clone(),
            },
            task,
            FunctionCallState::default(),
        )
        .await;
    assert_eq!(error, None);
    assert_eq!(fs::read_to_string(&marker).unwrap(), "a");
    let saved = output.function_calls.unwrap();
    events.0.lock().unwrap().clear();
    let (ctx, task) = scope(dir.path());
    let (output, error) = interpreter
        .resume(
            ctx,
            InterpreterInput {
                script: format!("{first}{second}"),
            },
            task,
            FunctionCallState {
                calls: saved.clone(),
                index: i64::MAX,
            },
        )
        .await;
    assert_eq!(error, None);
    assert_eq!(fs::read_to_string(&marker).unwrap(), "ab");
    let calls = output.function_calls.unwrap();
    assert_eq!(
        calls.iter().map(|call| call.index).collect::<Vec<_>>(),
        [0, 1]
    );
    assert_eq!(output.tool_stats.unwrap()["execute_command"], 2);
    assert_eq!(events.0.lock().unwrap().len(), 4);

    for invalid_output in [false, true] {
        events.0.lock().unwrap().clear();
        let mut calls = saved.clone();
        let script = if invalid_output {
            calls[0].output = ToolOutput::default();
            first.clone()
        } else {
            second.clone()
        };
        let (ctx, task) = scope(dir.path());
        let (output, error) = interpreter
            .resume(
                ctx,
                InterpreterInput { script },
                task,
                FunctionCallState { calls, index: -1 },
            )
            .await;
        let message = if invalid_output {
            "stored tool output is missing its result arm"
        } else {
            "durable replay input does not match stored calls"
        };
        assert!(error.unwrap().0.starts_with(message));
        assert_eq!(fs::read_to_string(&marker).unwrap(), "ab");
        assert!(output.function_calls.unwrap().is_empty());
        assert_eq!(output.tool_stats.unwrap()["execute_command"], 1);
        let events = events.0.lock().unwrap();
        assert_eq!(events.len(), 1);
        assert_eq!(events[0]["type"], "tool.called");
    }
}
