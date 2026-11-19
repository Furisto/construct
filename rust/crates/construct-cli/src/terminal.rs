//! Transport and presentation data shared by interactive and scripted sessions.
use crate::{app::ApiClient, Result};
use construct_connect::Request;
use construct_proto::construct::v1::*;
use serde::Serialize;
use std::collections::BTreeMap;

pub async fn resolve_agent(client: &ApiClient, name: &str) -> Result<String> {
    if uuid::Uuid::parse_str(name).is_ok() {
        return Ok(name.into());
    }
    let agents = client
        .agent
        .list_agents(Request::new(ListAgentsRequest {
            filter: Some(list_agents_request::Filter {
                names: vec![name.into()],
            }),
            ..Default::default()
        }))
        .await
        .map_err(|e| format!("failed to list agents: {e}"))?
        .message
        .agents;
    match agents.as_slice() {
        [] => Err(format!("agent {name} not found").into()),
        [agent] => Ok(agent
            .metadata
            .as_ref()
            .ok_or("missing agent metadata")?
            .id
            .clone()),
        _ => Err(format!("multiple agents found for {name}").into()),
    }
}
pub async fn get_task(client: &ApiClient, id: &str) -> Result<Task> {
    client
        .task
        .get_task(Request::new(GetTaskRequest { id: id.into() }))
        .await
        .map_err(|e| format!("failed to get task: {e}"))?
        .message
        .task
        .ok_or_else(|| "missing task".into())
}
pub async fn continue_task(client: &ApiClient, id: &str) -> Result<Task> {
    if id != "last" {
        return get_task(client, id).await;
    }
    client
        .task
        .list_tasks(Request::new(ListTasksRequest {
            sort_field: Some(SortField::UpdatedAt as i32),
            sort_order: Some(SortOrder::Desc as i32),
            ..Default::default()
        }))
        .await
        .map_err(|e| format!("failed to list tasks: {e}"))?
        .message
        .tasks
        .into_iter()
        .next()
        .ok_or_else(|| "no tasks found".into())
}
pub async fn create_task(client: &ApiClient, agent: &str, workspace: &str) -> Result<Task> {
    client
        .task
        .create_task(Request::new(CreateTaskRequest {
            agent_id: agent.into(),
            project_directory: workspace.into(),
            ..Default::default()
        }))
        .await
        .map_err(|e| format!("failed to create task: {e}"))?
        .message
        .task
        .ok_or_else(|| "missing task".into())
}
pub async fn send_message(client: &ApiClient, id: &str, content: &str) -> Result<Message> {
    client
        .message
        .create_message(Request::new(CreateMessageRequest {
            task_id: id.into(),
            content: vec![MessagePart {
                data: Some(message_part::Data::Text(message_part::Text {
                    content: content.into(),
                })),
            }],
        }))
        .await
        .map_err(|e| format!("failed to send message: {e}"))?
        .message
        .message
        .ok_or_else(|| "missing message".into())
}
pub fn text_parts(message: &Message) -> Vec<&str> {
    message
        .spec
        .iter()
        .flat_map(|s| &s.content)
        .filter_map(|p| match &p.data {
            Some(message_part::Data::Text(t)) => Some(t.content.as_str()),
            _ => None,
        })
        .collect()
}
pub fn is_complete_response(message: &Message) -> bool {
    // persistModelResponse marks assistant responses without tool calls processed immediately.
    // Phase alone is unsafe: reconciliation defers an awaiting update even between tool turns.
    message
        .metadata
        .as_ref()
        .is_some_and(|m| m.role == MessageRole::Assistant as i32)
        && (message.status.as_ref().is_some_and(|s| s.is_final_response)
            || message.spec.as_ref().is_some_and(|s| {
                !s.content
                    .iter()
                    .any(|p| matches!(p.data, Some(message_part::Data::ToolCall(_))))
            }))
}

#[derive(Debug, Serialize)]
pub struct DisplayUsage {
    pub input_tokens: i64,
    pub output_tokens: i64,
    pub cache_write_tokens: i64,
    pub cache_read_tokens: i64,
    pub cost: f64,
    pub tool_uses: Option<BTreeMap<String, i64>>,
}
#[derive(Debug, Serialize)]
pub struct DisplayAnswer {
    pub task_id: String,
    pub agent: String,
    pub model: String,
    pub turn: i64,
    pub result: String,
    pub usage: DisplayUsage,
}
impl DisplayAnswer {
    pub fn new(task: &Task, message: &Message) -> Self {
        let usage = task.status.as_ref().and_then(|s| s.usage.as_ref());
        Self {
            task_id: task
                .metadata
                .as_ref()
                .map(|m| m.id.clone())
                .unwrap_or_default(),
            agent: task
                .spec
                .as_ref()
                .and_then(|s| s.agent_id.clone())
                .unwrap_or_default(),
            model: String::new(),
            turn: task.status.as_ref().map_or(0, |s| s.turn),
            result: text_parts(message)
                .last()
                .copied()
                .unwrap_or_default()
                .into(),
            usage: DisplayUsage {
                input_tokens: usage.map_or(0, |u| u.input_tokens),
                output_tokens: usage.map_or(0, |u| u.output_tokens),
                cache_write_tokens: usage.map_or(0, |u| u.cache_write_tokens),
                cache_read_tokens: usage.map_or(0, |u| u.cache_read_tokens),
                cost: usage.map_or(0., |u| u.cost),
                tool_uses: usage
                    .map(|u| u.tool_uses.iter().map(|(k, v)| (k.clone(), *v)).collect()),
            },
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum AppState {
    Normal,
    Waiting,
    Error,
    Help,
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum UiMode {
    Input,
    Scroll,
}
#[derive(Clone, Debug)]
pub enum FeedContent {
    User(String),
    Assistant(String),
    ToolCall(ToolCall),
    ToolResult(ToolResult),
    Typing,
    SubmitReport(SubmitReport),
    Error(String),
}
#[derive(Clone, Debug)]
pub struct FeedMessage {
    pub content: FeedContent,
    pub timestamp: chrono::DateTime<chrono::Utc>,
}
#[derive(Clone, Debug)]
pub enum SessionEvent {
    Message(Message),
    Chunk(MessageChunkEvent),
    ToolCalled(ToolCalledEvent),
    ToolResult(ToolResultEvent),
    TaskUpdated(Task),
    Error(String),
}

pub fn tool_call_label(call: &ToolCall) -> Option<(String, String)> {
    use tool_call::Input;
    let (name, input) = match call.input.as_ref()? {
        Input::ReadFile(i) => (
            "Read",
            if i.start_line != 0 && i.end_line != 0 {
                format!("{} L{}-{}", i.path, i.start_line, i.end_line)
            } else {
                i.path.clone()
            },
        ),
        Input::CreateFile(i) => ("Create", i.path.clone()),
        Input::EditFile(i) => ("Edit", i.path.clone()),
        Input::ExecuteCommand(i) => ("Execute", i.command.clone()),
        Input::FindFile(i) => {
            let path = if i.path.is_empty() { "." } else { &i.path };
            let path = if path.len() > 50 {
                format!(
                    "{}...",
                    path.chars()
                        .rev()
                        .take(50)
                        .collect::<String>()
                        .chars()
                        .rev()
                        .collect::<String>()
                )
            } else {
                path.into()
            };
            let exclude = if i.exclude_pattern.is_empty() {
                "none".into()
            } else if i.exclude_pattern.len() > 50 {
                format!(
                    "{}...",
                    i.exclude_pattern.chars().take(47).collect::<String>()
                )
            } else {
                i.exclude_pattern.clone()
            };
            (
                "Find",
                format!("pattern: {}, path: {path}, exclude: {exclude}", i.pattern),
            )
        }
        Input::Grep(i) => (
            "Grep",
            if i.include_pattern.is_empty() {
                i.query.clone()
            } else {
                format!("{} in {}", i.query, i.include_pattern)
            },
        ),
        Input::Handoff(i) => ("Handoff", i.requested_agent.clone()),
        Input::ListFiles(i) => (
            if i.recursive { "List -R" } else { "List" },
            if i.path.is_empty() {
                ".".into()
            } else {
                i.path.clone()
            },
        ),
        Input::CodeInterpreter(_) => ("Interpreter", "Script".into()),
        _ => return None,
    };
    Some((name.into(), input))
}
pub fn format_tool_call(call: &ToolCall) -> String {
    tool_call_label(call)
        .map(|(name, input)| format!("◆ {name}({input})"))
        .unwrap_or_default()
}

pub fn format_tool_result(result: &ToolResult, width: usize) -> String {
    match result.result.as_ref() {
        Some(tool_result::Result::CodeInterpreter(output)) => format!(
            "◆ Interpreter(Output)\n{}",
            indent_lines(
                &format_markdown(&code_markdown(&output.output), width.saturating_sub(4)),
                "    "
            )
        ),
        Some(tool_result::Result::Fetch(output)) => format!("◆ Fetch({})", output.url),
        _ => String::new(),
    }
}
pub fn trim_markdown_edges(content: &str) -> String {
    static RE: std::sync::OnceLock<regex::Regex> = std::sync::OnceLock::new();
    RE.get_or_init(|| {
        regex::Regex::new(r"^(?:\x1b\[[0-9;]*m|\s)*|(?:\x1b\[[0-9;]*m|\s)*$").unwrap()
    })
    .replace_all(content, "")
    .into_owned()
}
pub fn indent_lines(content: &str, indentation: &str) -> String {
    content
        .split('\n')
        .map(|line| {
            if line.trim().is_empty() {
                line.into()
            } else {
                format!("{indentation}{line}")
            }
        })
        .collect::<Vec<_>>()
        .join("\n")
}
pub fn code_markdown(code: &str) -> String {
    format!("```\n{code}\n```")
}

pub fn format_markdown(content: &str, width: usize) -> String {
    use pulldown_cmark::{Event, Options, Parser, Tag, TagEnd};
    let mut out = String::new();
    let mut code = false;
    for event in Parser::new_ext(
        content,
        Options::ENABLE_TABLES | Options::ENABLE_STRIKETHROUGH,
    ) {
        match event {
            Event::Text(text) | Event::Html(text) | Event::InlineHtml(text) => out.push_str(&text),
            Event::Code(text) => out.push_str(&format!("\x1b[38;5;203m{text}\x1b[0m")),
            Event::Start(Tag::Strong | Tag::Heading { .. }) => out.push_str("\x1b[1m"),
            Event::Start(Tag::Emphasis) => out.push_str("\x1b[3m"),
            Event::Start(Tag::Strikethrough) => out.push_str("\x1b[9m"),
            Event::End(TagEnd::Strong | TagEnd::Emphasis | TagEnd::Strikethrough) => {
                out.push_str("\x1b[0m")
            }
            Event::End(TagEnd::Heading(_)) => out.push_str("\x1b[0m\n\n"),
            Event::Start(Tag::Item) => out.push_str("• "),
            Event::End(TagEnd::Item) => out.push('\n'),
            Event::Start(Tag::BlockQuote(_)) => out.push_str("│ "),
            Event::Start(Tag::CodeBlock(_)) => {
                code = true;
                out.push_str("\x1b[38;5;252m");
            }
            Event::End(TagEnd::CodeBlock) => {
                code = false;
                out.push_str("\x1b[0m\n");
            }
            Event::End(TagEnd::Paragraph) => out.push_str("\n\n"),
            Event::End(TagEnd::Link) => out.push_str("\x1b[0m"),
            Event::Start(Tag::Link { .. }) => out.push_str("\x1b[4;38;5;75m"),
            Event::SoftBreak if !code => out.push(' '),
            Event::SoftBreak | Event::HardBreak => out.push('\n'),
            Event::Rule => out.push_str(&format!("{}\n", "─".repeat(width.max(1)))),
            Event::TaskListMarker(done) => out.push_str(if done { "[x] " } else { "[ ] " }),
            _ => {}
        }
    }
    let wrapped = out
        .lines()
        .map(|line| textwrap::fill(line, width.max(1)))
        .collect::<Vec<_>>()
        .join("\n");
    trim_markdown_edges(&wrapped)
}
pub fn add_bottom_margin(index: usize, count: usize) -> bool {
    index == 0 || index + 1 != count
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Style {
    pub foreground: Option<u8>,
    pub background: Option<u8>,
    pub border_color: Option<u8>,
    pub bold: bool,
    pub italic: bool,
    pub padding: [u16; 4],
    pub margin: [u16; 4],
}
pub const SYMBOLS: &[(&str, &str, Option<u8>, bool)] = &[
    ("info", "ⓘ", Some(33), true),
    ("error", "❌", None, false),
    ("small_error", "✗", Some(196), true),
    ("warning", "⚠️", None, false),
    ("success", "✔", Some(10), true),
    ("attention", "❗", None, false),
    ("question", "❓", None, false),
    ("action", "🚀", None, false),
    ("continue", "➡️", None, false),
    ("link", "→", Some(75), false),
    ("docs", "📚", None, false),
    ("community", "💬", None, false),
    ("bug", "⚑", Some(208), false),
];
pub fn named_style(name: &str) -> Style {
    let mut s = Style::default();
    match name {
        "app" => s.margin = [1, 2, 1, 2],
        "header" => {
            s.border_color = Some(62);
            s.padding = [0, 1, 0, 1];
            s.margin[2] = 1;
        }
        "input" => s.margin[0] = 1,
        "agent_name" => {
            s.foreground = Some(255);
            s.bold = true;
        }
        "agent_diamond" => {
            s.foreground = Some(250);
            s.bold = true;
        }
        "agent_model" => {
            s.foreground = Some(245);
            s.italic = true;
        }
        "separator" | "bullet_separator" => s.foreground = Some(240),
        "task_status" => s.foreground = Some(34),
        "usage" => s.foreground = Some(241),
        "error" => {
            s.foreground = Some(9);
            s.bold = true;
        }
        "tool_call" => {
            s.foreground = Some(250);
            s.padding[3] = 1;
        }
        "user_message" => {
            s.foreground = Some(255);
            s.border_color = Some(245);
            s.margin[2] = 1;
            s.padding = [0, 1, 0, 1];
        }
        "assistant_message" => {
            s.foreground = Some(254);
            s.padding[3] = 1;
        }
        "bold" => s.bold = true,
        "code_interpreter" => {
            s.foreground = Some(252);
            s.padding = [0, 1, 0, 1];
            s.border_color = Some(34);
        }
        "help_overlay" => {
            s.border_color = Some(39);
            s.background = Some(235);
            s.padding = [1, 2, 1, 2];
            s.margin = [1; 4];
        }
        "help_title" => {
            s.foreground = Some(39);
            s.bold = true;
        }
        "help_item" => s.foreground = Some(252),
        _ => {}
    }
    s
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn completion() {
        let mut message = Message::default();
        assert!(!is_complete_response(&message));
        message.metadata = Some(MessageMetadata {
            role: MessageRole::Assistant as i32,
            ..Default::default()
        });
        message.spec = Some(MessageSpec::default());
        assert!(is_complete_response(&message));
        message.spec.as_mut().unwrap().content.push(MessagePart {
            data: Some(message_part::Data::ToolCall(ToolCall::default())),
        });
        assert!(!is_complete_response(&message));
    }
    #[test]
    fn formatting() {
        assert_eq!(
            crate::render::strip_ansi(&format_markdown("**Hello** *world*", 80)),
            "Hello world"
        );
        assert_eq!(format_markdown("", 0), "");
        assert_eq!(format_markdown("hello world", 5), "hello\nworld");
        assert_eq!(trim_markdown_edges("\x1b[31m \nHello\n \x1b[0m"), "Hello");
        assert_eq!(trim_markdown_edges(""), "");
        assert_eq!(indent_lines("a\n\nb", "  "), "  a\n\n  b");
        assert_eq!(format_tool_call(&ToolCall::default()), "");
        let call = ToolCall {
            input: Some(tool_call::Input::ReadFile(tool_call::ReadFileInput {
                path: "a.rs".into(),
                start_line: 1,
                end_line: 4,
            })),
            ..Default::default()
        };
        assert_eq!(format_tool_call(&call), "◆ Read(a.rs L1-4)");
        assert_eq!(
            DisplayAnswer::new(&Task::default(), &Message::default()).result,
            ""
        );
    }
}
