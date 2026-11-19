#!/usr/bin/env bash
# Go vs Rust skills parity (see skills-parity.py). Linux prerequisites: Go, Rust,
# Python 3, git, dbus-run-session, gnome-keyring-daemon. All traffic is loopback.
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd -- "$script_dir/../.." && pwd)"
if [[ ${1:-} != --inner ]]; then
  (cd "$repo" && CGO_ENABLED=0 go build -o /tmp/construct-go ./frontend/cli)
  cargo build --manifest-path "$repo/rust/Cargo.toml" --locked -p construct-cli
  output="${SKILLS_PARITY_ROOT:-$HOME/skills-parity}"
  mkdir -p "$output"
  run="$(mktemp -d "$(cd "$output" && pwd)/run-XXXXXXXX")"
  exec dbus-run-session -- bash "$0" --inner "$run"
fi
run="$2"
mkdir -p "$run/home"
HOME="$run/home" gnome-keyring-daemon --unlock --components=secrets <<< 'parity-only' > "$run/keyring.log" 2>&1
python3 "$script_dir/skills-parity.py" --go /tmp/construct-go --rust "$repo/rust/target/debug/construct" --run "$run"
