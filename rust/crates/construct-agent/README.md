# Agent runtime and supporting services

This crate ports `backend/agent`'s supporting services, `backend/prompt`, and
`backend/analytics`, including the stored-message reconciler and worker runtime.
Go remains in place; there are no new protobufs or stored formats.

- `conv`: memory records ↔ model messages, parsed tool calls/results and usage.
  Stored payloads use `construct_types::to_go_json`, including nil slices,
  declaration order and HTML escaping. Go intentionally discards tool result
  text when converting model responses into stored tool-result blocks.
- `environment`: sorted immediate project entries, shell fallback and ordered
  developer-tool discovery. Injectable lookup helpers keep tests hermetic.
- `prompt`: embeds the original markdown files without copies. The renderer
  supports fields, `if`, indexed tool ranges and left whitespace trimming used
  there; other syntax is rejected. Like Go, tools are inserted only where the
  agent's instructions request them. Skills are formatted through
  `construct_skill::format_available_skills`; Go's current edit/plan markdown
  does not reference `.AvailableSkills`, so those prompts omit discovered skills.
- `provider`: loads providers through core's store contract and decrypts with
  `model_provider:<uuid>`. xAI deliberately uses Go's `api.xai.com/v1` override.
- `title`: first five messages, user-message gate, Anthropic budget model,
  assistant prefill, two retries, quote trimming and 80-byte limit, with an
  optional store-backed title update wrapper. The standalone generation function
  accepts a scripted provider and explicit provider kind for deterministic tests.
- `metrics`: isolated Prometheus registry, the seven named workqueue vectors
  and exponential buckets from Go, plus the process collector on Linux.
- `analytics`: all Go event emitters, in-memory/no-op clients and a bounded
  nonblocking PostHog sender using `https://eu.i.posthog.com`. Go's key is a
  build-injected package variable; Rust uses build-time `POSTHOG_API_KEY` or
  explicit `PostHog::with_key`, never a runtime credential lookup. A full queue
  drops events. `close().await` (also exposed as `flush`) stops acceptance and
  waits for delivery attempts. Unlike the Go SDK this sender does not batch or
  retry failed sends.
- `reconciler`: Go's system/assistant/user pending-message priority, model history,
  streaming chunks, atomic input/assistant/usage/cost updates and CodeAct execution
  with production interceptors. Logical phases `await_input`, `execute_tools`,
  `invoke_model`, `suspended` map to Go's stored `awaiting`, `running`, `suspended`.
  Like Go, every pass publishes its phase, then restores `awaiting`, even for a
  suspended task; `desired_phase=suspended` continues to gate execution. Tool
  results are system messages; Go publishes neither their creation nor input
  processing updates. Go's per-phase (rather than cumulative) tool-use update is
  retained. Runtime-owned write transactions are serialized for SQLite snapshots.
- `queue`: dirty/processing deduplication, earliest-deadline delayed adds, and all
  workqueue metrics labeled `name="construct"`. Retry counters count delayed
  provider requeues. New triggers received during processing cause one more pass.
- `runtime`: 50 workers by default, internal trigger/suspend subscriptions, shared
  cancellation, provider factory injection and graceful worker/title/analytics
  shutdown. `Runtime::open_sqlite` installs store event hooks; `Runtime::new`
  accepts `Arc<dyn Store>`, whose trait has no hook-registration method. It exposes
  store/encryption/router accessors without depending on an API-server crate.
- `wire::message_status`: shared protobuf status projection for Rust RPC/event
  adapters. Completed assistant messages without tool calls are final; tool-call
  and unfinished messages are not. This branch has no Rust API server. The existing
  Go RPC/event conversion in `backend/api/conv/message.go` is fixed at its common
  status construction site, so Go clients receive the corrected final flag too.

## Compatibility limits

Malformed nil provider metadata returns an error instead of Go's nil-pointer
panic. Go can truncate a title in the middle of a UTF-8 codepoint; Rust strings
cannot contain invalid UTF-8, so truncation backs up to a character boundary. ASCII
truncation is identical. Non-UTF-8 filesystem names are displayed lossily.
Windows shell/PATH behavior is implemented but unverified on Windows; process
collection is Linux-only, matching the Rust Prometheus collector's support.

Title work is started at most once per task per runtime, including failed title
attempts (Go singleflight only deduplicates overlapping attempts). Successful
titles still use the step-1 Anthropic title generator and are stored on the task.
Shutdown waits for workers instead of returning after Go's five-second timeout;
model cancellation marks its input processed using a live store transaction.
Streaming chunk IDs match the persisted assistant ID (Go generates separate IDs).
Malformed tool-call JSON is recorded in `reconciler().last_error(task_id)`, rather
than silently discarded as Go does; input processing/requeue behavior is retained.
`/tmp/tool_call` debug logging is omitted: repository searches found no readers.
Runtime errors are retained through `last_error` rather than published as messages,
consistent with Go's currently unimplemented `publishError` event path.

## Tests

```sh
python3 rust/scripts/agent-goldens.py # repository root; needs Go
cd rust
cargo +1.89.0 test -p construct-agent --all-features --locked
```

Fixtures are generated by the unchanged Go conversion/template/analytics code
and Tink encryption. Encryption material in `tests/fixtures/provider.json` is
test-only. Normal tests do not call model APIs or send analytics.
Runtime tests use SQLite and core's scripted provider, including streaming/tool
event order, durable tool JSON, costs, cancellation, retries, deduplication, titles,
hooks, empty history and terminal errors. Go status conversion regression tests:
`CGO_ENABLED=0 go test ./backend/api/conv ./backend/api` (repository root).
