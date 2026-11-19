# construct-codeact

JavaScript CodeAct interpreter over `construct-core::tool::Tool` and the host
implementations in `construct-tools`. Supports Rust 1.89. There is no daemon,
provider SDK, Go subprocess or system JavaScript library dependency.

## Layout and API

- `src/lib.rs`: task, session and durable function-call state.
- `src/bindings.rs`, `src/mapping.js`: positional argument conversion, synchronous
  host calls, Go-style print formatting, constructors and descriptions.
- `src/middleware.rs`: statistics, durable calls, transient typed events, reset.
- `src/replay.rs`: ordered stored-result matching, tolerant of Go's duplicate indices.
- `src/result.rs`: retained Go struct/slice identity for printing tool results.
- `src/interpreter.rs`: isolated QuickJS runtime, strict mode, cancellation and
  interpreter errors with partial output.
- `tests/compatibility.rs`: positional mappings, errors, middleware, print,
  cancellation and real filesystem/command integration.

Constructors: `create_file`, `read_file`, `edit_file`, `list_files`, `grep`,
`find_file`, `execute_command`, `fetch`, `submit_report`, `handoff(store)`, `print`.
`host_tool(Arc<dyn Tool>)` permits injected host implementations. Descriptions
come directly from the host tools, except print's verbatim rendered Go text.
`Interpreter::new(tools, interceptors).interpret(context, input, task).await`
returns `(InterpreterOutput, Option<InterpreterError>)`, retaining console output,
successful calls and attempt counts on failure. Persist with
`construct_types::to_go_json`, never ordinary serde JSON.

Binding arguments remain QuickJS values rather than serde values: serde cannot
distinguish undefined from null, execute accessors, or implement JavaScript's
String/ToInteger/ToBoolean coercions. `CodeActTool::input` consequently accepts
`&[rquickjs::Value]` and returns a VM Result, so conversion exceptions remain
JavaScript exceptions. A mapped input is the existing domain `ToolInput` union.
The dispatcher converts once and passes an immutable `Invocation` to middleware
and `CodeActTool::call`; neither layer reads the original arguments again. Print
alone exports its original values. Property accessors within mapping are also
read once, including fetch fields/headers, report fields and array-like lengths.

### Resuming stored calls

Neither the Go interpreter nor the initial Rust port had a resume consumer:
`interpret` always allocated fresh state, and `DurableFunction` only recorded
calls. `Interpreter::resume(context, input, task, FunctionCallState).await` now
re-runs a script using its stored successful results, then executes new calls.
Deserialize `InterpreterOutput.function_calls` with the existing domain types
and supply those records as `FunctionCallState.calls`.

Replay matches **tool name plus converted input in stored vector order**, not
numeric index. Matching a later record skips obsolete Go coercion records; this
handles both `list_files@0,@1,@2, read_file@1` and duplicate-index histories without
confusing the outer read with a list result. Every actual logical invocation
gets one newly recorded result, attempt count and event pair. Indices are rebuilt
sequentially in completion order from zero; stored record indices and the legacy
state's `index` are not cursor offsets. Original stored records are not mutated.

Cached calls restore their JS result and temporary result slot without executing
the host again. Once the stored suffix is exhausted, each new invocation executes
once. If no matching tool/input remains while stored records are unconsumed, or
the matched output is missing its result arm, replay returns a catchable error
without executing that host call. This is deliberately fail-closed rather than
guessing and repeating a side effect. Callers must supply the original script's
inputs and a compatible workspace; replay does not undo Go's historical side
effects or rewrite cached result content to pretend they happened only once.

## Dependencies and execution

`rquickjs` **0.9.0** embeds vendored QuickJS, built by the `cc` crate without
system QuickJS dependencies. It builds with the locked workspace on Rust 1.89.
The synchronous VM runs entirely within `tokio::task::spawn_blocking`; bindings
drive host async execution using the captured Tokio runtime handle. Interrupt
callbacks check `ToolContext.cancellation`, including for infinite JS loops.
The remaining dependencies are existing workspace contracts, serde JSON, Tokio
and UUID; tempfile is test-only.

## Go semantics and quirks

- Interceptors wrap in list order: **the last is outermost**.
  `production_interceptors(publisher)` gives stats, durable, events, reset.
- Print is excluded from statistics, durable calls and tool events. Reset still
  runs for print. Statistics increment before the host invocation; throwing
  calls neither record a durable call nor publish a result event.
- Durable indexes advance only after successful calls. Handoff returns undefined
  and sets no result: it records an empty output union, with no result event.
- Conversion completes before the outer call's middleware. A getter/coercion's
  nested calls therefore complete and record first, with unique sequential
  indices. Temporary results are cleared at binding entry and again by the reset
  wrapper before execution, so a nested result cannot leak into an outer handoff.
- Events have `tool.called`/`tool.result` types, `created` action, current UTC time,
  task scope, empty IDs, and typed `ToolCallEvent`/`ToolResultEvent` payloads.
- The interpreter preserves Go's unusual literal `use strict;` prefix test.
  Schema properties were checked against invopop's actual Go reflection output;
  outer `$schema`, `required`, and additionalProperties are intentionally absent.
- Session agent IDs initially remain nil, as in Go's `Interpret`; a host
  `ToolContext.agent_id` is not silently substituted into the binding input.
- Print uses Go-style space-separated values, newline, `<nil>`, bracketed arrays,
  sorted `map[k:v]` objects, and int64 versus float64 numeric formatting. Host
  results retain Go struct identity: `print(read_file(path))` uses `&{path content}`,
  while a JS object copy uses `map[content:... path:...]`. Struct fields print in
  declaration order, including zero-valued fields omitted from stored JSON.
  Reading a struct-valued field or slice element and printing it directly uses
  its addressable `&{...}` form; aggregates inline their constituent structs.
- Tool errors use core's Display, with the Go session's empty `toolName` detail.
  They are catchable Error objects; uncaught tool errors return their message.

## Compatibility boundaries

This is not a byte-for-byte replacement for Sobek for arbitrary JavaScript:

- Native uncaught script errors use QuickJS's error wording and omit Sobek's
  `<eval>:line:column(program-counter)` stack suffix. Tool-error messages and
  the cancellation message `execution cancelled` are preserved. In the Go golden
  corpus TypeError/ReferenceError messages match exactly after stripping only
  that numeric Sobek location suffix; other engine-specific diagnostics are not
  guaranteed to match.
- Function references cannot print Go's process-specific function pointer
  addresses. Exotic exported JS values and prototype mutation are not golden
  parity guarantees.
- **Intentional Go bug fix:** arguments are converted once, not once per layer.
  A nested side-effecting tool runs once, not three times. Events, durable records
  and execution reuse the same converted input. New records use logical
  completion order and sequential indices instead of Go's captured-pointer
  record loss and duplicate/non-monotonic indices. Old Go records remain readable
  through the explicit resume API above.
- Retained result metadata models non-mutating reflected structs/slices, not all
  Sobek reflection behavior: JS mutation is not written back into already
  recorded `ToolOutput` values, and omitted fields/nil slices remain governed by
  the Go JSON contract rather than a live reflected Go value. Go-specific pointer
  address formatting in arbitrary nested JS containers is not reproduced.
- Missing arguments for Go handlers that panic on a nil input instead throw a
  catchable TypeError. Null-to-object conversion also throws a JS TypeError.
  Go panic text is not reproduced. Invalid-argument diagnostic details are limited
  to JSON-exportable JavaScript values.
- JavaScript safe-integer boundaries are handled explicitly for ToInteger, but
  JSON/result/UTF-8 and host filesystem/HTTP/backend boundaries still inherit
  the documented `construct-types` and `construct-tools` limitations. Core's
  deterministic sorted error detail order replaces Go's random map order.

## Real Go interpreter goldens

`tests/fixtures/go.json` contains **33 real Go interpreter executions**. Each
stores the exact script, fixed seeded file contents, console output, nullable
interpreter error, Go `encoding/json` bytes for `InterpreterOutput`, and ordered
events with their exact Go JSON payload bytes. The Go runner uses precisely the
daemon tool set (including fetch, excluding report/handoff), runtime interceptor
order, an OS-backed temp directory, and a recording publisher that constructs
events through `event.NewToolCalledEvent`/`NewToolResultEvent`. Go's constants are
`EventTypeToolCalled = "tool.called"` and `EventTypeToolResult = "tool.result"`,
matching the Rust middleware.

Regenerate from the repository root with Go 1.24+ and Python 3:

```sh
python3 rust/scripts/codeact-goldens.py
```

This follows the `rust/scripts/tools-goldens.py` precedent: committed Go generator
source has a `.go.txt` extension, and generated JSON is replayed without Go during
ordinary Rust tests. Unlike the tools generator's temporary in-package helpers,
this generator can use exported APIs: its Python wrapper writes `main.go` only
to a disposable directory **outside backend/**, runs it from the Go workspace,
and removes it. No Go source under `backend/` is changed.

The corpus covers primitive/nested print values, numeric thresholds and negative
zero, full/ranged reads, create/edit, list/find/grep, empty search results, command
success and caught nonzero exit, caught/uncaught tool errors, uncaught TypeError
and ReferenceError, strict mode, empty scripts, durable indexes, attempt counts,
events, and struct identity versus plain-JS object copies. Nested cases cover
coercions, argument getters, print-export getters, caught/uncaught nested errors,
and nested calls before and after a durable state has been installed. Go's
three conversions, lost calls and non-monotonic/duplicate indices are preserved
in that fixture as historical evidence, **not** the desired Rust behavior.
`tests/fixtures/once.json` explicitly marks eight known divergences and stores
Rust's expected console output, full serialized calls/stats and ordered events:
`nested_coercion_fresh`, `nested_coercion_existing`, `nested_getter`,
`nested_coercion_print`, `nested_caught_error`, `nested_late_fresh`,
`nested_late_existing`, and `nested_side_effect`. The side-effect probe appends
to a real file inside coercion: Go writes `xxx`, while Rust must write exactly
`x`. All other Go cases are still compared directly. Tests also feed the actual
Go duplicate histories into resume with hosts that reject any execution.
Print formats all arguments before appending its line, so nested getter output
appears before the outer line.
Print exports objects rather than calling their `toString`; a non-enumerable
`toString` probe is asserted not to run. Custom bindings and interceptors receive
`&RefCell<Session>` and must not hold a borrow across JS or `next` calls.

Fetch cases use identical committed JSON/HTML/error response bodies served by
Go's `httptest` and a Rust loopback-only HTTP server. No external network request
is made. They compare full result structs, errors, durable calls, stats and events.
The Rust corpus also replays in a child process with ripgrep hidden from PATH.

`tests/go_goldens.rs` compares console/error strings and **serialized output and
payload bytes**, not merely decoded JSON equality. Normalization is limited to:

1. Temp directory paths replaced by `{{ROOT}}` on both sides.
2. Sorting only contiguous `Details:` lines (Go randomizes error detail map
   iteration); the generator does this too for repeatable fixture output.
3. Removing the numeric Sobek `<eval>` location/program-counter suffix from
   interpreter errors. Error type and message remain exact assertions.
4. Event timestamps are excluded from the recorded projection; event type,
   action, task ID, tool, input/output payload JSON, and ordering are all compared.
5. The ephemeral loopback HTTP port is replaced by `{{PORT}}`; scheme, host,
   paths, response bodies and all other HTTP/result bytes are left unchanged.

The normalization test proves suggestions/ordinary lines are not reordered,
different error messages still differ, and non-console JSON bytes are untouched.
The eight once-per-call divergences above are asserted without normalizing away
the differences; all remaining corpus cases match Go. The Rust grep
fallback retains newlines to match Go's ripgrep-generated golden even without
ripgrep; Go's own grep fallback strips those newlines. Other external
network/backend behavior remains subject to the host-tool compatibility limits.

## Checks

From `rust/`:

```sh
cargo fmt --all --check
cargo +1.89.0 clippy --workspace --all-targets --all-features --locked -- -D warnings
cargo +1.89.0 test -p construct-codeact --all-features --locked
```
