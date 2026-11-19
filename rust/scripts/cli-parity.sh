#!/usr/bin/env bash
set -euo pipefail
repo="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$repo"
CGO_ENABLED=0 go build -o rust/target/construct-go ./frontend/cli
cargo +1.89.0 build --manifest-path rust/Cargo.toml --locked -p construct-cli
exec python3 rust/scripts/cli-parity.py "$@"
