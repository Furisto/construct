use construct_core::{
    tool::{Tool, ToolContext},
    CancellationToken,
};
use construct_tools::{
    filesystem::{CreateFile, EditFile, ListFiles, ReadFile},
    system::ExecuteCommand,
};
use construct_types::to_go_json;
use serde_json::Value;
use std::{fs, path::Path};

fn context() -> ToolContext {
    ToolContext {
        workspace_root: "/unused".into(),
        task_id: None,
        agent_id: None,
        cancellation: CancellationToken::new(),
        events: None,
    }
}

async fn check(tool: &dyn Tool, fixture: &str) {
    let cases: Value = serde_json::from_str(fixture).unwrap();
    for case in cases.as_array().unwrap() {
        let dir = tempfile::tempdir().unwrap();
        let root = dir.path();
        for entry in case["before"].as_array().unwrap() {
            seed(root, entry);
        }
        let mut input = case["input"].clone();
        for name in ["Path", "path", "WorkingDirectory"] {
            if let Some(path) = input[name].as_str().filter(|p| p.starts_with('/')) {
                input[name] = root
                    .join(path.trim_start_matches('/'))
                    .to_string_lossy()
                    .into_owned()
                    .into();
            }
        }
        let result = tool.execute(&context(), input).await;
        let name = case["name"].as_str().unwrap();
        if let Some(expected) = case.get("error") {
            let error = result.expect_err(name);
            assert_eq!(error.message, expected["Message"], "{name}");
            assert_eq!(
                serde_json::json!(error.suggestions),
                expected["Suggestions"],
                "{name}"
            );
            let details = serde_json::json!(error.details)
                .to_string()
                .replace(root.to_str().unwrap(), "");
            assert_eq!(
                serde_json::from_str::<Value>(&details).unwrap(),
                expected["Details"],
                "{name}"
            );
        } else {
            let output = result.unwrap_or_else(|e| panic!("{name}: {e}"));
            let json = String::from_utf8(to_go_json(&output).unwrap())
                .unwrap()
                .replace(root.to_str().unwrap(), "");
            let actual: Value = serde_json::from_str(&json).unwrap();
            assert_eq!(actual[tool.name()], case["result"], "{name}");
            // Compare the exact Go-compatible byte encoding, not only parsed JSON.
            let expected = format!(
                "{{\"{}\":{}}}",
                tool.name(),
                case["result_json"].as_str().unwrap()
            );
            // Rust String cannot distinguish invalid source bytes from U+FFFD.
            assert_eq!(json, expected.replace("\\ufffd", "�"), "{name}");
        }
        for entry in case["after"].as_array().unwrap() {
            let path = root.join(entry["path"].as_str().unwrap().trim_start_matches('/'));
            if entry["directory"].as_bool().unwrap() {
                assert!(path.is_dir(), "{name}: {path:?}");
            } else {
                assert_eq!(fs::read(&path).unwrap(), bytes(entry), "{name}: {path:?}");
            }
        }
    }
}

fn bytes(entry: &Value) -> Vec<u8> {
    if let Some(blob) = entry.get("blob").and_then(Value::as_str) {
        return fs::read(
            Path::new(env!("CARGO_MANIFEST_DIR"))
                .join("tests/fixtures/blobs")
                .join(blob),
        )
        .unwrap();
    }
    entry["bytes"]
        .as_array()
        .map(|a| a.iter().map(|v| v.as_u64().unwrap() as u8).collect())
        .unwrap_or_default()
}

fn seed(root: &Path, entry: &Value) {
    let path = root.join(entry["path"].as_str().unwrap().trim_start_matches('/'));
    if entry["directory"].as_bool().unwrap() {
        fs::create_dir_all(path).unwrap();
    } else {
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(path, bytes(entry)).unwrap();
    }
}

#[tokio::test]
async fn go_create_file_18_cases() {
    check(
        &CreateFile,
        include_str!("fixtures/TestGoldenCreateFile.json"),
    )
    .await;
}
#[tokio::test]
async fn go_read_file_22_cases() {
    check(&ReadFile, include_str!("fixtures/TestGoldenReadFile.json")).await;
}
#[tokio::test]
async fn go_list_files_10_cases() {
    check(
        &ListFiles,
        include_str!("fixtures/TestGoldenListFiles.json"),
    )
    .await;
}
#[tokio::test]
async fn go_edit_file_5_cases_including_go_skipped_cases() {
    check(&EditFile, include_str!("fixtures/TestGoldenEditFile.json")).await;
}
#[tokio::test]
async fn go_execute_command_10_cases() {
    check(
        &ExecuteCommand,
        include_str!("fixtures/TestGoldenExecuteCommand.json"),
    )
    .await;
}
