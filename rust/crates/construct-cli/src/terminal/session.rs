use super::{FeedContent, UiMode};
use construct_proto::construct::v1::*;
use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use std::collections::{HashMap, HashSet};

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Action {
    Send(String),
    Suspend,
    FetchTask,
    ListAgents,
    FetchModel(String),
    SwitchAgent(String),
    Reconnect,
    Quit,
}
#[derive(Clone, Debug)]
pub enum Input {
    Key(KeyEvent, u64),
    Wire(Event),
    Task(Task),
    Agents(Vec<Agent>),
    Model(String, Model),
    Error(String),
    Tick,
}
pub struct Session {
    pub task: Task,
    pub agent: Agent,
    pub agents: Vec<Agent>,
    pub models: HashMap<String, Model>,
    pub feed: Vec<FeedContent>,
    pub partial: String,
    pub input: String,
    pub cursor: usize,
    pub show_help: bool,
    pub mode: UiMode,
    pub waiting: bool,
    pub frame: usize,
    pub scroll: usize,
    pub page_height: usize,
    pub usage: MessageUsage,
    pub cost: f64,
    seen: HashSet<String>,
    tools: HashSet<String>,
    results: HashSet<String>,
    last_ctrl_c: Option<u64>,
    running: bool,
    received_answer: bool,
}
impl Session {
    pub fn new(task: Task, agent: Agent) -> Self {
        Self {
            cost: task
                .status
                .as_ref()
                .and_then(|s| s.usage.as_ref())
                .map_or(0., |u| u.cost),
            task,
            agent,
            agents: vec![],
            models: HashMap::new(),
            feed: vec![],
            partial: String::new(),
            input: String::new(),
            cursor: 0,
            show_help: false,
            mode: UiMode::Input,
            waiting: false,
            frame: 0,
            scroll: 0,
            page_height: 10,
            usage: MessageUsage::default(),
            seen: HashSet::new(),
            tools: HashSet::new(),
            results: HashSet::new(),
            last_ctrl_c: None,
            running: false,
            received_answer: false,
        }
    }
    pub fn task_id(&self) -> String {
        self.task
            .metadata
            .as_ref()
            .map(|m| m.id.clone())
            .unwrap_or_default()
    }
    pub fn model_id(&self) -> String {
        self.agent
            .spec
            .as_ref()
            .map(|s| s.model_id.clone())
            .unwrap_or_default()
    }
    pub fn phase(&self) -> TaskPhase {
        self.task
            .status
            .as_ref()
            .map(|s| s.phase())
            .unwrap_or_default()
    }
    pub fn spinning(&self) -> bool {
        self.waiting || self.phase() == TaskPhase::Running
    }
    pub fn error(&mut self, error: String) {
        self.waiting = false;
        if let Some(FeedContent::Error(last)) = self.feed.last_mut() {
            *last = error;
        } else {
            self.feed.push(FeedContent::Error(error));
        }
    }
    fn tool(&mut self, call: ToolCall) {
        // Go only shows interpreter scripts in verbose mode, which the TUI never enables.
        if matches!(call.input, Some(tool_call::Input::CodeInterpreter(_))) {
            return;
        }
        if call.id.is_empty() || self.tools.insert(call.id.clone()) {
            self.feed.push(FeedContent::ToolCall(call));
        }
    }
    fn result(&mut self, result: ToolResult) {
        if matches!(result.result, Some(tool_result::Result::CodeInterpreter(_))) {
            return;
        }
        if result.id.is_empty() || self.results.insert(result.id.clone()) {
            self.feed.push(FeedContent::ToolResult(result));
        }
    }
    fn message(&mut self, message: Message) {
        if let Some(metadata) = &message.metadata {
            if !metadata.id.is_empty() && !self.seen.insert(metadata.id.clone()) {
                return;
            }
        }
        let assistant = message
            .metadata
            .as_ref()
            .is_some_and(|m| m.role == MessageRole::Assistant as i32);
        if assistant {
            self.received_answer = true;
            if let Some(usage) = message.status.as_ref().and_then(|s| s.usage) {
                self.usage = usage;
            }
            if super::is_complete_response(&message) {
                self.waiting = false;
            }
        } else {
            self.scroll = 0;
        }
        for part in message.spec.into_iter().flat_map(|s| s.content) {
            match part.data {
                Some(message_part::Data::Text(text)) => {
                    self.partial.clear();
                    self.feed.push(if assistant {
                        FeedContent::Assistant(text.content)
                    } else {
                        FeedContent::User(text.content)
                    });
                }
                Some(message_part::Data::ToolCall(call)) => self.tool(call),
                Some(message_part::Data::ToolResult(result)) => self.result(result),
                Some(message_part::Data::Error(error)) => self.error(error.message),
                None => {}
            }
        }
    }
    fn insert(&mut self, text: &str) {
        if self.input.chars().count() + text.chars().count() <= 32768 {
            self.input.insert_str(self.cursor, text);
            self.cursor += text.len();
        }
    }
    fn line_start(&self) -> usize {
        self.input[..self.cursor].rfind('\n').map_or(0, |i| i + 1)
    }
    fn line_end(&self) -> usize {
        self.input[self.cursor..]
            .find('\n')
            .map_or(self.input.len(), |i| self.cursor + i)
    }
    fn vertical(&mut self, down: bool) {
        let start = self.line_start();
        let column = self.input[start..self.cursor].chars().count();
        let target = if down {
            let end = self.line_end();
            if end == self.input.len() {
                return;
            }
            end + 1
        } else {
            if start == 0 {
                return;
            }
            self.input[..start - 1].rfind('\n').map_or(0, |i| i + 1)
        };
        self.cursor = target
            + self.input[target..]
                .split('\n')
                .next()
                .unwrap_or_default()
                .char_indices()
                .nth(column)
                .map_or_else(
                    || {
                        self.input[target..]
                            .split('\n')
                            .next()
                            .unwrap_or_default()
                            .len()
                    },
                    |(i, _)| i,
                );
    }
    pub fn update(&mut self, input: Input) -> Vec<Action> {
        match input {
            Input::Tick => self.frame = (self.frame + 1) % 10,
            Input::Error(error) => self.error(error),
            Input::Agents(agents) => self.agents = agents,
            Input::Model(id, model) => {
                self.models.insert(id, model);
            }
            Input::Task(task) => {
                self.cost = task
                    .status
                    .as_ref()
                    .and_then(|s| s.usage.as_ref())
                    .map_or(0., |u| u.cost);
                self.task = task;
                if self.phase() == TaskPhase::Running {
                    self.running = true;
                } else {
                    if self.waiting
                        && self.running
                        && !self.received_answer
                        && self.phase() == TaskPhase::Awaiting
                    {
                        self.error("Task stopped before producing a response. Check the daemon log for details.".into());
                    }
                    self.waiting = false;
                }
            }
            Input::Wire(event) => match event.payload {
                Some(event::Payload::Message(payload)) => {
                    if let Some(message) = payload.message {
                        self.message(message);
                    }
                }
                Some(event::Payload::MessageChunk(chunk)) => {
                    self.partial.push_str(&chunk.chunk);
                    self.waiting = true;
                }
                Some(event::Payload::ToolCalled(tool)) => {
                    if let Some(call) = tool.tool_call {
                        self.tool(call);
                    }
                }
                Some(event::Payload::ToolResult(tool)) => {
                    if let Some(result) = tool.tool_result {
                        self.result(result);
                    }
                }
                Some(event::Payload::Task(task)) => {
                    if task.previous_phase.as_deref() == Some("running") {
                        self.running = true;
                    }
                    if task
                        .task
                        .as_ref()
                        .and_then(|t| t.metadata.as_ref())
                        .is_some_and(|m| m.id == self.task_id())
                    {
                        if let Some(task) = task.task {
                            self.update(Input::Task(task));
                        }
                        return vec![Action::FetchTask];
                    }
                }
                _ => {}
            },
            Input::Key(key, now) => {
                let ctrl = key.modifiers.contains(KeyModifiers::CONTROL);
                let alt = key.modifiers.contains(KeyModifiers::ALT);
                if ctrl && key.code == KeyCode::Char('c') {
                    if self
                        .last_ctrl_c
                        .is_some_and(|last| now.saturating_sub(last) < 1000)
                    {
                        return vec![Action::Quit];
                    }
                    self.input.clear();
                    self.cursor = 0;
                    self.last_ctrl_c = Some(now);
                    return vec![];
                }
                self.last_ctrl_c = None;
                if (ctrl
                    && matches!(
                        key.code,
                        KeyCode::Char('?')
                            | KeyCode::Char('/')
                            | KeyCode::Char('_')
                            | KeyCode::Char('7')
                    ))
                    || key.code == KeyCode::F(1) && self.show_help
                {
                    self.show_help = !self.show_help;
                    return vec![];
                }
                if self.show_help {
                    if matches!(key.code, KeyCode::Esc | KeyCode::Char('h' | 'H')) {
                        self.show_help = false;
                    }
                    return vec![];
                }
                match key.code {
                    KeyCode::F(1) => self.mode = UiMode::Input,
                    KeyCode::F(2) => self.mode = UiMode::Scroll,
                    KeyCode::Esc => {
                        self.waiting = false;
                        return vec![Action::Suspend];
                    }
                    KeyCode::Char('l') if ctrl => {
                        self.feed.clear();
                        self.partial.clear();
                    }
                    KeyCode::Char('r') if ctrl => return vec![Action::Reconnect],
                    KeyCode::Tab => {
                        if self.agents.len() > 1 {
                            let current = self.agent.metadata.as_ref().map(|m| &m.id);
                            let next = self
                                .agents
                                .iter()
                                .position(|a| a.metadata.as_ref().map(|m| &m.id) == current)
                                .map_or(0, |i| (i + 1) % self.agents.len());
                            self.agent = self.agents[next].clone();
                            return vec![
                                Action::FetchModel(self.model_id()),
                                Action::SwitchAgent(
                                    self.agent
                                        .metadata
                                        .as_ref()
                                        .map(|m| m.id.clone())
                                        .unwrap_or_default(),
                                ),
                            ];
                        }
                    }
                    KeyCode::Up if self.mode == UiMode::Scroll => {
                        self.scroll = self.scroll.saturating_add(1)
                    }
                    KeyCode::Down if self.mode == UiMode::Scroll => {
                        self.scroll = self.scroll.saturating_sub(1)
                    }
                    KeyCode::PageUp => self.scroll = self.scroll.saturating_add(self.page_height),
                    KeyCode::PageDown => self.scroll = self.scroll.saturating_sub(self.page_height),
                    KeyCode::Home if self.mode == UiMode::Scroll => self.scroll = usize::MAX,
                    KeyCode::End if self.mode == UiMode::Scroll => self.scroll = 0,
                    KeyCode::Char('k') if self.mode == UiMode::Scroll => {
                        self.scroll = self.scroll.saturating_add(1)
                    }
                    KeyCode::Char('j') if self.mode == UiMode::Scroll => {
                        self.scroll = self.scroll.saturating_sub(1)
                    }
                    KeyCode::Char('u') if ctrl => {
                        self.scroll = self.scroll.saturating_add(self.page_height / 2)
                    }
                    KeyCode::Char('d') if ctrl => {
                        self.scroll = self.scroll.saturating_sub(self.page_height / 2)
                    }
                    KeyCode::Enter if alt || ctrl => self.insert("\n"),
                    KeyCode::Enter => {
                        if !self.input.is_empty() {
                            let text = self.input.trim().to_owned();
                            self.input.clear();
                            self.cursor = 0;
                            self.waiting = true;
                            self.running = false;
                            self.received_answer = false;
                            self.scroll = 0;
                            return vec![Action::Send(text)];
                        }
                    }
                    KeyCode::Left => {
                        self.cursor = self.input[..self.cursor]
                            .char_indices()
                            .next_back()
                            .map_or(0, |(i, _)| i);
                    }
                    KeyCode::Right => {
                        if let Some(c) = self.input[self.cursor..].chars().next() {
                            self.cursor += c.len_utf8();
                        }
                    }
                    KeyCode::Up => self.vertical(false),
                    KeyCode::Down => self.vertical(true),
                    KeyCode::Home | KeyCode::Char('a') if key.code == KeyCode::Home || ctrl => {
                        self.cursor = self.line_start()
                    }
                    KeyCode::End | KeyCode::Char('e') if key.code == KeyCode::End || ctrl => {
                        self.cursor = self.line_end()
                    }
                    KeyCode::Char('k') if ctrl => {
                        let end = self.line_end();
                        self.input.drain(self.cursor..end);
                    }
                    KeyCode::Char('w') if ctrl => {
                        let before = &self.input[..self.cursor];
                        let end = before
                            .trim_end()
                            .rfind(char::is_whitespace)
                            .map_or(0, |i| i + 1);
                        self.input.drain(end..self.cursor);
                        self.cursor = end;
                    }
                    KeyCode::Backspace => {
                        if let Some((i, _)) = self.input[..self.cursor].char_indices().next_back() {
                            self.input.drain(i..self.cursor);
                            self.cursor = i;
                        }
                    }
                    KeyCode::Delete => {
                        if let Some(c) = self.input[self.cursor..].chars().next() {
                            self.input.drain(self.cursor..self.cursor + c.len_utf8());
                        }
                    }
                    KeyCode::Char(c) if !ctrl && self.mode == UiMode::Input => {
                        self.insert(&c.to_string())
                    }
                    _ => {}
                }
            }
        }
        vec![]
    }
}

pub const HELP: &str = "KEYBOARD SHORTCUTS\n\nGeneral:\n  Ctrl+?        - Toggle this help\n  Ctrl+C        - Clear input; twice within 1s to quit\n  Esc           - Suspend task execution\n  Ctrl+L        - Clear conversation\n  Ctrl+R        - Reconnect to task\n  Tab           - Switch agent\n\nInput Mode (F1):\n  Enter         - Send message\n  Alt+Enter     - New line (Ctrl+Enter also supported)\n  F2            - Switch to scroll mode\n\nScroll Mode (F2):\n  ↑↓, k/j       - Line up/down\n  PgUp/PgDn     - Page up/down\n  Ctrl+U/D      - Half page up/down\n  Home/End      - Go to top/bottom\n  F1            - Switch to input mode\n\nPress H or Esc to close this help.";

#[derive(Default)]
pub struct Picker {
    pub selected: usize,
}
impl Picker {
    pub fn key(&mut self, key: KeyEvent, count: usize) -> Option<bool> {
        match key.code {
            KeyCode::Esc | KeyCode::Char('q') => return Some(false),
            KeyCode::Char('c') if key.modifiers.contains(KeyModifiers::CONTROL) => {
                return Some(false)
            }
            KeyCode::Enter => return Some(count > 0),
            KeyCode::Up | KeyCode::Char('k') => self.selected = self.selected.saturating_sub(1),
            KeyCode::Down | KeyCode::Char('j') => {
                self.selected = (self.selected + 1).min(count.saturating_sub(1))
            }
            KeyCode::Home | KeyCode::Char('g') => self.selected = 0,
            KeyCode::End | KeyCode::Char('G') => self.selected = count.saturating_sub(1),
            KeyCode::PageUp | KeyCode::Char('b') => {
                self.selected = self.selected.saturating_sub(10)
            }
            KeyCode::PageDown | KeyCode::Char('f') => {
                self.selected = self
                    .selected
                    .saturating_add(10)
                    .min(count.saturating_sub(1))
            }
            _ => {}
        }
        None
    }
}

#[cfg(test)]
pub(super) mod tests {
    use super::*;

    pub fn task(phase: TaskPhase) -> Task {
        Task {
            metadata: Some(TaskMetadata {
                id: "task".into(),
                ..Default::default()
            }),
            status: Some(TaskStatus {
                phase: phase as i32,
                ..Default::default()
            }),
            ..Default::default()
        }
    }
    pub fn state() -> Session {
        Session::new(
            task(TaskPhase::Awaiting),
            Agent {
                metadata: Some(AgentMetadata {
                    id: "agent".into(),
                    ..Default::default()
                }),
                spec: Some(AgentSpec {
                    name: "Coder".into(),
                    model_id: "model".into(),
                    ..Default::default()
                }),
            },
        )
    }
    pub fn wire(payload: event::Payload) -> Input {
        Input::Wire(Event {
            payload: Some(payload),
            ..Default::default()
        })
    }
    fn key(state: &mut Session, code: KeyCode, modifiers: KeyModifiers) -> Vec<Action> {
        state.update(Input::Key(KeyEvent::new(code, modifiers), 100))
    }
    fn message(id: &str, role: MessageRole, parts: Vec<message_part::Data>) -> Input {
        wire(event::Payload::Message(MessageEvent {
            message: Some(Message {
                metadata: Some(MessageMetadata {
                    id: id.into(),
                    role: role as i32,
                    ..Default::default()
                }),
                spec: Some(MessageSpec {
                    content: parts
                        .into_iter()
                        .map(|data| MessagePart { data: Some(data) })
                        .collect(),
                }),
                status: Some(MessageStatus {
                    is_final_response: false,
                    usage: Some(MessageUsage {
                        input_tokens: 10,
                        output_tokens: 5,
                        ..Default::default()
                    }),
                }),
            }),
        }))
    }
    #[test]
    fn submit_empty_newline_and_unicode_editing() {
        let mut s = state();
        assert!(key(&mut s, KeyCode::Enter, KeyModifiers::NONE).is_empty());
        assert!(!s.spinning());
        key(&mut s, KeyCode::Char('é'), KeyModifiers::NONE);
        key(&mut s, KeyCode::Char('界'), KeyModifiers::NONE);
        key(&mut s, KeyCode::Left, KeyModifiers::NONE);
        key(&mut s, KeyCode::Delete, KeyModifiers::NONE);
        key(&mut s, KeyCode::Enter, KeyModifiers::ALT);
        key(&mut s, KeyCode::Char('x'), KeyModifiers::NONE);
        key(&mut s, KeyCode::Up, KeyModifiers::NONE);
        assert_eq!(s.cursor, "é".len());
        key(&mut s, KeyCode::Down, KeyModifiers::NONE);
        assert_eq!(s.cursor, s.input.len());
        assert_eq!(
            key(&mut s, KeyCode::Enter, KeyModifiers::NONE),
            vec![Action::Send("é\nx".into())]
        );
        assert!(s.input.is_empty());
        assert_eq!(s.cursor, 0);
        assert!(s.spinning());
    }
    #[test]
    fn chunks_complete_usage_replay_and_spinner() {
        let mut s = state();
        s.update(Input::Task(task(TaskPhase::Running)));
        for chunk in ["Hello", " world"] {
            s.update(wire(event::Payload::MessageChunk(MessageChunkEvent {
                chunk: chunk.into(),
                ..Default::default()
            })));
        }
        assert_eq!(s.partial, "Hello world");
        let final_message = message(
            "answer",
            MessageRole::Assistant,
            vec![message_part::Data::Text(message_part::Text {
                content: "Hello world".into(),
            })],
        );
        s.update(final_message.clone());
        s.update(final_message);
        assert!(s.partial.is_empty());
        assert_eq!(s.feed.len(), 1);
        assert!(matches!(&s.feed[0], FeedContent::Assistant(text) if text == "Hello world"));
        assert_eq!(s.usage.input_tokens, 10);
        s.update(Input::Task(task(TaskPhase::Awaiting)));
        assert!(!s.spinning());
        s.update(Input::Tick);
        assert_eq!(s.frame, 1);
    }
    #[test]
    fn submitted_message_reply_is_displayed_and_replay_deduplicates() {
        let mut s = state();
        key(&mut s, KeyCode::Char('q'), KeyModifiers::NONE);
        assert_eq!(
            key(&mut s, KeyCode::Enter, KeyModifiers::NONE),
            vec![Action::Send("q".into())]
        );
        let response = message(
            "sent",
            MessageRole::User,
            vec![message_part::Data::Text(message_part::Text {
                content: "q".into(),
            })],
        );
        s.update(response.clone());
        s.update(response);
        assert!(matches!(&s.feed[0], FeedContent::User(text) if text == "q"));
        assert_eq!(s.feed.len(), 1);
        assert!(s.waiting);
    }
    #[test]
    fn tool_parts_events_and_missing_variants_are_safe_and_deduplicated() {
        let mut s = state();
        let call = ToolCall {
            id: "call".into(),
            tool_name: "unknown".into(),
            ..Default::default()
        };
        let result = ToolResult {
            id: "result".into(),
            ..Default::default()
        };
        s.waiting = true;
        s.update(wire(event::Payload::ToolCalled(ToolCalledEvent {
            tool_call: Some(call.clone()),
            ..Default::default()
        })));
        s.update(wire(event::Payload::ToolResult(ToolResultEvent {
            tool_result: Some(result.clone()),
            ..Default::default()
        })));
        s.update(message(
            "tools",
            MessageRole::Assistant,
            vec![
                message_part::Data::ToolCall(call),
                message_part::Data::ToolResult(result),
            ],
        ));
        assert_eq!(s.feed.len(), 2);
        assert!(s.waiting);
        s.update(wire(event::Payload::ToolCalled(ToolCalledEvent::default())));
        s.update(wire(event::Payload::Message(MessageEvent::default())));
        s.update(Input::Wire(Event::default()));
        s.update(message("empty", MessageRole::Assistant, vec![]));
        assert!(!s.waiting);
        s.update(message(
            "error",
            MessageRole::System,
            vec![message_part::Data::Error(message_part::Error {
                message: "tool failed".into(),
            })],
        ));
        assert!(matches!(s.feed.last(), Some(FeedContent::Error(error)) if error == "tool failed"));
    }
    #[test]
    fn cancel_errors_and_two_press_quit() {
        let mut s = state();
        s.waiting = true;
        assert_eq!(
            key(&mut s, KeyCode::Esc, KeyModifiers::NONE),
            vec![Action::Suspend]
        );
        s.update(Input::Task(task(TaskPhase::Suspended)));
        assert!(!s.spinning());
        s.update(Input::Error("first".into()));
        s.update(Input::Error("second".into()));
        assert_eq!(s.feed.len(), 1);
        assert!(matches!(&s.feed[0], FeedContent::Error(error) if error == "second"));
        key(&mut s, KeyCode::Char('x'), KeyModifiers::NONE);
        assert!(key(&mut s, KeyCode::Char('c'), KeyModifiers::CONTROL).is_empty());
        assert!(s.input.is_empty());
        assert_eq!(
            key(&mut s, KeyCode::Char('c'), KeyModifiers::CONTROL),
            vec![Action::Quit]
        );
        s.update(Input::Key(
            KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL),
            2000,
        ));
        assert!(s
            .update(Input::Key(
                KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL),
                3100
            ))
            .is_empty());
    }
    #[test]
    fn stopped_without_response_is_reported_but_tool_turn_gap_is_not() {
        let mut s = state();
        s.waiting = true;
        s.update(wire(event::Payload::Task(TaskEvent {
            task: Some(task(TaskPhase::Awaiting)),
            previous_phase: Some("running".into()),
        })));
        assert!(matches!(s.feed.last(), Some(FeedContent::Error(_))));
        assert!(!s.spinning());
        s.feed.clear();
        s.waiting = true;
        s.update(message(
            "tool",
            MessageRole::Assistant,
            vec![message_part::Data::ToolCall(ToolCall::default())],
        ));
        s.update(Input::Task(task(TaskPhase::Awaiting)));
        assert!(!s
            .feed
            .iter()
            .any(|entry| matches!(entry, FeedContent::Error(_))));
    }
    #[test]
    fn help_scroll_switch_and_reconnect() {
        let mut s = state();
        key(&mut s, KeyCode::Char('7'), KeyModifiers::CONTROL);
        assert!(s.show_help);
        key(&mut s, KeyCode::Char('x'), KeyModifiers::NONE);
        assert!(s.input.is_empty());
        key(&mut s, KeyCode::Esc, KeyModifiers::NONE);
        assert!(!s.show_help);
        key(&mut s, KeyCode::F(2), KeyModifiers::NONE);
        key(&mut s, KeyCode::PageUp, KeyModifiers::NONE);
        assert_eq!(s.scroll, s.page_height);
        key(&mut s, KeyCode::End, KeyModifiers::NONE);
        assert_eq!(s.scroll, 0);
        key(&mut s, KeyCode::F(1), KeyModifiers::NONE);
        assert_eq!(s.mode, UiMode::Input);
        assert_eq!(
            key(&mut s, KeyCode::Char('r'), KeyModifiers::CONTROL),
            vec![Action::Reconnect]
        );
        assert!(key(&mut s, KeyCode::Tab, KeyModifiers::NONE).is_empty());
        let mut other = s.agent.clone();
        other.metadata.as_mut().unwrap().id = "other".into();
        s.update(Input::Agents(vec![s.agent.clone(), other]));
        assert_eq!(
            key(&mut s, KeyCode::Tab, KeyModifiers::NONE),
            vec![
                Action::FetchModel("model".into()),
                Action::SwitchAgent("other".into())
            ]
        );
        key(&mut s, KeyCode::Char('l'), KeyModifiers::CONTROL);
        assert!(s.feed.is_empty());
    }
    #[test]
    fn picker_navigation_and_empty_cancel() {
        let mut p = Picker::default();
        for (code, selected) in [
            (KeyCode::End, 24),
            (KeyCode::PageUp, 14),
            (KeyCode::Char('k'), 13),
            (KeyCode::Char('g'), 0),
            (KeyCode::Char('f'), 10),
        ] {
            assert_eq!(p.key(KeyEvent::new(code, KeyModifiers::NONE), 25), None);
            assert_eq!(p.selected, selected);
        }
        assert_eq!(
            p.key(KeyEvent::new(KeyCode::Enter, KeyModifiers::NONE), 25),
            Some(true)
        );
        assert_eq!(
            p.key(KeyEvent::new(KeyCode::Char('q'), KeyModifiers::NONE), 25),
            Some(false)
        );
        p.key(KeyEvent::new(KeyCode::End, KeyModifiers::NONE), 0);
        assert_eq!(p.selected, 0);
        assert_eq!(
            p.key(KeyEvent::new(KeyCode::Enter, KeyModifiers::NONE), 0),
            Some(false)
        );
    }
}
