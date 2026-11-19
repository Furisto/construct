#!/usr/bin/env bash
# Artifacts survive failures; all daemons, credentials and model traffic are isolated.
# Requires Go, Rust 1.89.0, curl, Python 3 and PyYAML. No D-Bus/keyring needed.
set -euo pipefail
repo="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
root="${PARITY_ROOT:-$HOME/construct-parity}"
mkdir -p "$root"
cd "$repo"
python3 -c 'import yaml'
CGO_ENABLED=0 go build -o "$root/construct-go" ./frontend/cli
cargo +1.89.0 build --manifest-path rust/Cargo.toml --locked -p construct-cli
exec python3 "$repo/rust/scripts/parity-agents.py" --go "$root/construct-go" --rust "$repo/rust/target/debug/construct" --output "$root"
