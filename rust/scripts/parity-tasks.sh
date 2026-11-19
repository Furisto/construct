#!/usr/bin/env bash
# Go vs Rust task/message parity. Linux prerequisites: Go, Rust, Python 3, tmux,
# dbus-run-session, gnome-keyring-daemon. Model traffic is loopback; credentials are fake.
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd -- "$script_dir/../.." && pwd)"
out="${PARITY_ROOT:-$HOME/parity}"
mkdir -p "$out"
if [[ ${1:-} != --inner ]]; then
  (cd "$repo" && CGO_ENABLED=0 go build -o "$out/construct-go" ./frontend/cli)
  cargo build --manifest-path "$repo/rust/Cargo.toml" --locked -p construct-cli
  exec dbus-run-session -- bash "$script_dir/parity-tasks.sh" --inner "$@"
fi
shift
keyring_home="$(mktemp -d)"
HOME="$keyring_home" gnome-keyring-daemon --unlock --components=secrets <<< 'parity-only-password' > "$out/keyring.log" 2>&1
python3 "$script_dir/parity-tasks.py" --go-bin "$out/construct-go" --rust-bin "$repo/rust/target/debug/construct" --out "$out" "$@"
