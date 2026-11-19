use super::{
    session::{Action, Input, Picker, Session, HELP},
    FeedContent, UiMode,
};
use crate::{app::ApiClient, Result};
use ansi_to_tui::IntoText;
use construct_connect::{Request, Streaming};
use construct_proto::construct::v1::*;
use crossterm::{
    event::{
        DisableBracketedPaste, EnableBracketedPaste, Event as TerminalEvent, EventStream,
        KeyEventKind, MouseEventKind,
    },
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use futures_util::{future::BoxFuture, stream::FuturesUnordered, FutureExt, StreamExt};
use ratatui::{
    backend::CrosstermBackend,
    layout::{Constraint, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Text},
    widgets::{Block, Borders, Clear, Paragraph, Row, Table, TableState, Wrap},
    Frame, Terminal,
};
use std::{
    cell::RefCell,
    io::{self, Stdout, Write},
    marker::PhantomData,
    rc::Rc,
    sync::{Arc, Mutex, Once},
    time::{Duration, Instant},
};
use tokio_util::sync::CancellationToken;

type Restore = Arc<dyn Fn() + Send + Sync>;
thread_local! {
    static ACTIVE_RESTORE: RefCell<Option<Restore>> = const { RefCell::new(None) };
}
struct RestoreTerminal {
    restore: Restore,
    _thread_bound: PhantomData<Rc<()>>,
}
impl RestoreTerminal {
    fn enter() -> Result<Self> {
        Self::enter_with(
            Arc::new(Mutex::new(io::stdout())),
            enable_raw_mode,
            disable_raw_mode,
        )
    }
    fn enter_with<W: Write + Send + 'static>(
        writer: Arc<Mutex<W>>,
        enable: impl FnOnce() -> io::Result<()>,
        disable: impl Fn() -> io::Result<()> + Send + Sync + 'static,
    ) -> Result<Self> {
        static HOOK: Once = Once::new();
        HOOK.call_once(|| {
            let previous = std::panic::take_hook();
            std::panic::set_hook(Box::new(move |info| {
                ACTIVE_RESTORE.with(|active| {
                    if let Some(restore) = active.borrow().as_ref() {
                        restore();
                    }
                });
                previous(info);
            }));
        });
        enable()?;
        let output = writer.clone();
        let restore: Restore = Arc::new(move || {
            let _ = disable();
            let mut writer = output.lock().unwrap_or_else(|error| error.into_inner());
            let _ = execute!(
                *writer,
                DisableBracketedPaste,
                LeaveAlternateScreen,
                crossterm::cursor::Show
            );
        });
        let guard = Self {
            restore: restore.clone(),
            _thread_bound: PhantomData,
        };
        ACTIVE_RESTORE.with(|active| *active.borrow_mut() = Some(restore));
        {
            let mut writer = writer
                .lock()
                .map_err(|_| io::Error::other("terminal writer poisoned"))?;
            execute!(*writer, EnterAlternateScreen, EnableBracketedPaste)?;
        }
        Ok(guard)
    }
}
impl Drop for RestoreTerminal {
    fn drop(&mut self) {
        ACTIVE_RESTORE.with(|active| active.borrow_mut().take());
        (self.restore)();
    }
}
fn terminal() -> Result<Terminal<CrosstermBackend<Stdout>>> {
    Ok(Terminal::new(CrosstermBackend::new(io::stdout()))?)
}

pub fn status(state: &Session) -> String {
    let agent = state
        .agent
        .spec
        .as_ref()
        .map(|s| s.name.as_str())
        .unwrap_or("Unknown");
    let model = state
        .models
        .get(&state.model_id())
        .and_then(|m| m.spec.as_ref());
    let model_name = model
        .map(|m| {
            if m.name.chars().count() > 12 {
                format!("{}...", m.name.chars().take(12).collect::<String>())
            } else {
                m.name.clone()
            }
        })
        .unwrap_or_default();
    let phase = if state.spinning() {
        format!(
            "{} Thinking",
            ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"][state.frame % 10]
        )
    } else if state.phase() == TaskPhase::Suspended {
        "Suspended".into()
    } else {
        String::new()
    };
    let u = &state.usage;
    let mut usage = format!("Tokens: {}↑ {}↓", u.input_tokens, u.output_tokens);
    if u.cache_read_tokens > 0 || u.cache_write_tokens > 0 {
        usage += &format!(
            " (Cache: {}↑ {}↓)",
            u.cache_read_tokens, u.cache_write_tokens
        );
    }
    if let Some(model) = model.filter(|m| m.context_window > 0) {
        let tokens = i128::from(u.input_tokens)
            + i128::from(u.output_tokens)
            + i128::from(u.cache_read_tokens)
            + i128::from(u.cache_write_tokens);
        usage += &format!(
            " | Context: {}%",
            (tokens * 100 / i128::from(model.context_window)).min(100)
        );
    }
    format!(
        "» {agent}{} • {phase}    {usage} | Cost: ${:.2}",
        if model_name.is_empty() {
            String::new()
        } else {
            format!(" • {model_name}")
        },
        state.cost
    )
}
fn markdown(content: &str, width: u16) -> Text<'static> {
    super::format_markdown(content, usize::from(width.max(1)))
        .into_text()
        .unwrap_or_else(|_| Text::raw(content.to_owned()))
}
pub fn feed(state: &Session, width: u16) -> Text<'static> {
    let mut lines = Vec::new();
    if state.feed.is_empty() && state.partial.is_empty() {
        return Text::raw("────────────────────────────────────────────────────\nWelcome! Type your message below.\nPress Ctrl + ? for help at any time.\nPress Ctrl + C to clear the input area.\nPress Ctrl + C twice to exit.\nPress Esc to stop the agent execution.\n────────────────────────────────────────────────────");
    }
    for entry in &state.feed {
        let before = lines.len();
        match entry {
            FeedContent::User(text) => {
                lines.push(Line::styled("╭─ You", Style::default().fg(Color::DarkGray)));
                for mut line in markdown(text, width.saturating_sub(3)).lines {
                    line.spans.insert(0, "│ ".into());
                    lines.push(line);
                }
                lines.push(Line::styled("╰─", Style::default().fg(Color::DarkGray)));
            }
            FeedContent::Assistant(text) => {
                lines.extend(markdown(text, width.saturating_sub(1)).lines)
            }
            FeedContent::ToolCall(call) => {
                let label = super::format_tool_call(call);
                let label = if label.is_empty() {
                    format!("◆ {}", call.tool_name)
                } else {
                    label
                };
                lines.push(Line::styled(label, Style::default().fg(Color::Gray)));
            }
            FeedContent::ToolResult(result) => {
                let formatted = super::format_tool_result(result, usize::from(width));
                if !formatted.is_empty() {
                    lines.extend(formatted.into_text().unwrap_or_default().lines);
                }
                if let Some(error) = &result.error {
                    lines.push(Line::styled(
                        format!("❌ Error: {}", error.message),
                        Style::default().fg(Color::Red),
                    ));
                }
            }
            FeedContent::Error(error) => {
                for line in format!("❌ Error: {error}").lines() {
                    lines.push(Line::styled(
                        line.to_owned(),
                        Style::default().fg(Color::Red),
                    ));
                }
            }
            FeedContent::SubmitReport(report) => {
                lines.extend(markdown(&report.summary, width).lines)
            }
            FeedContent::Typing => lines.push(Line::raw("Thinking…")),
        }
        // Results Go does not render (e.g. create_file) add no spacing either.
        if lines.len() > before {
            lines.push(Line::raw(""));
        }
    }
    if !state.partial.is_empty() {
        lines.extend(markdown(&state.partial, width).lines);
    }
    Text::from(lines)
}
fn input_text(input: &str, cursor: usize, width: u16) -> (Text<'static>, usize, usize) {
    let width = usize::from(width.max(1));
    let mut lines = vec![String::new()];
    let mut column = 0;
    let mut position = (0, 0);
    for (index, c) in input
        .char_indices()
        .chain(std::iter::once((input.len(), '\0')))
    {
        let size = Line::raw(c.to_string()).width();
        if c != '\n' && column + size > width || column == width {
            lines.push(String::new());
            column = 0;
        }
        if index == cursor {
            position = (lines.len() - 1, column);
        }
        if index == input.len() {
            break;
        }
        if c == '\n' {
            lines.push(String::new());
            column = 0;
        } else {
            lines.last_mut().unwrap().push(c);
            column += size;
        }
    }
    (
        Text::from(lines.into_iter().map(Line::raw).collect::<Vec<_>>()),
        position.0,
        position.1,
    )
}
pub fn render(frame: &mut Frame<'_>, state: &mut Session) {
    let area = frame.area();
    let inner = Rect {
        x: area.x.saturating_add(2),
        y: area.y.saturating_add(1),
        width: area.width.saturating_sub(4),
        height: area.height.saturating_sub(2),
    };
    let layout = Layout::vertical([
        Constraint::Length(3),
        Constraint::Min(1),
        Constraint::Length(6),
    ])
    .split(inner);
    frame.render_widget(
        Paragraph::new(status(state))
            .style(Style::default().fg(Color::White))
            .wrap(Wrap { trim: false }),
        layout[0],
    );
    state.page_height = usize::from(layout[1].height);
    let text = feed(state, layout[1].width);
    let paragraph = Paragraph::new(text).wrap(Wrap { trim: false });
    let total = paragraph.line_count(layout[1].width);
    let bottom = total.saturating_sub(state.page_height);
    state.scroll = state.scroll.min(bottom);
    frame.render_widget(
        paragraph.scroll((
            bottom.saturating_sub(state.scroll).min(u16::MAX as usize) as u16,
            0,
        )),
        layout[1],
    );
    let title = if state.mode == UiMode::Input {
        " Message · Enter send · Alt+Enter newline · F2 scroll "
    } else {
        " Scroll · F1 input "
    };
    let (mut input, row, col) = input_text(
        &state.input,
        state.cursor,
        layout[2].width.saturating_sub(2),
    );
    if state.input.is_empty() {
        input = Text::raw("Type your message...");
    }
    let input_height = usize::from(layout[2].height.saturating_sub(2));
    let input_scroll = row.saturating_sub(input_height.saturating_sub(1));
    frame.render_widget(
        Paragraph::new(input)
            .block(Block::default().borders(Borders::ALL).title(title))
            .scroll((input_scroll.min(u16::MAX as usize) as u16, 0)),
        layout[2],
    );
    if state.mode == UiMode::Input
        && !state.show_help
        && layout[2].width > 2
        && layout[2].height > 2
    {
        frame.set_cursor_position((
            layout[2].x + 1 + (col as u16).min(layout[2].width.saturating_sub(3)),
            layout[2].y + 1 + (row - input_scroll) as u16,
        ));
    }
    if state.show_help {
        frame.render_widget(Clear, area);
        frame.render_widget(
            Paragraph::new(HELP)
                .block(
                    Block::default()
                        .borders(Borders::ALL)
                        .border_style(Style::default().fg(Color::Cyan)),
                )
                .wrap(Wrap { trim: false }),
            area,
        );
    }
}

enum Reply {
    Inputs(Vec<Input>),
    Stream(Streaming<EventSubscribeResponse>),
}
async fn subscribe(client: &ApiClient, id: String) -> Result<Reply> {
    Ok(Reply::Stream(
        client
            .event
            .subscribe(Request::new(EventSubscribeRequest {
                event_types: vec!["task.*".into(), "message.*".into(), "tool.*".into()],
                task_id: Some(id),
                ..Default::default()
            }))
            .await?
            .message,
    ))
}
async fn perform(client: &ApiClient, id: String, action: Action) -> Result<Reply> {
    let input = match action {
        Action::Send(content) => {
            let message = super::send_message(client, &id, &content).await?;
            Some(Input::Wire(Event {
                payload: Some(event::Payload::Message(MessageEvent {
                    message: Some(message),
                })),
                ..Default::default()
            }))
        }
        Action::Suspend => {
            client
                .task
                .suspend_task(Request::new(SuspendTaskRequest {
                    task_id: id.clone(),
                }))
                .await?;
            Some(Input::Task(super::get_task(client, &id).await?))
        }
        Action::FetchTask => Some(Input::Task(super::get_task(client, &id).await?)),
        Action::ListAgents => Some(Input::Agents(
            client
                .agent
                .list_agents(Request::new(ListAgentsRequest::default()))
                .await?
                .message
                .agents,
        )),
        Action::FetchModel(model_id) => Some(Input::Model(
            model_id.clone(),
            client
                .model
                .get_model(Request::new(GetModelRequest { id: model_id }))
                .await?
                .message
                .model
                .ok_or("missing model")?,
        )),
        Action::SwitchAgent(agent_id) => {
            client
                .task
                .update_task(Request::new(UpdateTaskRequest {
                    id,
                    agent_id: Some(agent_id),
                }))
                .await?;
            None
        }
        Action::Reconnect => return subscribe(client, id).await,
        Action::Quit => None,
    };
    Ok(Reply::Inputs(input.into_iter().collect()))
}
pub async fn run(
    client: &ApiClient,
    task: Task,
    agent: Agent,
    cancellation: CancellationToken,
) -> Result<()> {
    let _guard = RestoreTerminal::enter()?;
    let mut terminal = terminal()?;
    let mut state = Session::new(task, agent);
    let mut pending: FuturesUnordered<BoxFuture<'_, Result<Reply>>> = FuturesUnordered::new();
    for action in [
        Action::Reconnect,
        Action::ListAgents,
        Action::FetchModel(state.model_id()),
    ] {
        pending.push(perform(client, state.task_id(), action).boxed());
    }
    let mut stream: Option<Streaming<EventSubscribeResponse>> = None;
    let mut keys = EventStream::new();
    let mut tick = tokio::time::interval(Duration::from_millis(166));
    let start = Instant::now();
    loop {
        terminal.draw(|frame| render(frame, &mut state))?;
        let input = tokio::select! {
            _ = cancellation.cancelled() => break,
            _ = tick.tick() => Some(Input::Tick),
            key = keys.next() => match key {
                Some(Ok(TerminalEvent::Key(key))) if key.kind != KeyEventKind::Release => Some(Input::Key(key, start.elapsed().as_millis() as u64)),
                Some(Ok(TerminalEvent::Paste(text))) => { for c in text.chars() { state.update(Input::Key(crossterm::event::KeyEvent::new(crossterm::event::KeyCode::Char(c), crossterm::event::KeyModifiers::NONE), 0)); } None }
                Some(Ok(TerminalEvent::Mouse(mouse))) => { match mouse.kind { MouseEventKind::ScrollUp => state.scroll = state.scroll.saturating_add(3), MouseEventKind::ScrollDown => state.scroll = state.scroll.saturating_sub(3), _ => {} } None }
                Some(Err(error)) => return Err(error.into()),
                None => break,
                _ => None,
            },
            reply = pending.next(), if !pending.is_empty() => {
                match reply {
                    Some(Ok(Reply::Stream(next))) => { stream = Some(next); None }
                    Some(Ok(Reply::Inputs(inputs))) => { for input in inputs { state.update(input); } None }
                    Some(Err(error)) => Some(Input::Error(api_error(error))),
                    None => None,
                }
            },
            event = async { match &mut stream { Some(stream) => stream.next().await, None => std::future::pending().await } } => {
                match event {
                    Some(Ok(event)) => event.event.map(Input::Wire),
                    Some(Err(error)) => { stream = None; Some(Input::Error(api_error(error.into()))) }
                    None => { stream = None; Some(Input::Error("Event stream closed. Press Ctrl+R to reconnect.".into())) }
                }
            },
        };
        if let Some(input) = input {
            for action in state.update(input) {
                if action == Action::Quit {
                    return Ok(());
                }
                if let Action::FetchModel(id) = &action {
                    if state.models.contains_key(id) {
                        continue;
                    }
                }
                if action == Action::Reconnect {
                    stream = None;
                }
                pending.push(perform(client, state.task_id(), action).boxed());
            }
        }
    }
    Ok(())
}
fn api_error(error: crate::Error) -> String {
    crate::errors::transform_error(error)
        .map(|e| {
            e.downcast_ref::<crate::errors::UserFacingError>()
                .map(|e| e.user_message.clone())
                .unwrap_or_else(|| e.to_string())
        })
        .unwrap_or_default()
}

pub fn render_picker(frame: &mut Frame<'_>, tasks: &[Task], selected: usize) {
    let rows = tasks.iter().map(|task| {
        let metadata = task.metadata.as_ref();
        let spec = task.spec.as_ref();
        let date = |time: Option<(i64, i32)>| {
            time.and_then(|(seconds, nanos)| {
                chrono::DateTime::from_timestamp(seconds, nanos as u32)
            })
            .map(|t| t.format("%Y-%m-%d %H:%M").to_string())
            .unwrap_or_default()
        };
        Row::new(vec![
            metadata.map(|m| m.id.clone()).unwrap_or_default(),
            date(
                metadata
                    .and_then(|m| m.created_at.as_ref())
                    .map(|t| (t.seconds, t.nanos)),
            ),
            date(
                metadata
                    .and_then(|m| m.updated_at.as_ref())
                    .map(|t| (t.seconds, t.nanos)),
            ),
            spec.map(|s| {
                if s.workspace.is_empty() {
                    "unspecified".into()
                } else {
                    s.workspace.clone()
                }
            })
            .unwrap_or_else(|| "unspecified".into()),
            task.status
                .as_ref()
                .map_or(0, |s| s.message_count)
                .to_string(),
            spec.map(|s| s.description.clone()).unwrap_or_default(),
        ])
    });
    let table = Table::new(
        rows,
        [
            Constraint::Length(36),
            Constraint::Length(16),
            Constraint::Length(16),
            Constraint::Length(25),
            Constraint::Length(8),
            Constraint::Min(15),
        ],
    )
    .header(
        Row::new([
            "ID",
            "Created",
            "Updated",
            "Workspace",
            "Messages",
            "Summary",
        ])
        .style(Style::default().fg(Color::Cyan)),
    )
    .block(
        Block::default()
            .borders(Borders::ALL)
            .title("Select a task to resume")
            .title_bottom("Press ↑/↓ or j/k to navigate, Enter to select, Esc to cancel"),
    )
    .row_highlight_style(
        Style::default()
            .fg(Color::Cyan)
            .bg(Color::DarkGray)
            .add_modifier(Modifier::BOLD),
    );
    frame.render_stateful_widget(
        table,
        frame.area(),
        &mut TableState::default().with_selected(Some(selected)),
    );
}
pub async fn pick_task(tasks: Vec<Task>, cancellation: CancellationToken) -> Result<Option<Task>> {
    let _guard = RestoreTerminal::enter()?;
    let mut terminal = terminal()?;
    let mut picker = Picker::default();
    let mut events = EventStream::new();
    loop {
        terminal.draw(|frame| render_picker(frame, &tasks, picker.selected))?;
        tokio::select! {
            _ = cancellation.cancelled() => return Ok(None),
            event = events.next() => match event {
                Some(Ok(TerminalEvent::Key(key))) if key.kind != KeyEventKind::Release => {
                    if let Some(selected) = picker.key(key, tasks.len()) { return Ok(if selected { tasks.get(picker.selected).cloned() } else { None }); }
                }
                Some(Err(error)) => return Err(error.into()),
                None => return Ok(None),
                _ => {}
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::terminal::session::tests::{state, task};
    use ratatui::{backend::TestBackend, buffer::Buffer};
    use std::sync::atomic::{AtomicBool, Ordering};

    const RESTORE_SEQUENCE: &[u8] = b"\x1b[?2004l\x1b[?1049l\x1b[?25h";

    #[derive(Default, Clone)]
    struct TerminalState {
        raw: Arc<AtomicBool>,
        output: Arc<Mutex<Vec<u8>>>,
    }
    impl TerminalState {
        fn enter(&self) -> RestoreTerminal {
            let enable = self.raw.clone();
            let disable = self.raw.clone();
            RestoreTerminal::enter_with(
                self.output.clone(),
                move || {
                    enable.store(true, Ordering::SeqCst);
                    Ok(())
                },
                move || {
                    disable.store(false, Ordering::SeqCst);
                    Ok(())
                },
            )
            .unwrap()
        }
        fn restored(&self) -> bool {
            !self.raw.load(Ordering::SeqCst)
                && self.output.lock().unwrap().ends_with(RESTORE_SEQUENCE)
        }
    }
    #[test]
    fn panic_hook_restores_before_unwinding_and_guard_restores_on_drop() {
        struct ObserveUnwind {
            terminal: TerminalState,
            restored: Arc<AtomicBool>,
        }
        impl Drop for ObserveUnwind {
            fn drop(&mut self) {
                self.restored
                    .store(self.terminal.restored(), Ordering::SeqCst);
            }
        }
        let terminal = TerminalState::default();
        let hook_restored = Arc::new(AtomicBool::new(false));
        let result = std::panic::catch_unwind(|| {
            let _guard = terminal.enter();
            assert!(terminal.raw.load(Ordering::SeqCst));
            assert_eq!(*terminal.output.lock().unwrap(), b"\x1b[?1049h\x1b[?2004h");
            {
                let mut output = terminal.output.lock().unwrap();
                execute!(*output, crossterm::cursor::Hide).unwrap();
            }
            let _observe = ObserveUnwind {
                terminal: terminal.clone(),
                restored: hook_restored.clone(),
            };
            panic!("forced panic with an active terminal");
        });
        assert!(result.is_err());
        assert!(
            hook_restored.load(Ordering::SeqCst),
            "hook must restore before guard unwinds"
        );
        assert!(terminal.restored());
        let output = terminal.output.lock().unwrap();
        let restore_count = output
            .windows(RESTORE_SEQUENCE.len())
            .filter(|bytes| *bytes == RESTORE_SEQUENCE)
            .count();
        assert_eq!(restore_count, 2, "panic hook and guard drop both restore");
        ACTIVE_RESTORE.with(|active| assert!(active.borrow().is_none()));
    }
    #[test]
    fn terminal_guard_restores_on_normal_exit() {
        let terminal = TerminalState::default();
        let guard = terminal.enter();
        assert!(terminal.raw.load(Ordering::SeqCst));
        drop(guard);
        assert!(terminal.restored());
    }
    #[test]
    fn raw_mode_entry_failure_does_not_enter_alternate_screen() {
        let output = Arc::new(Mutex::new(Vec::<u8>::new()));
        let disabled = Arc::new(AtomicBool::new(false));
        let observed = disabled.clone();
        assert!(RestoreTerminal::enter_with(
            output.clone(),
            || Err(io::Error::other("raw mode unavailable")),
            move || {
                observed.store(true, Ordering::SeqCst);
                Ok(())
            }
        )
        .is_err());
        assert!(output.lock().unwrap().is_empty());
        assert!(!disabled.load(Ordering::SeqCst));
        ACTIVE_RESTORE.with(|active| assert!(active.borrow().is_none()));
    }

    fn rows(buffer: &Buffer) -> Vec<String> {
        (0..buffer.area.height)
            .map(|y| {
                let mut row = String::new();
                let mut x = 0;
                while x < buffer.area.width {
                    let symbol = buffer[(x, y)].symbol();
                    row.push_str(symbol);
                    x += Line::raw(symbol).width().max(1) as u16;
                }
                row.trim_end().to_owned()
            })
            .collect()
    }
    #[test]
    fn feed_snapshot_with_markdown_tool_parts_and_errors() {
        let mut s = state();
        let mut error = ToolResult {
            error: Some(Default::default()),
            ..Default::default()
        };
        error.error.as_mut().unwrap().message = "denied".into();
        s.feed = vec![
            FeedContent::User("prompt".into()),
            FeedContent::Assistant("Say **Ready** now".into()),
            FeedContent::ToolCall(ToolCall {
                input: Some(tool_call::Input::ReadFile(tool_call::ReadFileInput {
                    path: "a.rs".into(),
                    ..Default::default()
                })),
                ..Default::default()
            }),
            FeedContent::ToolCall(ToolCall {
                tool_name: "unknown".into(),
                ..Default::default()
            }),
            FeedContent::ToolResult(ToolResult::default()),
            FeedContent::ToolResult(error),
            FeedContent::Error("connection lost".into()),
        ];
        let mut terminal = Terminal::new(TestBackend::new(60, 22)).unwrap();
        terminal
            .draw(|f| f.render_widget(Paragraph::new(feed(&s, 60)), f.area()))
            .unwrap();
        let visible = rows(terminal.backend().buffer())
            .into_iter()
            .filter(|line| !line.is_empty())
            .collect::<Vec<_>>();
        assert_eq!(
            visible,
            [
                "╭─ You",
                "│ prompt",
                "╰─",
                "Say Ready now",
                "◆ Read(a.rs)",
                "◆ unknown",
                "❌ Error: denied",
                "❌ Error: connection lost"
            ]
        );
        assert!(terminal.backend().buffer()[(4, 4)]
            .modifier
            .contains(Modifier::BOLD));
    }
    #[test]
    fn status_snapshot_and_spinner_stops() {
        let mut s = state();
        s.models.insert(
            "model".into(),
            Model {
                spec: Some(ModelSpec {
                    name: "model-name-long".into(),
                    context_window: 100,
                    ..Default::default()
                }),
                ..Default::default()
            },
        );
        s.usage = MessageUsage {
            input_tokens: 10,
            output_tokens: 5,
            cache_read_tokens: 2,
            cache_write_tokens: 3,
            ..Default::default()
        };
        s.cost = 1.25;
        s.update(Input::Task(task(TaskPhase::Running)));
        s.cost = 1.25;
        let mut terminal = Terminal::new(TestBackend::new(150, 26)).unwrap();
        terminal.draw(|frame| render(frame, &mut s)).unwrap();
        assert_eq!(rows(terminal.backend().buffer())[1].trim(), "» Coder • model-name-l... • ⠋ Thinking    Tokens: 10↑ 5↓ (Cache: 2↑ 3↓) | Context: 20% | Cost: $1.25");
        s.update(Input::Task(task(TaskPhase::Awaiting)));
        terminal.draw(|frame| render(frame, &mut s)).unwrap();
        assert!(!rows(terminal.backend().buffer())
            .join("\n")
            .contains("Thinking"));
        s.update(Input::Task(task(TaskPhase::Suspended)));
        assert!(status(&s).contains("Suspended"));
    }
    #[test]
    fn welcome_and_help_snapshots() {
        let mut s = state();
        let mut terminal = Terminal::new(TestBackend::new(80, 38)).unwrap();
        terminal.draw(|frame| render(frame, &mut s)).unwrap();
        let screen = rows(terminal.backend().buffer()).join("\n");
        assert!(screen.contains("Welcome! Type your message below."));
        assert!(screen.contains("Press Ctrl + C twice to exit."));
        assert!(screen.contains("Type your message..."));
        s.show_help = true;
        terminal.draw(|frame| render(frame, &mut s)).unwrap();
        let lines = rows(terminal.backend().buffer())
            .into_iter()
            .skip(1)
            .take(36)
            .map(|line| line.trim_matches('│').trim().to_owned())
            .filter(|line| !line.is_empty())
            .collect::<Vec<_>>();
        assert_eq!(
            lines,
            [
                "KEYBOARD SHORTCUTS",
                "General:",
                "Ctrl+?        - Toggle this help",
                "Ctrl+C        - Clear input; twice within 1s to quit",
                "Esc           - Suspend task execution",
                "Ctrl+L        - Clear conversation",
                "Ctrl+R        - Reconnect to task",
                "Tab           - Switch agent",
                "Input Mode (F1):",
                "Enter         - Send message",
                "Alt+Enter     - New line (Ctrl+Enter also supported)",
                "F2            - Switch to scroll mode",
                "Scroll Mode (F2):",
                "↑↓, k/j       - Line up/down",
                "PgUp/PgDn     - Page up/down",
                "Ctrl+U/D      - Half page up/down",
                "Home/End      - Go to top/bottom",
                "F1            - Switch to input mode",
                "Press H or Esc to close this help."
            ]
        );
    }
    #[test]
    fn scrolling_tiny_terminals_and_unicode_input() {
        let mut s = state();
        s.feed = (0..40)
            .map(|i| FeedContent::Assistant(format!("message {i}")))
            .collect();
        let mut terminal = Terminal::new(TestBackend::new(60, 20)).unwrap();
        terminal.draw(|frame| render(frame, &mut s)).unwrap();
        let bottom = rows(terminal.backend().buffer()).join("\n");
        assert!(bottom.contains("message 39"));
        assert!(!bottom.contains("message 0\n"));
        s.scroll = usize::MAX;
        terminal.draw(|frame| render(frame, &mut s)).unwrap();
        let top = rows(terminal.backend().buffer()).join("\n");
        assert!(top.contains("message 0"));
        assert!(!top.contains("message 39"));
        let (_, row, col) = input_text("é界x", "é界x".len(), 3);
        assert_eq!((row, col), (1, 1));
        for (width, height) in [(1, 1), (2, 2), (10, 5)] {
            let mut tiny = Terminal::new(TestBackend::new(width, height)).unwrap();
            tiny.draw(|frame| render(frame, &mut s)).unwrap();
        }
    }
    #[test]
    fn picker_snapshot() {
        let tasks = vec![task(TaskPhase::Awaiting)];
        let mut terminal = Terminal::new(TestBackend::new(150, 8)).unwrap();
        terminal
            .draw(|frame| render_picker(frame, &tasks, 0))
            .unwrap();
        let screen = rows(terminal.backend().buffer()).join("\n");
        assert!(screen.contains("Select a task to resume"));
        assert!(screen.contains("Workspace"));
        assert!(screen.contains("unspecified"));
        assert!(screen.contains("Esc to cancel"));
    }
}
