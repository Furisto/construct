# Go encoding/json goldens

Generated with Go 1.24.1 from the real Go types, not hand-authored JSON.
Generator source: [`rust/scripts/types-golden.go.txt`](../../../../scripts/types-golden.go.txt).
The `.txt` extension deliberately excludes it from Go builds, tests, and gofmt.
Only generated fixtures and the non-built generator source are committed; the
throwaway module and executable Go source live outside the repository.

Reproduce from the repository root (adjust absolute paths in the module):

```sh
root="$PWD"
mkdir -p "$HOME/golden-gen"
cd "$HOME/golden-gen"
# Create go.mod with the following contents, replacing ROOT with $root:
# module construct-types-golden
# go 1.24.1
# require github.com/furisto/construct/backend v0.0.0
# replace github.com/furisto/construct/backend => ROOT/backend
# replace github.com/furisto/construct/shared => ROOT/shared
# replace github.com/furisto/construct/api/go => ROOT/api/go
cp "$root/rust/scripts/types-golden.go.txt" main.go
GOWORK=off go mod tidy
GOWORK=off go run . "$root/rust/crates/construct-types/tests/fixtures"
```

Files contain exactly `json.Marshal` bytes, with no final newline. The four
module bundles cover every serializable struct: `zero` retains nil collections,
`empty` allocates empty collections (without allocating nil pointers), `full`
populates fields, and `special` includes HTML, line/paragraph separators, quotes,
backslashes, controls, and Unicode. Reflection fill depth is bounded at five to
terminate recursive interpreter calls. Rust tests construct the same values
independently via typed struct literals, then compare both their serialization
and a deserialize/reserialize cycle against the original bytes.

Additional fixtures cover Go float-format boundaries/subnormals/negative zero,
RFC3339Nano timestamps including Go's year-one zero, all provider content-block
shapes, raw JSON ordering/compaction, and public provider-error retry behavior.

## Representation decisions

- Use `construct_types::to_go_json`, not ordinary serde_json serialization, for
  Go-compatible float spelling and default HTML/U+2028/U+2029 escaping.
- Field declaration order matches Go; `BTreeMap` sorts keys. `RawValue` preserves
  raw object ordering and number spellings; the helper compacts whitespace and
  escapes HTML just as Go's `json.Marshal(json.RawMessage)` does.
- `Option<Vec<_>>` and `Option<BTreeMap<_, _>>` retain nil (`null`) versus allocated
  empty (`[]`/`{}`). Omitempty collections omit both. Optional pointers use
  `Option<Box<_>>`: a present pointer to zero is not omitted. Go does **not** omit
  zero-valued struct fields, notably `EditFileResult.patch_info`.
- Open string newtypes retain Go's zero and unknown enum values. Associated
  string constants enumerate known values without restricting forward reads.
- Decimal is an arbitrary-precision **canonical string** newtype with zero `"0"`;
  it deliberately provides no arithmetic or input normalization. Callers must
  use shopspring/decimal canonical spelling when constructing it.
- `Timestamp` wraps chrono UTC, defaults to year one, and trims fractional zeros.
  Domain timestamps are UTC; non-UTC input offsets normalize to UTC. Go strings
  with invalid UTF-8 cannot be represented by a Rust `String`.
- Entity records contain scalar columns and Time mixin fields, not Ent edges.
  They intentionally do not implement serde (nor expose secrets through JSON).
  JSON columns are `Task.tool_uses`, `Message.content`, `Message.usage`, and
  `Model.capabilities`; their component types are covered by these goldens.
  Nullable foreign keys/JSON/times use `Option`; optional scalar columns use the
  same zero-value representation as Ent. Defaults are not database insert defaults.
- ProviderError is Rust-only because Go's arbitrary `error` has no fixed JSON
  schema. Its public retry policy and formatted messages are checked against Go,
  including the distinction between public Retryable and private retryableInternal.
- Events and subscription options are internal Rust shapes, not serialized.
  Pattern tests port both Go matchPattern tables, including internal isolation.
