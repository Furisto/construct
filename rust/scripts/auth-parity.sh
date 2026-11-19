#!/usr/bin/env bash
set -euo pipefail
repo="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo"
export RUSTUP_TOOLCHAIN="${RUSTUP_TOOLCHAIN:-1.89.0}"
export RUSTUP_HOME="${RUSTUP_HOME:-$HOME/.rustup}" CARGO_HOME="${CARGO_HOME:-$HOME/.cargo}"
export GOPATH="$(go env GOPATH)" GOCACHE="$(go env GOCACHE)"
mkdir -p rust/target/auth-parity
export CONSTRUCT_GO_BIN="${CONSTRUCT_GO_BIN:-$repo/rust/target/auth-parity/construct-go}"
CGO_ENABLED=0 go build -o "$CONSTRUCT_GO_BIN" ./frontend/cli
cargo build --manifest-path rust/Cargo.toml --locked -p construct-cli
python3 rust/scripts/auth-parity.py "$CONSTRUCT_GO_BIN" "$repo/rust/target/debug/construct"
# Existing mixed CLI tests also byte-compare context/config YAML and renderers.
# A private, unlocked Secret Service is required for live cross-reading tests.
dbus-run-session -- bash -c '
  set -euo pipefail
  export HOME="$(mktemp -d "$PWD/rust/target/auth-parity/keyring-XXXXXXXX")"
  export XDG_CONFIG_HOME="$HOME/config" XDG_DATA_HOME="$HOME/data"
  mkdir -p "$XDG_CONFIG_HOME" "$XDG_DATA_HOME"
  printf auth-parity-only | gnome-keyring-daemon --unlock --components=secrets
  export CONSTRUCT_KEYRING_INTEROP=1 CONSTRUCT_TEST_KEYRING=1
  bash rust/scripts/secret-interop.sh
  cargo test --manifest-path rust/Cargo.toml --locked -p construct-cli --test go_interop -- --nocapture --test-threads=1
  cargo test --manifest-path rust/Cargo.toml --locked -p construct-server --test go_interop -- --nocapture --test-threads=1
'
echo 'PASS auth parity: RPC/error diffs, contexts, both mixed CLI/daemon directions, file secrets and live Secret Service'
