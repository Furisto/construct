# construct-tools

Host implementations of Construct's Go tool core, implementing
`construct_core::tool::Tool` with the existing `construct_types::tools` inputs
and outputs. This crate does not embed CodeAct's JavaScript VM, native tools, a
daemon, or a concrete database driver. It supports the workspace's Rust 1.89 MSRV.

## Tools

| Module | Tools |
| --- | --- |
| `filesystem` | `CreateFile`, `EditFile`, `ReadFile`, `ListFiles`, `FindFile`, `Grep` |
| `system` | `ExecuteCommand` |
| `web` | `Fetch` |
| `communication` | `AskUser`, `SubmitReport`, `Handoff` |

Descriptions are the rendered tool-level text from `backend/tool/codeact/`
(including its documented examples); `ask_user` uses that file's comment text.
Schemas reflect the corresponding Go input records, including PascalCase fields
where Go has no JSON tags. Descriptions sometimes describe CodeAct's positional
JavaScript APIs; the Rust `execute` entry point always accepts a JSON object.
Serialize results with `construct_types::to_go_json`, not plain serde JSON.

Filesystem tools operate on the host filesystem. Absolute paths are not confined
to `ToolContext.workspace_root`, just as in Go. Read-file preserves Go's Scanner
line limit, range markers, CRLF stripping, and line counts. Edit-file validates
all edits before writing, applies the first match sequentially, supports trimmed
line matching, and returns conflict warnings and unified patches. Invalid source
bytes are preserved when editing; string results replace invalid UTF-8 bytes with
U+FFFD individually. Find and grep use executable `rg` from PATH when available;
their fallbacks are glob traversal and system `grep`, respectively. Consequently,
the two search backends have Go's different ignore/hidden-file behavior.
The grep fallback retains the emitted line endings, unlike Go's fallback parser
which strips them, so match/context values agree with Go's ripgrep output even
when ripgrep is absent.

Execute-command uses `/bin/sh -c` with Go's `set -eu` wrapper. Stdout and stderr
share one file descriptor and are returned as combined `stdout` with empty
`stderr`. Nonzero exits are tool errors, not successful results. Cancellation
kills and reaps the shell child (not an entire descendant process group).

Fetch accepts HTTP(S), defaults to 30 seconds, caps the original body at 5 MiB,
supports JSON and HTML/XHTML, and converts extracted HTML to cleaned Markdown.
`Fetch::new` permits an injected reqwest client for transport policy; the default
does not require external services. HTTP tests run only against loopback servers.

Handoff takes `Arc<dyn Store>` at construction. It resolves agents, updates the
task, and creates the assistant handover message inside one transaction. Failures
roll back both writes. Missing task/current-agent IDs fall back to ToolContext,
matching CodeAct's session injection. Timestamps/UUIDs are assigned by the caller
as required by Store. **Successful handoff returns `ToolOutput::default()` (`{}`)**:
the shared output contract has no handoff arm, and Go has no success payload.

**Ask-user intentionally retains Go's stub behavior:** it returns
`ToolOutput::default()` (`{}`), even for empty input, without validation, user I/O,
or invented answers. The Go CodeAct file has documentation but no interactive
executor. Applications needing interaction must provide that separately.

## Dependencies

Workspace dependencies (`serde`, `serde_json`, `tokio`, `futures-util`) supply JSON,
async execution, cancellation coordination, process/HTTP streaming, and blocking
filesystem dispatch. Existing core/types contracts remain the source of truth.

| Additional crate | Rationale |
| --- | --- |
| `uuid` | Existing workspace convention for Store-assigned message IDs |
| `similar` | Unified line diffs without shelling out; normalizes Go's hunk ranges |
| `globset` | Compiled `**`/brace globs for the no-ripgrep find backend |
| `reqwest` (Rustls, stream) | Injectable async HTTP(S), redirects, bounded body reads |
| `readability` | Rust article extraction without an external browser or Go runtime |
| `html2md` | HTML-to-Markdown with Go-compatible heading handling |
| `regex` | Case-insensitive HTML heading normalization before article extraction |
| `tempfile` | One shared combined-output file descriptor; isolated test files |

## Compatibility boundaries

The committed Go goldens are regression guarantees, not a claim that independent
HTML extractors, glob implementations, or diff algorithms agree on every input.
In particular:

- Rust readability/Markdown extraction can differ from Go's libraries on complex
  pages, and diff alignment can differ on ambiguous repeated lines. Golden cases
  pin the upstream test corpus and simple HTML/XHTML responses.
- `globset` covers common Go doublestar patterns, but is not a complete port of
  doublestar syntax or symlink semantics. Filesystem traversal errors follow the
  Go fallback's best-effort behavior.
- Parent directories use host directory creation permissions (normally 0755),
  rather than reproducing Go create-file's 0644 directory mode, which makes newly
  created directories unsearchable on a real non-root filesystem. New regular
  files use 0644, subject to umask, and existing modes are preserved.
- Rust strings cannot retain invalid UTF-8. Go's JSON encoder emits `\ufffd` for
  invalid bytes but a literal U+FFFD for genuine replacement characters; the
  domain contract cannot preserve that distinction. Golden tests normalize only
  this representation difference; replacement counts and file bytes are checked.
- Underlying OS, HTTP, URL-parser and Store diagnostics can differ by backend.
  Standard tool errors and Go-tested validation text/details are preserved;
  malformed JSON is reported through ToolError rather than Go's typed-call API.
- Store determines transaction isolation, conflicts, and commit atomicity. There
  is no Ent graph; model references come from the source agent record.

## Parity tests and fixture generation

From `rust/`:

```sh
cargo test -p construct-tools --all-features
cargo fmt --check
cargo clippy --workspace --all-features --all-targets -- -D warnings
cargo test --workspace --all-features
```

`tests/go_goldens.rs` replays all create/read/list/edit/command Go scenarios,
including previously skipped edit cases, against isolated real files. It checks
tool messages, suggestions, details, before/after file bytes, and Go-compatible
JSON. `src/filesystem/search.rs` tests all 13 upstream ripgrep snapshots, fallback
find goldens, and fallback grep grouping. Edit's unit tests cover the 13 upstream
escape-normalization cases (the helper is not used by Go edit). `tests/behavior.rs`
covers Store handoffs and rollback, reports/stub asks, command cancellation,
read ranges/limits, invalid UTF-8 edits, real searches, and hermetic fetches.
CodeAct's interpreter test currently contains only commented-out scenarios; the
JavaScript VM itself is deliberately out of scope.

Regenerate fixtures from the repository root with Go 1.24+ and Python 3:

```sh
python3 rust/scripts/tools-goldens.py
```

The script temporarily adds Go test helpers, records actual tool outputs, removes
the helpers in `finally`, and deduplicates file bytes by SHA-256 under
`tests/fixtures/blobs/`. Run it on a clean worktree; it does not require credentials,
the daemon, or external HTTP services. Input strings round-trip through JSON
because Rust Tool accepts JSON, not arbitrary Go byte strings. Committed fixtures
allow ordinary Rust tests to run without Go installed.
