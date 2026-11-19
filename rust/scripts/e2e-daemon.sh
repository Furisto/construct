#!/usr/bin/env bash
# Prerequisites: Go, Rust, Python 3, curl, tmux, ripgrep; on Linux also
# dbus-run-session and gnome-keyring-daemon. macOS uses the login Keychain.
# All model traffic is loopback; all credentials are fake.
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd -- "$script_dir/../.." && pwd)"

if [[ ${1:-} != --inner ]]; then
  output="${E2E_ROOT:-$HOME/e2e}"
  mkdir -p "$output"
  output="$(cd "$output" && pwd)"
  (
    cd "$repo"
    CGO_ENABLED=0 go build -o /tmp/construct-go ./frontend/cli
    cargo build --manifest-path rust/Cargo.toml --locked -p construct-cli
    run="$(mktemp -d "$output/run-XXXXXXXX")"
    if [[ $(uname) == Darwin ]]; then
      bash "$script_dir/e2e-daemon.sh" --inner "$output" "$run"
    else
      dbus-run-session -- bash "$script_dir/e2e-daemon.sh" --inner "$output" "$run"
    fi
  ) 2>&1 | tee "$output/e2e.log"
  exit "${PIPESTATUS[0]}"
fi

output="$2"
run="$3"
echo "Artifacts: $run"
# The macOS Keychain belongs to the real HOME, so only XDG paths are isolated there.
[[ $(uname) == Darwin ]] || export HOME="$run/home"
export XDG_CONFIG_HOME="$run/config" XDG_DATA_HOME="$run/data"
export XDG_STATE_HOME="$run/state" XDG_RUNTIME_DIR="$run/runtime" TERM=xterm-256color
mkdir -p "$run/home" "$XDG_CONFIG_HOME/construct" "$XDG_RUNTIME_DIR" "$run/workspace"
chmod 700 "$run/home" "$XDG_RUNTIME_DIR"
printf 'secret:\n  provider: file\n' > "$XDG_CONFIG_HOME/construct/config.yaml"
# Enrollment tokens use the CLI keyring even when daemon encryption uses files.
# Start a private Secret Service inside this run's D-Bus session and isolated HOME.
if [[ $(uname) != Darwin ]]; then
  printf 'e2e-only-password' | gnome-keyring-daemon --unlock --components=secrets > "$run/keyring.log" 2>&1
fi
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1; }
GO=/tmp/construct-go
RUST="$repo/rust/target/debug/construct"
socket="$run/daemon.sock"
tmux_name="construct-e2e-$$"
daemon_pid="" mock_pid=""
cleanup() {
  tmux -L "$tmux_name" kill-server 2>/dev/null || true
  if [[ -n $daemon_pid ]]; then kill -TERM "$daemon_pid" 2>/dev/null || true; wait "$daemon_pid" || true; fi
  if [[ -n $mock_pid ]]; then kill "$mock_pid" 2>/dev/null || true; wait "$mock_pid" 2>/dev/null || true; fi
}
trap cleanup EXIT
python3 "$script_dir/e2e-mock.py" --workspace "$run/workspace" --port-file "$run/mock.port" --requests "$run/mock-requests.jsonl" > "$run/mock.log" 2>&1 &
mock_pid=$!
for _ in {1..100}; do [[ -s $run/mock.port ]] && break; sleep .1; done
export ANTHROPIC_BASE_URL="http://127.0.0.1:$(< "$run/mock.port")/"
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')"
wait_daemon() {
  for _ in {1..150}; do
    if curl --silent --fail --unix-socket "$socket" http://localhost/healthz >/dev/null; then return; fi
    kill -0 "$daemon_pid" || { echo 'Daemon exited during startup'; return 1; }
    sleep .1
  done
  echo 'Daemon readiness timed out'; return 1
}
stop_daemon() {
  kill -TERM "$daemon_pid"
  wait "$daemon_pid"
  daemon_pid=""
}
wait_reply() {
  local binary="$1" task="$2" marker="$3" context="${4:-local}" destination="$run/$3-messages.json"
  for _ in {1..150}; do
    "$binary" --context "$context" message list --task "$task" --output json --wide > "$destination"
    if python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(0 if "MOCK_REPLY " + sys.argv[2] + " tool-ok" in json.dumps(d) else 1)' "$destination" "$marker"; then
      test "$(< "$run/workspace/proof-$marker.txt")" = "CODEACT_OK $marker"
      cat "$destination"
      echo "PASS: $marker reply persisted; CodeAct wrote proof-$marker.txt"
      return
    fi
    sleep .2
  done
  echo "FAIL: reply timed out for $marker; inspect $run daemon logs"; return 1
}

echo '=== Go phase: create existing database using Go CLI ==='
"$GO" daemon run --listen-unix "$socket" > "$run/go-daemon.log" 2>&1 &
daemon_pid=$!
wait_daemon
"$GO" context add local --endpoint "$socket"
"$GO" context use local
provider="$("$GO" modelprovider create mock-anthropic --type anthropic --api-key e2e-fake-key 2>&1)"
# The Go CLI has no --url flag, and Go's factory ignores the persisted URL.
# Go's SDK supports ANTHROPIC_BASE_URL. Seed the same URL in its record offline so
# the Rust factory must honor the stored setting, without depending on that env.
stop_daemon
python3 - "$XDG_DATA_HOME/construct/construct.db" "$provider" "$ANTHROPIC_BASE_URL" <<'PY'
import sqlite3, sys
with sqlite3.connect(sys.argv[1]) as db:
    changed = db.execute('UPDATE model_providers SET url = ? WHERE id = ?', (sys.argv[3].rstrip('/'), sys.argv[2])).rowcount
    assert changed == 1, changed
PY
"$GO" daemon run --listen-unix "$socket" >> "$run/go-daemon.log" 2>&1 &
daemon_pid=$!
wait_daemon
model="$("$GO" model create e2e-model --provider "$provider" --context-window 128000 2>&1)"
agent="$("$GO" agent create e2e-agent --model "$model" --prompt 'Use the provided tools and answer the user.' 2>&1)"
"$GO" agent list --output json > "$run/go-agent-list.json"
python3 - "$run/go-agent-list.json" "$agent" <<'PY'
import json, sys
agents = json.load(open(sys.argv[1]))
assert any(a['id'] == sys.argv[2] and a['name'] == 'e2e-agent' for a in agents)
print('PASS: Go CLI created/listed e2e-agent')
PY
go_task="$("$GO" task create --agent "$agent" --workspace "$run/workspace" 2>&1)"
"$GO" message create "$go_task" GO_INITIAL
wait_reply "$GO" "$go_task" GO_INITIAL
stop_daemon
key_before="$(sha256 "$XDG_DATA_HOME/construct/secrets/encryption_key")"

echo '=== Rust phase: read Go data, enroll TCP, execute new and old tasks ==='
unset ANTHROPIC_BASE_URL
"$RUST" daemon run --listen-unix "$socket" --listen-http "127.0.0.1:$port" > "$run/rust-daemon.log" 2>&1 &
daemon_pid=$!
wait_daemon
curl --silent --show-error --fail --max-time 5 "http://127.0.0.1:$port/healthz" > "$run/tcp-health.txt"
echo 'PASS: Rust Unix and TCP health endpoints are available'
for resource in modelprovider model agent task; do
  "$RUST" --context local "$resource" list --output json > "$run/rust-$resource-list.json"
  python3 - "$run/rust-$resource-list.json" <<'PY'
import json, sys
for item in json.load(open(sys.argv[1])):
    print({k: v for k, v in item.items() if k in ('id', 'name', 'title', 'agent', 'model')})
PY
done
"$RUST" --context local message list --task "$go_task" --output json --wide | tee "$run/rust-go-messages.json"
python3 - "$run" "$agent" "$go_task" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
for file, value in [('rust-modelprovider-list.json','mock-anthropic'), ('rust-model-list.json','e2e-model'), ('rust-agent-list.json',sys.argv[2]), ('rust-task-list.json',sys.argv[3]), ('rust-go-messages.json','MOCK_REPLY GO_INITIAL')]:
    assert value in json.dumps(json.loads((root/file).read_text())), file
print('PASS: Rust CLI reads Go providers/models/agents/tasks/messages')
PY
setup="$("$RUST" --context local daemon token create-setup e2e-tcp --output json | python3 -c 'import json,sys; print(json.load(sys.stdin)["setup_code"])')"
"$RUST" context add tcp-e2e --endpoint "http://127.0.0.1:$port" --setup-code "$setup"
unset setup
rust_task="$("$RUST" --context tcp-e2e task create --agent "$agent" --workspace "$run/workspace" 2>&1)"
"$RUST" --context tcp-e2e message create "$rust_task" RUST_TCP
wait_reply "$RUST" "$rust_task" RUST_TCP tcp-e2e
"$RUST" --context tcp-e2e message create "$go_task" GO_RESUMED
wait_reply "$RUST" "$go_task" GO_RESUMED tcp-e2e
test "$key_before" = "$(sha256 "$XDG_DATA_HOME/construct/secrets/encryption_key")"
echo 'PASS: Rust reused Go encryption key and decrypted Go provider credentials'

echo '=== TUI: resume Go-created task and send a new message ==='
printf -v tui_command '%q --context local resume %q' "$RUST" "$go_task"
tmux -L "$tmux_name" new-session -d -x 140 -y 48 -s tui "$tui_command"
for _ in {1..100}; do
  tmux -L "$tmux_name" capture-pane -p -t tui > "$output/tui-before.txt"
  if rg -q 'Enter send' "$output/tui-before.txt"; then break; fi
  sleep .1
done
tmux -L "$tmux_name" send-keys -t tui -l TUI_MESSAGE
tmux -L "$tmux_name" send-keys -t tui Enter
wait_reply "$RUST" "$go_task" TUI_MESSAGE
for _ in {1..100}; do
  tmux -L "$tmux_name" capture-pane -p -t tui > "$output/tui-reply.txt"
  if rg -q 'MOCK_REPLY TUI_MESSAGE tool-ok' "$output/tui-reply.txt"; then break; fi
  sleep .1
done
rg 'MOCK_REPLY TUI_MESSAGE tool-ok' "$output/tui-reply.txt"
cp "$output"/tui-{before,reply}.txt "$run/"
tmux -L "$tmux_name" send-keys -t tui C-c
tmux -L "$tmux_name" send-keys -t tui C-c
tmux -L "$tmux_name" kill-server 2>/dev/null || true
echo 'PASS: TUI pane shows the persisted mock reply'
stop_daemon

echo '=== Go read-back: reopen Rust-written DB ==='
"$GO" daemon run --listen-unix "$socket" > "$run/go-readback.log" 2>&1 &
daemon_pid=$!
wait_daemon
"$GO" --context local task list --limit 100 --output json | tee "$run/go-readback-tasks.json"
python3 - "$run/go-readback-tasks.json" "$go_task" "$rust_task" <<'PY'
import json, sys
ids = {task['id'] for task in json.load(open(sys.argv[1]))}
assert set(sys.argv[2:]) <= ids, ids
print('PASS: Go lists both Go-created and Rust-created tasks')
PY
wait_reply "$GO" "$rust_task" RUST_TCP
wait_reply "$GO" "$go_task" TUI_MESSAGE
stop_daemon
echo "PASS: Go reads Rust-written tasks/messages; artifacts in $run"
echo 'E2E PASS: Go -> Rust (Unix + TCP + CodeAct + TUI) -> Go'
