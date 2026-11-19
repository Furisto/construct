#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/../.." && pwd)"
export CONSTRUCT_GO_BIN="$repo_root/rust/target/go-interop/construct"
export CONSTRUCT_GO_STORE_FIXTURE="$repo_root/rust/target/go-interop/store-fixture"

cd "$repo_root"
go version
cargo --version
mkdir -p "$(dirname -- "$CONSTRUCT_GO_BIN")"
printf 'Building real Go daemon: %s\n' "$CONSTRUCT_GO_BIN"
CGO_ENABLED=0 go build -o "$CONSTRUCT_GO_BIN" ./frontend/cli
CGO_ENABLED=0 go build -o "$CONSTRUCT_GO_STORE_FIXTURE" ./rust/crates/construct-store/tests/gofixture/main.go

bash "$script_dir/secret-interop.sh"

# Each test writes secret.provider=file into an isolated XDG config directory.
# No Secret Service, D-Bus session, credentials, or external model calls required.
cargo test --manifest-path "$repo_root/rust/Cargo.toml" \
  -p construct-proto --test go_interop -- --nocapture --test-threads=1
cargo test --manifest-path "$repo_root/rust/Cargo.toml" \
  -p construct-store --test go_interop -- --nocapture --test-threads=1
