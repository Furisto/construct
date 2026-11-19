#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
output="$repo_root/rust/target/secret-interop.json"
cargo run --manifest-path rust/Cargo.toml -p construct-secret --example interop -- "$output"
go run ./backend/secret/testdata/interop verify "$output"
if [[ "${CONSTRUCT_TEST_KEYRING:-0}" == 1 ]]; then
  key="construct-interop-test-$$"
  go run ./backend/secret/testdata/interop keyring set "$key"
  cargo run --manifest-path rust/Cargo.toml -p construct-secret --example interop -- keyring "$key"
  go run ./backend/secret/testdata/interop keyring get "$key"
  echo 'Go↔Rust live keyring interoperability passed'
fi
