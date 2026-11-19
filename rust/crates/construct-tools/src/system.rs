//! Shell command execution with combined output and cancellation.
use crate::{custom, decode, description, schema};
use construct_core::tool::{Tool, ToolContext, ToolError, ToolErrorCode};
use construct_types::tools::{ExecuteCommandInput, ExecuteCommandResult, ToolOutput};
use serde_json::Value;
use std::process::Stdio;

/// Executes Go's `set -eu` wrapper with `/bin/sh -c`.
#[derive(Clone, Copy, Debug, Default)]
pub struct ExecuteCommand;

#[construct_core::async_trait]
impl Tool for ExecuteCommand {
    fn name(&self) -> &str {
        "execute_command"
    }
    fn description(&self) -> &str {
        description(self.name())
    }
    fn input_schema(&self) -> Value {
        schema(self.name())
    }
    async fn execute(&self, ctx: &ToolContext, value: Value) -> Result<ToolOutput, ToolError> {
        let input: ExecuteCommandInput = decode(value)?;
        if input.command.is_empty() {
            let mut error = ToolError::new(ToolErrorCode::InvalidArgument);
            error
                .details
                .insert("command".into(), "command is required".into());
            return Err(error);
        }
        let fail = |error: String, output: String| {
            custom(
                "error executing command",
                &[
                    "Check if the command is valid and executable.",
                    "Ensure the command is properly formatted for the target operating system.",
                ],
                &[
                    ("command", input.command.clone().into()),
                    ("error", error.into()),
                    ("output", output.into()),
                ],
            )
        };
        if ctx.cancellation.is_cancelled() {
            return Err(fail("context canceled".into(), String::new()));
        }
        let output = tempfile::tempfile().map_err(|e| fail(e.to_string(), String::new()))?;
        let stderr = output
            .try_clone()
            .map_err(|e| fail(e.to_string(), String::new()))?;
        let stdout = output
            .try_clone()
            .map_err(|e| fail(e.to_string(), String::new()))?;
        let script = format!("#!/bin/sh\n\t\tset -eu\n\t\t{}\n\t\t", input.command);
        let mut command = tokio::process::Command::new("/bin/sh");
        command
            .args(["-c", &script])
            .stdout(Stdio::from(stdout))
            .stderr(Stdio::from(stderr))
            .kill_on_drop(true);
        if !input.working_directory.is_empty() {
            command.current_dir(&input.working_directory);
        }
        let mut child = command
            .spawn()
            .map_err(|e| fail(e.to_string(), String::new()))?;
        let status = tokio::select! {
            status = child.wait() => status.map_err(|e| fail(e.to_string(), String::new()))?,
            () = ctx.cancellation.cancelled() => {
                child.kill().await.map_err(|e|fail(e.to_string(),String::new()))?;
                child.wait().await.map_err(|e|fail(e.to_string(),String::new()))?
            },
        };
        let bytes = tokio::task::spawn_blocking(move || {
            use std::io::{Read, Seek, SeekFrom};
            let mut output = output;
            output.seek(SeekFrom::Start(0))?;
            let mut bytes = Vec::new();
            output.read_to_end(&mut bytes)?;
            Ok::<_, std::io::Error>(bytes)
        })
        .await
        .map_err(|e| fail(e.to_string(), String::new()))?
        .map_err(|e| fail(e.to_string(), String::new()))?;
        let stdout = crate::go_string(&bytes);
        if !status.success() {
            return Err(fail(crate::process_error(status), stdout));
        }
        Ok(ToolOutput {
            execute_command: Some(Box::new(ExecuteCommandResult {
                command: input.command,
                stdout,
                stderr: String::new(),
                exit_code: status.code().unwrap_or(0) as i64,
            })),
            ..Default::default()
        })
    }
}
