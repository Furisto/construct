use construct_codeact::{production_interceptors, Interpreter, Task};
use construct_core::{
    event::{Event, EventPublisher},
    tool::ToolContext,
    CancellationToken,
};
use construct_types::{
    to_go_json,
    tools::{InterpreterInput, ToolCallEvent, ToolResultEvent},
};
use serde_json::{json, Value};
use std::{
    fs,
    sync::{Arc, Mutex},
};
use uuid::Uuid;

#[derive(Default)]
struct RecordingPublisher(Mutex<Vec<Value>>);
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

#[tokio::test]
async fn real_go_interpreter_goldens() {
    let cases: Vec<Value> = serde_json::from_str(include_str!("fixtures/go.json")).unwrap();
    let mut failures = Vec::new();
    for case in cases {
        let name = case["name"].as_str().unwrap();
        let dir = tempfile::tempdir().unwrap();
        for (path, content) in case["files"].as_object().unwrap() {
            let path = dir.path().join(path);
            fs::create_dir_all(path.parent().unwrap()).unwrap();
            fs::write(path, content.as_str().unwrap()).unwrap();
        }
        let root = dir.path().to_str().unwrap();
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
            project_directory: dir.path().into(),
        };
        let ctx = ToolContext {
            workspace_root: dir.path().into(),
            task_id: Some(task.id),
            agent_id: None,
            cancellation: CancellationToken::new(),
            events: None,
        };
        let input = InterpreterInput {
            script: case["script"].as_str().unwrap().replace("{{ROOT}}", root),
        };
        let (output, error) = interpreter.interpret(ctx, input, task).await;
        let actual_output = String::from_utf8(to_go_json(&output).unwrap())
            .unwrap()
            .replace(root, "{{ROOT}}");
        let actual_error = error.map(|error| error.to_string().replace(root, "{{ROOT}}"));
        let actual_events: Value = serde_json::from_str(
            &serde_json::to_string(&*events.0.lock().unwrap())
                .unwrap()
                .replace(root, "{{ROOT}}"),
        )
        .unwrap();
        let comparisons = [
            (
                "console_output",
                json!(normalize_details(
                    &output.console_output.replace(root, "{{ROOT}}")
                )),
                json!(normalize_details(case["console_output"].as_str().unwrap())),
            ),
            (
                "error",
                json!(normalize_error(actual_error.as_deref())),
                json!(normalize_error(case["error"].as_str())),
            ),
            (
                "output_json",
                json!(normalize_output(&actual_output)),
                json!(normalize_output(case["output_json"].as_str().unwrap())),
            ),
            ("events", actual_events, case["events"].clone()),
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
