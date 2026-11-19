# Authentication and endpoint parity (Linux)

Run `bash rust/scripts/auth-parity.sh` from any directory. Prerequisites: Go,
Rust 1.89.0, Python 3, `dbus-run-session`, and `gnome-keyring-daemon`.
There are no API keys, model requests, or external services. The wrapper builds
both CLIs and runs the existing mixed-implementation integration tests with a
private, unlocked Secret Service. `RUSTUP_TOOLCHAIN` can override the default.

The differential Python oracle launches both real daemons in isolated XDG
directories and compares HTTP statuses, complete RPC error objects, context
diagnostics/exit codes, JSON display bytes and context YAML bytes. It independently
asserts token encoding, SHA-256 database contents, list filtering/order, and 0600
context permissions. Artifacts (including daemon logs and redacted transcripts)
remain in `rust/target/auth-parity/run-*`. The isolated databases contain synthetic
credentials, not user credentials.

## Coverage

* Tokens: create/list/revoke, duplicates, missing names, invalid/missing UUIDs,
  excessive/negative expiry, active/expired list filtering, `ct_` + 32 random bytes
  encoded as unpadded base64url, SHA-256 of the entire plaintext token.
* Setup codes: alphabet/format, case-insensitive exchange, expiry, single use,
  missing/invalid codes, maximum lifetimes, duplicate token names (a failed
  duplicate-name exchange also consumes the code).
* Transports: Unix trusts local clients even with malformed Authorization; TCP
  rejects missing/malformed/unknown/expired/revoked credentials; valid tokens can
  list agents but cannot create/list/revoke tokens or create setup codes. Setup
  exchange is unauthenticated. Existing mixed tests cover both CLI directions,
  setup enrollment through an HTTP-to-Unix relay, and FD-3 Unix/TCP activation.
* Contexts: empty/current/previous, add/use/list/remove, missing names, invalid
  kind, JSON output, YAML serialization, cross-reading config and context files,
  and file permissions.
* Secrets: existing tests verify file-backed provider encryption/keyset formats,
  Go reading Rust-written provider secrets, Secret Service token/key cross-reading
  and overwrite/delete, `service=construct` / `username=<key>` attributes, and
  keyring-backed CLI enrollment/removal. The Linux implementations select the
  login collection; tests use a private GNOME login collection. macOS/Windows
  keyrings and non-default collection configurations are not exercised here.

## Differences deliberately retained

* Go's Cobra prints a usage trailer after runtime errors; Rust's clap CLI prints
  just the diagnostic. The oracle explicitly checks and strips only Go's usage
  trailer; the error line and exit code must match. Help layout is not normalized
  into apparent byte parity.
* Go advertises `context add --setup-code` but does not implement it. Rust keeps
  its working enrollment flag. Mixed tests exchange via the API for Go clients.
* Go's direct `--listen-http` startup fails because its activation kind is `tcp`
  while contexts allow `http`. The oracle uses real socket-activated TCP for both
  daemons, not a relay that would accidentally confer Unix administrator trust.
  Rust retains working direct HTTP startup (covered by the server mixed test).
* Go wraps authentication errors in another unauthenticated error. Rust now
  preserves the resulting `unauthenticated: ...` wire message, including for
  expired/revoked tokens. It also mirrors Go's outer unauthenticated code if the
  authentication store fails; storage backend error wording is backend-specific.
* Random IDs/tokens/timestamps are asserted structurally or via database hashes,
  not diffed as if deterministic. Context output and persisted YAML are diffed
  without such normalization. CLI output streams are combined because Go writes
  some successful output to stderr while Rust writes stdout.
