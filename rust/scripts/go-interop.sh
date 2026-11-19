#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/../.." && pwd)"
export CONSTRUCT_GO_BIN="$repo_root/rust/target/go-interop/construct"

cd "$repo_root"
go version
cargo --version
mkdir -p "$(dirname -- "$CONSTRUCT_GO_BIN")"
printf 'Building real Go daemon: %s\n' "$CONSTRUCT_GO_BIN"
CGO_ENABLED=0 go build -o "$CONSTRUCT_GO_BIN" ./frontend/cli

# Each test writes secret.provider=file into an isolated XDG config directory.
# No Secret Service, D-Bus session, credentials, or external model calls required.
cargo test --manifest-path "$repo_root/rust/Cargo.toml" \
  -p construct-proto --test go_interop -- --nocapture --test-threads=1
