use construct_core::store::*;
use construct_store::SqliteStore;
use construct_types::Timestamp;
use sqlx::{Connection, Row, SqliteConnection};
use std::{path::Path, process::Command};
use uuid::Uuid;

fn id(n: u32) -> Uuid {
    Uuid::parse_str(&format!("00000000-0000-0000-0000-{n:012}")).unwrap()
}
fn time(text: &str) -> Timestamp {
    serde_json::from_value(serde_json::json!(text)).unwrap()
}

fn fixture(binary: &std::ffi::OsStr, operation: &str, path: &Path) {
    let output = Command::new(binary)
        .arg(operation)
        .arg(path)
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "Go {operation}: {}",
        String::from_utf8_lossy(&output.stderr)
    );
    print!("{}", String::from_utf8_lossy(&output.stdout));
}

async fn schema(path: &Path) -> Vec<(String, String, String, Option<String>)> {
    let mut connection =
        SqliteConnection::connect_with(&sqlx::sqlite::SqliteConnectOptions::new().filename(path))
            .await
            .unwrap();
    sqlx::query("SELECT type, name, tbl_name, sql FROM sqlite_master ORDER BY type, name")
        .fetch_all(&mut connection)
        .await
        .unwrap()
        .into_iter()
        .map(|r| {
            (
                r.get(0),
                r.get(1),
                r.get(2),
                r.get::<Option<String>, _>(3)
                    .map(|s| s.split_whitespace().collect::<Vec<_>>().join(" ")),
            )
        })
        .collect()
}

#[tokio::test]
async fn go_database_bidirectional_records_json_and_schema() {
    let Some(binary) = std::env::var_os("CONSTRUCT_GO_STORE_FIXTURE") else {
        eprintln!("skipping Go store interop: set CONSTRUCT_GO_STORE_FIXTURE or run rust/scripts/go-interop.sh");
        return;
    };
    let directory = tempfile::tempdir().unwrap();
    let go_path = directory.path().join("go.db");
    let rust_path = directory.path().join("rust.db");
    fixture(&binary, "create", &go_path);
    let original_schema = schema(&go_path).await;
    let store = SqliteStore::open(&go_path).await.unwrap();
    assert_eq!(
        schema(&go_path).await,
        original_schema,
        "opening Go DB must not alter its schema"
    );
    let fresh = SqliteStore::open(&rust_path).await.unwrap();
    drop(fresh);
    assert_eq!(
        schema(&rust_path).await,
        original_schema,
        "fresh schemas must match including indexes and FKs"
    );
    drop(SqliteStore::open(&rust_path).await.unwrap());
    assert_eq!(
        schema(&rust_path).await,
        original_schema,
        "migration is idempotent"
    );

    let created = time("2025-02-03T04:05:06.123456789Z");
    let updated = time("2025-02-03T04:05:07.123456789Z");
    let mut monotonic = store.get_agent(id(7)).await.unwrap();
    assert!(monotonic.create_time > created);
    let mut offset = store.get_agent(id(8)).await.unwrap();
    assert_eq!(offset.create_time, created);
    assert_eq!(offset.update_time, updated);
    monotonic.id = id(107);
    monotonic.name = "rust-monotonic-time".into();
    offset.id = id(108);
    offset.name = "rust-offset-time".into();
    store.create_agent(monotonic).await.unwrap();
    store.create_agent(offset).await.unwrap();
    let mut p = store.get_model_provider(id(1)).await.unwrap();
    assert_eq!(p.name, "provider <&>");
    assert_eq!(p.provider_type.as_ref(), "anthropic");
    assert_eq!(p.url, "https://example.test");
    assert_eq!(p.secret, [0, 1, 127, 128, 255]);
    assert!(p.enabled);
    assert_eq!(p.create_time, created);
    assert_eq!(p.update_time, updated);
    let mut m = store.get_model(id(2)).await.unwrap();
    assert_eq!(m.name, "model");
    assert_eq!(m.context_window, 123456);
    assert_eq!(
        m.capabilities,
        Some(vec!["image".into(), "prompt_cache".into()])
    );
    assert_eq!(
        (
            m.input_cost,
            m.output_cost,
            m.cache_write_cost,
            m.cache_read_cost
        ),
        (1.25, 2.5, 0.75, 0.125)
    );
    assert!(m.enabled);
    assert_eq!(m.alias, "alias");
    assert_eq!(m.model_provider_id, id(1));
    assert_eq!(m.create_time, created);
    assert_eq!(m.update_time, updated);
    let mut a = store.get_agent(id(3)).await.unwrap();
    assert_eq!(a.name, "agent");
    assert_eq!(a.description, "description");
    assert_eq!(a.instructions, "instructions <&>\u{2028}");
    assert!(a.builtin);
    assert_eq!(a.model_id, Some(id(2)));
    assert_eq!(a.create_time, created);
    assert_eq!(a.update_time, updated);
    let mut t = store.get_task(id(4)).await.unwrap();
    assert_eq!(t.project_directory, "/workspace");
    assert_eq!(
        (
            t.input_tokens,
            t.output_tokens,
            t.cache_write_tokens,
            t.cache_read_tokens
        ),
        (101, 102, 103, 104)
    );
    assert_eq!(t.cost, 1.25);
    assert_eq!(t.turns, 9);
    assert_eq!(t.tool_uses.as_ref().unwrap().get("a<&>"), Some(&1));
    assert_eq!(t.tool_uses.as_ref().unwrap().get("z"), Some(&2));
    assert_eq!(t.phase.as_ref(), "awaiting");
    assert_eq!(t.desired_phase.as_ref(), "running");
    assert_eq!(t.description, "task");
    assert_eq!(t.agent_id, Some(id(3)));
    assert_eq!(t.create_time, created);
    assert_eq!(t.update_time, updated);
    let mut msg = store.get_message(id(5)).await.unwrap();
    assert_eq!(msg.source.as_ref(), "assistant");
    let blocks = msg.content.as_ref().unwrap().blocks.as_ref().unwrap();
    assert_eq!(blocks.len(), 2);
    assert_eq!(blocks[0].kind.as_ref(), "text");
    assert_eq!(blocks[0].payload, "hello <&>\u{2028}");
    assert_eq!(blocks[1].kind.as_ref(), "tool_call");
    assert_eq!(blocks[1].payload, "{\"name\":\"shell\"}");
    let usage = msg.usage.as_ref().unwrap();
    assert_eq!(
        (
            usage.input_tokens,
            usage.output_tokens,
            usage.cache_write_tokens,
            usage.cache_read_tokens,
            usage.cost
        ),
        (11, 12, 13, 14, 0.125)
    );
    assert_eq!(msg.processed_time, Some(updated.clone()));
    assert_eq!(msg.task_id, id(4));
    assert_eq!(msg.agent_id, Some(id(3)));
    assert_eq!(msg.model_id, Some(id(2)));
    assert_eq!(msg.create_time, created);
    assert_eq!(msg.update_time, updated);
    let mut token = store.get_token(id(6)).await.unwrap();
    assert_eq!(token.name, "token");
    assert_eq!(token.r#type.as_ref(), "api_token");
    assert_eq!(token.token_hash, [9, 0, 255]);
    assert_eq!(token.description, "token description");
    assert_eq!(token.expires_at, time("2025-02-04T04:05:06.123456789Z"));
    assert_eq!(token.create_time, created);
    assert_eq!(token.update_time, updated);

    let tx = store.begin().await.unwrap();
    p.id = id(101);
    tx.create_model_provider(p).await.unwrap();
    m.id = id(102);
    m.model_provider_id = id(101);
    tx.create_model(m).await.unwrap();
    a.id = id(103);
    a.name = "rust-agent".into();
    a.model_id = Some(id(102));
    tx.create_agent(a).await.unwrap();
    t.id = id(104);
    t.agent_id = Some(id(103));
    tx.create_task(t).await.unwrap();
    msg.id = id(105);
    msg.task_id = id(104);
    msg.agent_id = Some(id(103));
    msg.model_id = Some(id(102));
    tx.create_message(msg).await.unwrap();
    token.id = id(106);
    token.name = "rust-token".into();
    token.token_hash = vec![10, 0, 255];
    tx.create_token(token).await.unwrap();
    tx.commit().await.unwrap();
    fixture(&binary, "verify", &go_path);
    // Ent's migrator must also accept a Rust-created database unchanged.
    fixture(&binary, "schema", &rust_path);
    assert_eq!(schema(&rust_path).await, original_schema);
    assert_eq!(
        store
            .list_messages(
                MessageFilter {
                    processed: Some(true),
                    ..Default::default()
                },
                ListOptions::default()
            )
            .await
            .unwrap()
            .len(),
        2
    );
}
