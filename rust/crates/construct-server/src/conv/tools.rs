use construct_proto::construct::v1::{self as pb, tool_call as tc, tool_result as tr};
use construct_types::tools as domain;

/// Convert a domain tool event's input; absent input produces an identifier-only call.
pub fn convert_tool_input_to_proto(
    id: &str,
    tool_name: &str,
    input: Option<&domain::ToolInput>,
) -> pb::ToolCall {
    pb::ToolCall {
        id: id.into(),
        tool_name: tool_name.into(),
        input: input.and_then(tool_input),
    }
}

fn tool_input(input: &domain::ToolInput) -> Option<tc::Input> {
    Some(if let Some(i) = &input.create_file {
        tc::Input::CreateFile(tc::CreateFileInput {
            path: i.path.clone(),
            content: i.content.clone(),
        })
    } else if let Some(i) = &input.edit_file {
        tc::Input::EditFile(tc::EditFileInput {
            path: i.path.clone(),
            diffs: i
                .diffs
                .iter()
                .flatten()
                .map(|d| tc::edit_file_input::DiffPair {
                    old: d.old.clone(),
                    new: d.new.clone(),
                })
                .collect(),
        })
    } else if let Some(i) = &input.execute_command {
        tc::Input::ExecuteCommand(tc::ExecuteCommandInput {
            command: i.command.clone(),
        })
    } else if let Some(i) = &input.find_file {
        tc::Input::FindFile(tc::FindFileInput {
            pattern: i.pattern.clone(),
            path: i.path.clone(),
            exclude_pattern: i.exclude_pattern.clone(),
            max_results: i.max_results as i32,
        })
    } else if let Some(i) = &input.grep {
        tc::Input::Grep(tc::GrepInput {
            query: i.query.clone(),
            path: i.path.clone(),
            include_pattern: i.include_pattern.clone(),
            exclude_pattern: i.exclude_pattern.clone(),
            case_sensitive: i.case_sensitive,
            max_results: i.max_results as i32,
        })
    } else if let Some(i) = &input.list_files {
        tc::Input::ListFiles(tc::ListFilesInput {
            path: i.path.clone(),
            recursive: i.recursive,
        })
    } else if let Some(i) = &input.read_file {
        tc::Input::ReadFile(tc::ReadFileInput {
            path: i.path.clone(),
            start_line: i.start_line.as_deref().copied().unwrap_or_default() as i32,
            end_line: i.end_line.as_deref().copied().unwrap_or_default() as i32,
        })
    } else if let Some(i) = &input.submit_report {
        tc::Input::SubmitReport(tc::SubmitReportInput {
            summary: i.summary.clone(),
            completed: i.completed,
            deliverables: i.deliverables.clone().unwrap_or_default(),
            next_steps: i.next_steps.clone(),
        })
    } else if let Some(i) = &input.ask_user {
        tc::Input::AskUser(tc::AskUserInput {
            question: i.question.clone(),
            options: i.options.clone().unwrap_or_default(),
        })
    } else if let Some(i) = &input.handoff {
        tc::Input::Handoff(tc::HandoffInput {
            requested_agent: i.requested_agent.clone(),
            handover_message: i.handover_message.clone(),
        })
    } else if let Some(i) = &input.fetch {
        tc::Input::Fetch(tc::FetchInput {
            url: i.url.clone(),
            headers: i.headers.clone().unwrap_or_default().into_iter().collect(),
            timeout: i.timeout as i32,
        })
    } else {
        return None;
    })
}

/// Convert a domain tool event's output, preserving Go's unexposed result variants.
pub fn convert_tool_output_to_proto(
    id: &str,
    tool_name: &str,
    output: Option<&domain::ToolOutput>,
) -> pb::ToolResult {
    pb::ToolResult {
        id: id.into(),
        tool_name: tool_name.into(),
        result: output.and_then(tool_output),
        ..Default::default()
    }
}

fn tool_output(output: &domain::ToolOutput) -> Option<tr::Result> {
    Some(if let Some(o) = &output.create_file {
        tr::Result::CreateFile(tr::CreateFileResult {
            overwritten: o.overwritten,
        })
    } else if let Some(o) = &output.edit_file {
        tr::Result::EditFile(tr::EditFileResult {
            path: o.path.clone(),
            patch_info: Some(tr::edit_file_result::PatchInfo {
                patch: o.patch_info.patch.clone(),
                lines_added: o.patch_info.lines_added as i32,
                lines_removed: o.patch_info.lines_removed as i32,
            }),
        })
    } else if let Some(o) = &output.execute_command {
        tr::Result::ExecuteCommand(tr::ExecuteCommandResult {
            stdout: o.stdout.clone(),
            stderr: o.stderr.clone(),
            exit_code: o.exit_code as i32,
            command: o.command.clone(),
        })
    } else if let Some(o) = &output.find_file {
        tr::Result::FindFile(tr::FindFileResult {
            files: o.files.clone().unwrap_or_default(),
            ..Default::default()
        })
    } else if let Some(o) = &output.grep {
        tr::Result::Grep(tr::GrepResult {
            matches: o
                .matches
                .iter()
                .flatten()
                .map(|m| tr::grep_result::GrepMatch {
                    file_path: m.file_path.clone(),
                    value: m.value.clone(),
                })
                .collect(),
            total_matches: o.total_matches as i32,
            truncated_count: o.truncated_matches as i32,
            searched_files: o.searched_files as i32,
        })
    } else if let Some(o) = &output.list_files {
        tr::Result::ListFiles(tr::ListFilesResult {
            path: o.path.clone(),
            entries: o
                .entries
                .iter()
                .flatten()
                .map(|e| tr::list_files_result::DirectoryEntry {
                    name: e.name.clone(),
                    r#type: e.r#type.clone(),
                    size: e.size,
                })
                .collect(),
        })
    } else if let Some(o) = &output.read_file {
        tr::Result::ReadFile(tr::ReadFileResult {
            content: o.content.clone(),
            ..Default::default()
        })
    } else if let Some(o) = &output.submit_report {
        tr::Result::SubmitReport(tr::SubmitReportResult {
            summary: o.summary.clone(),
            completed: o.completed,
            deliverables: o.deliverables.clone().unwrap_or_default(),
            next_steps: o.next_steps.clone(),
        })
    } else if output.ask_user.is_some() {
        return None;
    } else if let Some(o) = &output.fetch {
        tr::Result::Fetch(tr::FetchResult {
            url: o.url.clone(),
            title: o.title.clone(),
            content: o.content.clone(),
            content_type: o.content_type.clone(),
            byte_size: o.byte_size,
            ..Default::default()
        })
    } else {
        return None;
    })
}

pub(super) fn function_call_parts(call: &domain::FunctionCall) -> Vec<pb::MessagePart> {
    let mut input = domain::ToolInput::default();
    let mut output = domain::ToolOutput::default();
    macro_rules! select {
        ($field:ident) => {{
            input.$field = call.input.$field.clone();
            output.$field = call.output.$field.clone();
        }};
    }
    match call.tool_name.as_str() {
        "create_file" => select!(create_file),
        "edit_file" => select!(edit_file),
        "execute_command" => select!(execute_command),
        "find_file" => select!(find_file),
        "grep" => select!(grep),
        "list_files" => select!(list_files),
        "read_file" => select!(read_file),
        "submit_report" => select!(submit_report),
        "ask_user" => select!(ask_user),
        "handoff" => {
            input.handoff = call.input.handoff.clone();
        }
        "fetch" => select!(fetch),
        _ => return Vec::new(),
    }
    let mut tool_call = convert_tool_input_to_proto("", &call.tool_name, Some(&input));
    if tool_call.input.is_none() {
        return Vec::new();
    }
    if let Some(tc::Input::ReadFile(i)) = &mut tool_call.input {
        i.start_line = 0;
        i.end_line = 0;
    }
    let mut parts = vec![pb::MessagePart {
        data: Some(pb::message_part::Data::ToolCall(tool_call)),
    }];
    let mut tool_result = convert_tool_output_to_proto("", &call.tool_name, Some(&output));
    match &mut tool_result.result {
        Some(tr::Result::FindFile(o)) => {
            let source = output.find_file.as_ref().unwrap();
            o.total_files = source.total_files as i32;
            o.truncated_count = source.truncated_count as i32;
        }
        Some(tr::Result::Grep(o)) => o.truncated_count = 0,
        Some(tr::Result::ReadFile(o)) => o.path = output.read_file.as_ref().unwrap().path.clone(),
        Some(tr::Result::Fetch(o)) => {
            o.content_type.clear();
            o.truncated = output.fetch.as_ref().unwrap().truncated;
        }
        _ => {}
    }
    if tool_result.result.is_some() {
        parts.push(pb::MessagePart {
            data: Some(pb::message_part::Data::ToolResult(tool_result)),
        });
    }
    parts
}
