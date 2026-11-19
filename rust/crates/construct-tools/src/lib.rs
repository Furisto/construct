//! Go-compatible filesystem, system, web and communication tools.
#![deny(missing_docs)]

pub mod communication;
pub mod filesystem;
pub mod system;
pub mod web;

use construct_core::tool::{ToolError, ToolErrorCode};
use serde::de::DeserializeOwned;
use serde_json::Value;

fn custom(
    message: impl Into<String>,
    suggestions: &[&str],
    details: &[(&str, Value)],
) -> ToolError {
    let mut error = ToolError::custom(message, suggestions.iter().map(|s| (*s).into()).collect());
    error
        .details
        .extend(details.iter().map(|(k, v)| ((*k).into(), v.clone())));
    error
}

fn standard(code: ToolErrorCode, path: &str) -> ToolError {
    let mut error = ToolError::new(code);
    error.details.insert("path".into(), path.into());
    error
}

fn decode<T: DeserializeOwned>(mut input: Value) -> Result<T, ToolError> {
    // encoding/json accepts case-insensitive field names, including untagged Go fields.
    if let Some(object) = input.as_object_mut() {
        object.retain(|_, value| !value.is_null());
        for (key, value) in object.clone() {
            object.entry(key.to_ascii_lowercase()).or_insert(value);
        }
        for (lower, upper) in [
            ("path", "Path"),
            ("content", "Content"),
            ("recursive", "Recursive"),
            ("command", "Command"),
            ("workingdirectory", "WorkingDirectory"),
            ("summary", "Summary"),
            ("completed", "Completed"),
            ("deliverables", "Deliverables"),
            ("nextsteps", "NextSteps"),
            ("taskid", "TaskID"),
            ("currentagentid", "CurrentAgentID"),
            ("requestedagent", "RequestedAgent"),
            ("handovermessage", "HandoverMessage"),
        ] {
            if let Some(value) = object.get(lower).cloned() {
                object.entry(upper).or_insert(value);
            }
            if let Some(value) = object.get(upper).cloned() {
                object.entry(lower).or_insert(value);
            }
        }
    }
    serde_json::from_value(input)
        .map_err(|e| custom("Invalid input", &[], &[("error", e.to_string().into())]))
}

fn go_string(mut bytes: &[u8]) -> String {
    let mut result = String::new();
    while !bytes.is_empty() {
        match std::str::from_utf8(bytes) {
            Ok(valid) => {
                result.push_str(valid);
                break;
            }
            Err(error) => {
                let valid = error.valid_up_to();
                result.push_str(std::str::from_utf8(&bytes[..valid]).expect("valid UTF-8 prefix"));
                result.push('\u{fffd}');
                bytes = &bytes[valid + 1..];
            }
        }
    }
    result
}

fn process_error(status: std::process::ExitStatus) -> String {
    if let Some(code) = status.code() {
        return format!("exit status {code}");
    }
    #[cfg(unix)]
    {
        use std::os::unix::process::ExitStatusExt;
        let signal = status.signal().unwrap_or(0);
        let name = match signal {
            1 => "hangup",
            2 => "interrupt",
            3 => "quit",
            4 => "illegal instruction",
            6 => "aborted",
            8 => "floating point exception",
            9 => "killed",
            11 => "segmentation fault",
            13 => "broken pipe",
            14 => "alarm clock",
            15 => "terminated",
            _ => return format!("signal: signal {signal}"),
        };
        format!(
            "signal: {name}{}",
            if status.core_dumped() {
                " (core dumped)"
            } else {
                ""
            }
        )
    }
    #[cfg(not(unix))]
    {
        status.to_string()
    }
}

fn schema(name: &str) -> Value {
    serde_json::from_str(match name {
        "create_file" => include_str!("schemas/create_file.json"),
        "edit_file" => include_str!("schemas/edit_file.json"),
        "read_file" => include_str!("schemas/read_file.json"),
        "list_files" => include_str!("schemas/list_files.json"),
        "find_file" => include_str!("schemas/find_file.json"),
        "grep" => include_str!("schemas/grep.json"),
        "execute_command" => include_str!("schemas/execute_command.json"),
        "fetch" => include_str!("schemas/fetch.json"),
        "ask_user" => include_str!("schemas/ask_user.json"),
        "submit_report" => include_str!("schemas/submit_report.json"),
        "handoff" => include_str!("schemas/handoff.json"),
        _ => unreachable!(),
    })
    .expect("embedded schema")
}

fn description(name: &str) -> &'static str {
    match name {
        "create_file" => include_str!("descriptions/create_file.md"),
        "edit_file" => include_str!("descriptions/edit_file.md"),
        "read_file" => include_str!("descriptions/read_file.md"),
        "list_files" => include_str!("descriptions/list_files.md"),
        "find_file" => include_str!("descriptions/find_file.md"),
        "grep" => include_str!("descriptions/grep.md"),
        "execute_command" => include_str!("descriptions/execute_command.md"),
        "fetch" => include_str!("descriptions/fetch.md"),
        "ask_user" => include_str!("descriptions/ask_user.md"),
        "submit_report" => include_str!("descriptions/submit_report.md"),
        "handoff" => include_str!("descriptions/handoff.md"),
        _ => unreachable!(),
    }
}
