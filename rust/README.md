# Rust foundation

The existing Go implementation remains unchanged. These crates provide the shared
contracts for parallel Rust implementations; they do not yet run a Rust daemon.

## Crate map

| Crate | Responsibility |
| --- | --- |
| `construct-proto` | prost messages, pbjson proto3 JSON, generated Connect service traits/servers/clients |
| `construct-connect` | Hand-written Connect unary/streaming runtime, HTTP(S)/Unix transports, interceptors |
| `construct-types` | Domain records, stored Go JSON, tools, provider messages, skill lockfiles, event patterns |
| `construct-core` | Dyn-safe model/tool/secret/store/event contracts and reference in-memory event router |
| `construct-tools` | Host filesystem, command, HTTP and Store-backed communication tools; [parity and compatibility notes](crates/construct-tools/README.md) |

Crates live in `crates/`. Core depends only on domain types and infrastructure,
never generated protobuf types, concrete database drivers, provider SDKs, or CLI.
Public core/type items are checked with `deny(missing_docs)`.

## Build and test

From `rust/`, with Rust 1.89.0 or newer (the locked uuid dependency requires 1.89):

```sh
cargo build --workspace
cargo test --workspace
cargo clippy --workspace --all-targets -- -D warnings
cargo fmt --check

# Also compile and exercise the explicitly opt-in test doubles:
cargo build --workspace --all-features
cargo test --workspace --all-features
cargo clippy --workspace --all-features --all-targets -- -D warnings
```

`.github/workflows/rust.yml` pins both Linux jobs to stable release 1.89.0,
matching the inherited workspace MSRV. The jobs run all-feature checks and real
Go interop separately. When increasing the MSRV, update the workflow pin too.

Core's `testing` feature exposes `MemorySecretProvider`, `MemoryStore`, and
`ScriptedModelProvider`. Consumers should enable it only in dev-dependencies.
Plain tests are hermetic: real-Go tests return with a skip notice unless
`CONSTRUCT_GO_BIN` points to a built Go binary (`--nocapture` displays notices).

### Real Go interoperability (Linux)

With Go 1.24.1+ and Rust installed, from any directory:

```sh
/path/to/construct/rust/scripts/go-interop.sh
# Or use an already-built binary:
CONSTRUCT_GO_BIN=/absolute/path/to/construct cargo test \
  --manifest-path /path/to/construct/rust/Cargo.toml \
  -p construct-proto --test go_interop -- --nocapture --test-threads=1
```

The script builds Go with `CGO_ENABLED=0`. Tests use isolated HOME/XDG directories
and `secret.provider: file`; no Secret Service, D-Bus, API keys, or model inference
are needed. They exercise proto/JSON over Unix and authenticated TCP, including
unary CRUD/errors, a live event stream, and Go CLI -> Rust Unix server.

The Go daemon currently fails `--listen-http` during context upsert (`invalid
kind: tcp`: context validation accepts `http`). TCP tests instead pass a bound
loopback socket on FD 3 using the existing Linux systemd activation path, without
requiring a running systemd service. Auth remains the actual Go TCP Bearer path.

## Code generation

`construct-proto/build.rs` reads Construct schemas directly from `../api/def`;
that remains the single source of truth. Do not copy/edit generated Rust files.
`protoc-bin-vendored` removes the system protoc requirement. Required third-party
schemas are licensed/vendored under `construct-proto/proto-vendor`.

The extensible prost `ConnectServiceGenerator` emits service metadata, object-safe
async traits, clients, and axum-compatible routers. Prost/pbjson outputs go into
Cargo's OUT_DIR. Regeneration happens automatically on schema/build-script changes.
Generated handlers use `construct-connect::async_trait`; core uses the same macro
through `construct_core::async_trait`.

## Go JSON compatibility

Protobuf JSON is pbjson's proto3 mapping; **stored domain JSON is different**.
For persisted domain values use `construct_types::to_go_json`, not ordinary
`serde_json::to_string`:

- Go field tags **and declaration order** are preserved, including untagged
  PascalCase fields (`Text`, filesystem inputs) and `exitCode`.
- Maps are BTreeMaps (Go sorts keys). Non-omitempty nil slices/maps are `None`
  (`null`), allocated empties are `Some(empty)` (`[]`/`{}`). Omitempty collections
  omit both; pointers to zero remain present. Go does not omit zero structs.
- The helper provides Go's HTML/U+2028/U+2029 escaping and float formatting.
  Typed domain float fields reject non-finite values.
- `RawValue` preserves raw argument order/number spellings. The helper compacts
  its whitespace and escapes it as Go does, without sorting raw object keys.
- Timestamp wraps chrono UTC, defaults to year one, and trims fractional zeros
  like RFC3339Nano. Non-UTC inputs normalize to UTC.
- Decimal is a canonical arbitrary-precision string, not floating-point money.
  Callers supply shopspring/decimal canonical spelling; no arithmetic/normalizer
  is provided. String-backed enums retain unknown and zero strings.

Twenty Go-generated fixtures and generation instructions are in
`construct-types/tests/fixtures`. The real generator runs outside the repository;
its committed source is `scripts/types-golden.go.txt` so Go tooling ignores it.
Entity records intentionally have no serde implementation. JSON columns are
Task.tool_uses, Message.content/usage, and Model.capabilities.

## Implementing the core contracts

All traits are Send + Sync and usable behind Arc<dyn Trait>. Async methods use
`#[construct_core::async_trait]` on both trait implementations and declarations.
I/O implementations should honor cancellation and avoid blocking executor threads.

### ModelProvider

Implement `model::ModelProvider::invoke_model(&self, model, system_prompt,
messages, options)`. Return a provider-domain Message, not a protobuf message.
Translate ToolDefinition.input_schema (native Go uses an object schema with
properties and optional required); orchestration executes the tools separately.
Invoke text_sink in generation order and retry_callback before retries, without
holding internal locks. Retry/circuit-breaker policy belongs to implementations.
Observe InvokeOptions.cancellation and return ProviderErrorKind::CANCELED.

Go defines Anthropic/OpenAI/Gemini profiles, used in invocation options rather
than persisted schema fields. ModelProfile is a Rust-only enum; the concrete
structs retain Go JSON tags, omitempty, numeric nanosecond timeouts, and nullable
Gemini parameters. `validate` applies Go's validation/default mutations (also
rejecting non-finite parameters). None selects the provider's default invocation
profile, e.g. Anthropic max_tokens=8192; reject a mismatched explicit variant.
Do not log profiles containing credentials. Other provider kinds do not currently
have distinct Go profile types; compatible implementations select the suitable one.

For tests, ScriptedModelProvider consumes ModelStep values, reports optional
retry/chunk callbacks without sleeping, records input metadata, and returns the
scripted final message/error. Exhaustion and expected-model mismatches are errors.
It does not simulate provider-specific SDK/profile policy.

### Tool

Implement name, description, input_schema, and execute(ctx, Value) -> ToolOutput.
Tools decode their JSON input and return the matching construct-types ToolOutput
arm (not an untyped provider string). Native provider stringification and VM
bindings belong in adapters. Base/codeact error codes share one ToolErrorCode;
`ToolError::new` preserves standard messages/hints, and `custom` appends the
generic recovery hint. Details are structured JSON, not secret-bearing logs.

ToolContext carries workspace root, optional task/agent UUIDs, CancellationToken,
and optional EventPublisher. It is not a filesystem sandbox. Constructor-inject
filesystem abstractions, command runners, HTTP clients, user-I/O, and Arc<dyn
Store> for handoff. Go's base ask-user executor is currently a stub, so no fictional
ask-user transport is baked into the core. Tool implementations own safe path
resolution, cancellation of subprocesses, and permission policy.

### SecretProvider

Implement async get/set/delete with string keys/plaintext values. Set overwrites;
delete is idempotent (Go file-provider behavior). Match NotFound by variant, not
backend cause text. Marshal/TooLarge mirror Go typed failures; Internal covers I/O.
Use ENCRYPTION_KEY_SECRET and model_provider_secret_key(UUID); do not change their
spelling. Encryption/associated-data handling stays outside this storage trait.
MemorySecretProvider has no persistence, encryption, or backend size limits.

### EventPublisher / EventSubscriber

Publish synchronously and non-blockingly with respect to receiver capacity.
Subscribe returns an owned Subscription implementing Stream<Item=Arc<Event>>;
dropping it unsubscribes. Alternate subscribers can wrap any EventStream with
`Subscription::new(stream, cleanup)`; cleanup stops delivery without blocking I/O.
Event payload is Arc<dyn Any + Send + Sync>, matching Go's domain `any`; consumers
downcast to entity/tool payloads. API conversion is a separate concern.

EventRouter is the reference implementation: zero capacity selects 100; full
queues drop the **new** event; close/cancel drain queued events then end. Empty
patterns become `*`, explicit internal patterns are removed for external clients,
and wildcards never match internal events even with Internal=true. Task UUID
scope is optional; an invalid string is ignored as in Go. ReplayAfterMessageID is
an API/store replay concern (Go's router ignores it). Select next() against a
CancellationToken and drop on cancellation. Match-pattern tables are ported in
construct-types; core tests port the router lifecycle/filter/backpressure cases.

### Store and per-entity repositories

Implement AgentStore, TaskStore, MessageStore, ModelStore, ModelProviderStore,
TokenStore, then Store::begin. Each repository has uniquely named
create_ENTITY(record), get_ENTITY(id), update_ENTITY(record), delete_ENTITY(id),
and list_ENTITIES(filter, ListOptions) methods so Store is unambiguous as dyn.

Writes use complete scalar records. Callers assign UUIDs/timestamps and apply
insert defaults; update preserves immutable create_time/id and replaces mutable
fields. Repository methods are atomic and enforce database constraints. Empty
lists return Vec::new; missing get/update/delete is NotFound. Unique/FK/check
failures and optimistic conflicts are ConstraintViolation; bad inputs are Invalid.
Do not expose backend errors containing plaintext secrets.

Filters are ANDed, with OR within nonempty name/source/provider-type vectors.
They cover API predicates, task lifecycle reconciliation, message replay bounds,
and token hash lookup. API include_expired=false supplies now as expires_after;
explicit time keeps tests deterministic. Pagination runs after filtering/order;
None limit is unbounded, Some(0) empty. Default order is create_time descending,
UUID tiebreaker. Use ascending for conversation/replay. Name sort is invalid for
Task/Message. API adapters own page-token encoding; Go currently only applies
pagination/sorting in the task handler, but the Rust contract supports all lists.
Task UUID prefixes mirror Go's SQLite LIKE expression over canonical UUID text:
append `%`, compare ASCII case-insensitively, and retain `%`/`_` wildcard semantics.
Token name prefixes also compare ASCII case-insensitively, but escape wildcards
as Ent's NameHasPrefix does (unlike the custom UUIDHasPrefix helper).

`StoreTransaction: Store` exposes the same repositories plus consuming async
commit/rollback. Call `store.begin().await`, operate only on that returned object,
then commit. Drop, cancellation, panic, or failed commit must roll back without
partial writes. Nested begin returns Invalid. SQLx implementations can hold a
pool-owned transaction behind a tokio mutex; rusqlite implementations can use a
dedicated blocking connection actor and rollback-on-owner-drop. Do not require a
borrowed transaction/connection to escape as a 'static trait object.

MemoryStore is a simple HashMap fake, not a replacement for SQL conformance tests:
it enforces key/reference constraints, immutable create_time, and task-message
cascade, but not every Ent validator/default and does not emit events. It clones
transaction snapshots, exposes no uncommitted writes, and rejects commits if any
root write has changed the revision. Rollback/drop discard the snapshot. Its
serializable conflict behavior is intentionally stricter than row-level SQL.

### Skills and CLI sessions

Use construct-types::skill LockFile/LockEntry/GitSource/URLSource for disk JSON.
There is intentionally no speculative skill-manager or CLI trait in core: this
step defines their shared store/secrets/tools/events dependencies, not installer
policy or presentation. Keep those adapters separate from protobuf codegen and
the database/model implementations.
