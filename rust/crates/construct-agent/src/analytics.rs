//! Nonblocking analytics with Go-compatible event names and property keys.
use serde_json::{json, Value};
use std::sync::Mutex;

/// Go injects `backend/analytics.PostHogAPIKey` at build time; no runtime secret lookup.
pub const POSTHOG_API_KEY: &str = match option_env!("POSTHOG_API_KEY") {
    Some(key) => key,
    None => "",
};
pub const POSTHOG_ENDPOINT: &str = "https://eu.i.posthog.com";

#[derive(Clone, Debug, PartialEq)]
pub struct Event {
    pub distinct_id: String,
    pub event: String,
    pub properties: Value,
}

#[construct_core::async_trait]
pub trait Client: Send + Sync {
    fn enqueue(&self, event: Event);
    /// Stop accepting events and wait for pending delivery attempts.
    async fn close(&self);
}

#[derive(Default)]
pub struct InMemory {
    events: Mutex<Vec<Event>>,
}
impl InMemory {
    pub fn events(&self) -> Vec<Event> {
        self.events.lock().expect("analytics lock poisoned").clone()
    }
}
#[construct_core::async_trait]
impl Client for InMemory {
    fn enqueue(&self, event: Event) {
        self.events
            .lock()
            .expect("analytics lock poisoned")
            .push(event);
    }
    async fn close(&self) {}
}

#[derive(Default)]
pub struct Noop;
#[construct_core::async_trait]
impl Client for Noop {
    fn enqueue(&self, _: Event) {}
    async fn close(&self) {}
}

pub struct PostHog {
    sender: Mutex<Option<tokio::sync::mpsc::Sender<Event>>>,
    worker: tokio::sync::Mutex<Option<tokio::task::JoinHandle<()>>>,
}
impl PostHog {
    /// Construct inside a Tokio runtime. Supply the Go build-injected key explicitly
    /// when it is not supplied through the Rust build's POSTHOG_API_KEY.
    pub fn new() -> crate::Result<Self> {
        Self::with_key(POSTHOG_API_KEY)
    }
    pub fn with_key(key: &str) -> crate::Result<Self> {
        Self::with_endpoint(key, POSTHOG_ENDPOINT)
    }
    pub fn with_endpoint(key: &str, endpoint: &str) -> crate::Result<Self> {
        if key.is_empty() {
            return Err("posthog: API key is required".into());
        }
        let runtime = tokio::runtime::Handle::try_current()?;
        let http = reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(10))
            .build()?;
        let key = key.to_owned();
        let endpoint = format!("{}/capture/", endpoint.trim_end_matches('/'));
        let (sender, mut receiver) = tokio::sync::mpsc::channel::<Event>(1000);
        let worker = runtime.spawn(async move {
            while let Some(event) = receiver.recv().await {
                let mut properties = event.properties.as_object().cloned().unwrap_or_default();
                properties.insert("distinct_id".into(), Value::String(event.distinct_id));
                let _ = http
                    .post(&endpoint)
                    .json(&json!({"api_key": key, "event": event.event, "properties": properties}))
                    .send()
                    .await;
            }
        });
        Ok(Self {
            sender: Mutex::new(Some(sender)),
            worker: tokio::sync::Mutex::new(Some(worker)),
        })
    }
    pub async fn flush(&self) {
        self.close().await;
    }
}
#[construct_core::async_trait]
impl Client for PostHog {
    fn enqueue(&self, event: Event) {
        if let Some(sender) = self
            .sender
            .lock()
            .expect("analytics lock poisoned")
            .as_ref()
        {
            let _ = sender.try_send(event);
        }
    }
    async fn close(&self) {
        self.sender.lock().expect("analytics lock poisoned").take();
        if let Some(worker) = self.worker.lock().await.take() {
            let _ = worker.await;
        }
    }
}

macro_rules! emitter {
    ($name:ident, $event:literal, $($field:ident: $ty:ty),* $(,)?) => {
        pub fn $name(client: &dyn Client, $($field: $ty),*) {
            client.enqueue(Event { distinct_id: "user".into(), event: $event.into(), properties: json!({$(stringify!($field): $field),*}) });
        }
    };
}

emitter!(emit_agent_created, "agent_created", agent_id: &str, agent_name: &str, model_name: &str);
emitter!(emit_agent_updated, "agent_updated", agent_id: &str, agent_name: &str, updated_fields: Option<&[String]>);
emitter!(emit_model_provider_created, "model_provider_created", model_provider_id: &str, model_provider_name: &str, provider_type: &str);
emitter!(emit_model_created, "model_created", model_id: &str, model_name: &str, model_provider_id: &str, context_window: i32);
emitter!(emit_task_created, "task_created", task_id: &str, agent_id: &str);
emitter!(emit_task_updated, "task_updated", task_id: &str, updated_fields: Option<&[String]>);
emitter!(emit_agent_deleted, "agent_deleted", agent_id: &str, agent_name: &str);
emitter!(emit_model_provider_deleted, "model_provider_deleted", model_provider_id: &str, model_provider_name: &str, provider_type: &str);
emitter!(emit_model_deleted, "model_deleted", model_id: &str, model_name: &str, model_provider_id: &str);
emitter!(emit_task_deleted, "task_deleted", task_id: &str, agent_id: &str);
emitter!(emit_message_created, "message_created", message_id: &str, task_id: &str, role: &str);
emitter!(emit_message_deleted, "message_deleted", message_id: &str, task_id: &str, role: &str);
emitter!(emit_session_started, "session_started", session_type: &str, agent_name: &str, workspace: &str, has_existing_files: bool);
emitter!(emit_conversation_resumed, "conversation_resumed", task_id: &str, days_since_last_activity: i64, message_count: i64);
emitter!(emit_quick_ask, "quick_ask", agent_name: &str, max_turns: i64, files_included: i64, has_stdin_input: bool, question_length: i64);
emitter!(emit_tool_executed, "tool_executed", task_id: &str, tool_name: &str, execution_time_ms: i64, success: bool, error_type: &str);
emitter!(emit_code_interpreter_session, "code_interpreter_session", task_id: &str, scripts_executed: i64, errors_encountered: i64, lines_of_code: i64);
emitter!(emit_agent_handoff, "agent_handoff", from_agent_id: &str, to_agent_id: &str, task_id: &str, reason: &str, turn_number: i64);

#[cfg(test)]
mod tests {
    use super::*;
    #[tokio::test]
    async fn empty_noop_and_properties() {
        let client = InMemory::default();
        assert!(client.events().is_empty());
        emit_agent_created(&client, "a", "edit", "claude");
        emit_task_updated(&client, "t", None);
        emit_task_updated(&client, "t", Some(&[]));
        emit_tool_executed(&client, "t", "read_file", 42, false, "missing");
        assert_eq!(
            client.events()[0],
            Event {
                distinct_id: "user".into(),
                event: "agent_created".into(),
                properties: json!({"agent_id":"a", "agent_name":"edit", "model_name":"claude"})
            }
        );
        assert_eq!(client.events()[1].properties["updated_fields"], Value::Null);
        assert_eq!(client.events()[2].properties["updated_fields"], json!([]));
        assert_eq!(client.events()[3].properties["execution_time_ms"], 42);
        assert_eq!(client.events()[3].properties["error_type"], "missing");
        client.close().await;
        Noop.enqueue(client.events()[0].clone());
        Noop.close().await;
        assert!(PostHog::with_key("").is_err());
    }

    #[tokio::test]
    async fn posthog_nonblocking_delivery_shutdown_and_failure() {
        use axum::{extract::State, routing::post, Json, Router};
        use std::sync::Arc;
        let received = Arc::new(Mutex::new(Vec::<Value>::new()));
        let state = received.clone();
        let app = Router::new().route("/capture/", post(|State(state): State<Arc<Mutex<Vec<Value>>>>, Json(body): Json<Value>| async move {
            state.lock().unwrap().push(body);
            axum::http::StatusCode::OK
        })).with_state(state);
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let address = listener.local_addr().unwrap();
        let server = tokio::spawn(async move {
            axum::serve(listener, app).await.unwrap();
        });
        let client = PostHog::with_endpoint("test-key", &format!("http://{address}")).unwrap();
        emit_task_created(&client, "t", "a");
        client.close().await;
        emit_task_created(&client, "ignored", "a");
        client.close().await;
        let bodies = received.lock().unwrap().clone();
        assert_eq!(bodies.len(), 1);
        assert_eq!(bodies[0]["api_key"], "test-key");
        assert_eq!(bodies[0]["event"], "task_created");
        assert_eq!(
            bodies[0]["properties"],
            json!({"distinct_id":"user", "task_id":"t", "agent_id":"a"})
        );
        server.abort();
        let client = PostHog::with_endpoint("test-key", "http://127.0.0.1:0").unwrap();
        emit_task_created(&client, "t", "a");
        client.flush().await;
    }
}
