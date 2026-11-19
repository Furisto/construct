#!/usr/bin/env python3
"""Go vs Rust parity suite for tasks and messages (exec, resume, message list with tool calls).

Runs the same scenario once per (CLI, daemon) combination against a deterministic
mock Anthropic endpoint, normalizes ids/timestamps/paths, and compares every capture
against the all-Go baseline. Run through parity-tasks.sh (D-Bus + keyring + builds).
"""
import argparse
import difflib
import http.client
import json
import os
import re
import socket
import sqlite3
import struct
import subprocess
import sys
import threading
import time
import traceback
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
COMBOS = [("go", "go"), ("rust", "rust"), ("go", "rust"), ("rust", "go")]
MODEL_NAME = "claude-sonnet-4-20250514"


class UnixConnection(http.client.HTTPConnection):
    def __init__(self, path, timeout=30):
        super().__init__("localhost", timeout=timeout)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


class Api:
    def __init__(self, socket_path):
        self.socket_path = socket_path

    def call(self, service, method, body):
        conn = UnixConnection(self.socket_path)
        conn.request("POST", f"/api/construct.v1.{service}/{method}", json.dumps(body), {"Content-Type": "application/json"})
        resp = conn.getresponse()
        data = resp.read()
        conn.close()
        return resp.status, json.loads(data or b"{}")

    def ok(self, service, method, body):
        status, data = self.call(service, method, body)
        if status != 200:
            raise RuntimeError(f"{service}/{method}: {status} {data}")
        return data

    def subscribe(self, body, events, stop):
        conn = UnixConnection(self.socket_path, timeout=120)
        payload = json.dumps(body).encode()
        conn.request("POST", "/api/construct.v1.EventService/Subscribe", struct.pack(">BI", 0, len(payload)) + payload,
                     {"Content-Type": "application/connect+json", "Connect-Protocol-Version": "1"})
        resp = conn.getresponse()
        try:
            while not stop.is_set():
                header = resp.read(5)
                if len(header) < 5:
                    break
                flags, size = struct.unpack(">BI", header)
                frame = json.loads(resp.read(size) or b"{}")
                if flags & 2:
                    if frame:
                        events.append({"end": frame})
                    break
                events.append(frame)
        except (OSError, http.client.HTTPException):
            pass
        finally:
            conn.close()


class Side:
    def __init__(self, root, cli, daemon, bins):
        self.cli, self.daemon = cli, daemon
        self.name = f"cli-{cli}_daemon-{daemon}"
        self.run = root / self.name
        self.bins = bins
        self.ws = self.run / "workspace"
        self.socket = str(self.run / "d.sock")
        self.captures = {}
        self.order = []
        self.proc = None
        self.mock = None
        self.tmux = f"parity-{os.getpid()}-{cli}-{daemon}"

    # environment -----------------------------------------------------------
    def env(self):
        env = dict(os.environ)
        env.update(HOME=str(self.run / "home"), XDG_CONFIG_HOME=str(self.run / "config"), XDG_DATA_HOME=str(self.run / "data"),
                   XDG_STATE_HOME=str(self.run / "state"), XDG_RUNTIME_DIR=str(self.run / "runtime"), TERM="xterm-256color",
                   NO_COLOR="1", TZ="UTC", COLUMNS="200", LINES="50")
        if self.mock_url:
            env["ANTHROPIC_BASE_URL"] = self.mock_url
        return env

    def prepare(self):
        for sub in ("home", "config/construct", "data", "state", "runtime", "workspace"):
            (self.run / sub).mkdir(parents=True, exist_ok=True)
        os.chmod(self.run / "runtime", 0o700)
        (self.run / "config/construct/config.yaml").write_text("secret:\n  provider: file\n")
        port_file = self.run / "mock.port"
        self.mock = subprocess.Popen([sys.executable, str(SCRIPT_DIR / "parity-mock.py"), "--workspace", str(self.ws),
                                      "--port-file", str(port_file), "--requests", str(self.run / "mock-requests.jsonl")],
                                     stdout=open(self.run / "mock.log", "w"), stderr=subprocess.STDOUT)
        for _ in range(100):
            if port_file.exists() and port_file.read_text():
                break
            time.sleep(0.1)
        self.mock_url = f"http://127.0.0.1:{port_file.read_text()}"
        self.api = Api(self.socket)

    def start_daemon(self):
        log = open(self.run / f"daemon-{self.daemon}.log", "a")
        self.proc = subprocess.Popen([self.bins[self.daemon], "daemon", "run", "--listen-unix", self.socket],
                                     env=self.env(), stdout=log, stderr=subprocess.STDOUT)
        for _ in range(200):
            try:
                conn = UnixConnection(self.socket, timeout=1)
                conn.request("GET", "/healthz")
                if conn.getresponse().status == 200:
                    return
            except OSError:
                pass
            if self.proc.poll() is not None:
                raise RuntimeError(f"{self.name}: daemon exited during startup")
            time.sleep(0.1)
        raise RuntimeError(f"{self.name}: daemon readiness timed out")

    def stop_daemon(self):
        if self.proc:
            self.proc.terminate()
            try:
                self.proc.wait(20)
            except subprocess.TimeoutExpired:
                self.proc.kill()
            self.proc = None

    def close(self):
        subprocess.run(["tmux", "-L", self.tmux, "kill-server"], capture_output=True)
        self.stop_daemon()
        if self.mock:
            self.mock.kill()

    # capture helpers -------------------------------------------------------
    def construct(self, *args, stdin=None, timeout=90):
        cmd = [self.bins[self.cli], "--context", "local", *args]
        p = subprocess.run(cmd, env=self.env(), cwd=self.ws, capture_output=True, text=True,
                           input=stdin, stdin=None if stdin is not None else subprocess.DEVNULL, timeout=timeout)
        p.out = p.stdout + p.stderr
        with open(self.run / "commands.log", "a") as log:
            log.write(f"$ {' '.join(cmd)}\nexit={p.returncode}\n{p.out}\n")
        return p

    def cap(self, name, *args, stdin=None, stderr=False):
        p = self.construct(*args, stdin=stdin)
        text = f"$ construct {' '.join(args)}\nexit={p.returncode}\n--- stdout\n{p.stdout}--- stderr\n{p.stderr}"
        self.record(name, text)
        return p

    def record_pane(self, name, pane):
        (self.run / f"{name}.raw.txt").write_text(pane)
        self.record(name, pane_summary(pane))

    def record(self, name, text):
        self.captures[name] = text
        self.order.append(name)

    def wait_idle(self, task, marker, title=False):
        deadline = time.time() + 60
        while time.time() < deadline:
            t = self.api.ok("TaskService", "GetTask", {"id": task})["task"]
            msgs = self.api.ok("MessageService", "ListMessages", {"filter": {"taskIds": task}}).get("messages", [])
            done = any(m["metadata"].get("role") == "MESSAGE_ROLE_ASSISTANT" and f"{marker} " in json.dumps(m["spec"]) and
                       not any("toolCall" in p for p in m["spec"].get("content", [])) for m in msgs)
            phase = t.get("status", {}).get("phase")
            if done and phase in (None, "TASK_PHASE_AWAITING") and (not title or t["spec"].get("description")):
                time.sleep(0.3)
                return
            time.sleep(0.2)
        raise RuntimeError(f"{self.name}: timed out waiting for {marker} on {task}")

    def tmux_cmd(self, *args):
        return subprocess.run(["tmux", "-L", self.tmux, *args], capture_output=True, text=True, env=self.env())

    def pane(self):
        return self.tmux_cmd("capture-pane", "-p", "-t", "tui").stdout

    def wait_pane(self, needle, timeout=30):
        deadline = time.time() + timeout
        text = ""
        while time.time() < deadline:
            text = self.pane()
            if needle in text:
                time.sleep(0.5)
                return self.pane()
            time.sleep(0.2)
        return text + f"\n<<pane never showed {needle!r}>>"

    # scenario --------------------------------------------------------------
    def scenario(self):
        self.prepare()
        self.start_daemon()
        self.construct("context", "add", "local", "--endpoint", self.socket)
        self.construct("context", "use", "local")
        provider = self.construct("modelprovider", "create", "parity-anthropic", "--type", "anthropic", "--api-key", "parity-fake-key").out.strip()
        self.stop_daemon()
        # The Go CLI has no URL flag; Go reads ANTHROPIC_BASE_URL, Rust the stored URL. Set both.
        with sqlite3.connect(self.run / "data/construct/construct.db") as db:
            assert db.execute("UPDATE model_providers SET url = ? WHERE id = ?", (self.mock_url, provider)).rowcount == 1
        self.start_daemon()
        # Creating an Anthropic provider seeds its catalog models; use the seeded record.
        models = self.api.ok("ModelService", "ListModels", {}).get("models", [])
        model = next(m["metadata"]["id"] for m in models if m["spec"]["name"] == MODEL_NAME)
        self.construct("agent", "create", "parity-agent", "--model", model, "--prompt", "You are the parity agent.")
        notes = str(self.ws / "notes.txt")

        # task resource commands
        t1 = self.cap("task-create", "task", "create", "--agent", "parity-agent", "--workspace", str(self.ws)).out.strip()
        self.cap("task-create-missing-workspace", "task", "create", "--agent", "parity-agent", "--workspace", str(self.run / "nope"))
        self.cap("task-create-unknown-agent", "task", "create", "--agent", "no-such-agent")
        for fmt in ("json", "yaml", "table", "card"):
            self.cap(f"task-get-empty-{fmt}", "task", "get", t1, "-o", fmt)

        # exec
        p = self.cap("exec-tools-json", "exec", "P_EXEC_TOOLS please", "--agent", "parity-agent", "--workspace", str(self.ws), "-o", "json")
        t2 = json.loads(p.out.strip().splitlines()[-1])["task_id"]
        self.wait_idle(t2, "P_EXEC_TOOLS", title=True)
        self.cap("exec-text", "exec", "P_EXEC_TEXT please", "--agent", "parity-agent", "--workspace", str(self.ws))
        self.cap("exec-yaml", "exec", "P_YAML please", "--agent", "parity-agent", "--workspace", str(self.ws), "-o", "yaml")
        self.cap("exec-tool-error-json", "exec", "P_TOOL_ERROR please", "--agent", "parity-agent", "--workspace", str(self.ws), "-o", "json")
        self.cap("exec-stdin-file-json", "exec", "P_FILES summarize", "--agent", "parity-agent", "-f", notes, "-o", "json", stdin="piped stdin text\n")
        self.cap("exec-no-question", "exec", "--agent", "parity-agent")
        self.cap("exec-continue-id-json", "exec", f"--continue={t2}", "P_CONTINUE again", "--agent", "parity-agent", "-o", "json")
        self.wait_idle(t2, "P_CONTINUE")
        self.cap("exec-continue-last-json", "exec", "-c", "P_CONTINUE_LAST again", "--agent", "parity-agent", "-o", "json")
        self.wait_idle(t2, "P_CONTINUE_LAST")
        self.cap("exec-continue-missing", "exec", "--continue=00000000-0000-7000-8000-000000000000", "P_EXEC_TEXT", "--agent", "parity-agent")

        # message create on the empty task, with a raw event subscription
        events, stop = [], threading.Event()
        sub = threading.Thread(target=self.api.subscribe, args=({"eventTypes": ["message.*", "task.*", "tool.*"], "taskId": t1}, events, stop), daemon=True)
        sub.start()
        time.sleep(1)
        m1 = self.cap("message-create", "message", "create", t1, "P_MESSAGE go").out.strip()
        self.wait_idle(t1, "P_MESSAGE", title=True)
        time.sleep(1)
        stop.set()
        self.record("events-message-create", json.dumps(summarize_events(events), indent=1))
        self.cap("message-create-unknown-task", "message", "create", "00000000-0000-7000-8000-000000000000", "x")

        # message list / get, including tool calls and results
        for fmt in ("json", "yaml", "table", "card", "markdown"):
            self.cap(f"message-list-tools-{fmt}", "message", "list", "--task", t2, "-o", fmt)
        self.cap("message-list-tools-table-wide", "message", "list", "--task", t2, "--wide")
        self.cap("message-list-tools-json-wide", "message", "list", "--task", t2, "-o", "json", "--wide")
        self.cap("message-list-role-assistant", "message", "list", "--task", t2, "--role", "assistant", "-o", "json")
        self.cap("message-list-role-user", "message", "list", "--task", t2, "--role", "user", "-o", "json")
        self.cap("message-list-role-bad", "message", "list", "--task", t2, "--role", "system")
        self.cap("message-list-agent", "message", "list", "--task", t1, "--agent", "parity-agent", "-o", "json")
        self.cap("message-list-text-only-table", "message", "list", "--task", t1)
        msgs = self.api.ok("MessageService", "ListMessages", {"filter": {"taskIds": t2}})["messages"]
        tool_msg = next(m["metadata"]["id"] for m in msgs if any("toolCall" in p for p in m["spec"]["content"]))
        result_msg = next((m["metadata"]["id"] for m in msgs if any("toolResult" in p for p in m["spec"]["content"])), tool_msg)
        for fmt in ("json", "card"):
            self.cap(f"message-get-tool-call-{fmt}", "message", "get", tool_msg, "-o", fmt)
            self.cap(f"message-get-tool-result-{fmt}", "message", "get", result_msg, "-o", fmt)
        self.cap("message-get-missing", "message", "get", "00000000-0000-7000-8000-000000000000")

        # task get/list after execution
        for fmt in ("json", "yaml", "card"):
            self.cap(f"task-get-after-exec-{fmt}", "task", "get", t2, "-o", fmt)
        self.cap("task-list-json", "task", "list", "--limit", "20", "-o", "json")
        self.cap("task-list-table", "task", "list", "--limit", "20")
        self.cap("task-list-agent-json", "task", "list", "--agent", "parity-agent", "--limit", "2", "-o", "json")
        self.cap("task-list-default-limit", "task", "list", "-o", "json")

        # resume (TUI) through tmux
        cmd = " ".join(map(shell_quote, [self.bins[self.cli], "--context", "local", "resume", t2]))
        self.tmux_cmd("new-session", "-d", "-x", "160", "-y", "50", "-s", "tui", cmd)
        self.record_pane("resume-initial-pane", self.wait_pane("P_CONTINUE_LAST done"))
        self.tmux_cmd("send-keys", "-t", "tui", "-l", "P_RESUME_TUI now")
        self.tmux_cmd("send-keys", "-t", "tui", "Enter")
        self.wait_idle(t2, "P_RESUME_TUI")
        self.record_pane("resume-reply-pane", self.wait_pane("P_RESUME_TUI done"))
        self.tmux_cmd("kill-server")
        time.sleep(0.5)
        cmd = " ".join(map(shell_quote, [self.bins[self.cli], "--context", "local", "resume", "--last"]))
        self.tmux_cmd("new-session", "-d", "-x", "160", "-y", "50", "-s", "tui", cmd)
        self.record_pane("resume-last-pane", self.wait_pane("P_RESUME_TUI done"))
        self.tmux_cmd("kill-server")
        self.cap("resume-missing", "resume", "00000000-0000-7000-8000-000000000000")
        self.cap("message-list-after-resume-json", "message", "list", "--task", t2, "-o", "json")

        # raw API and storage snapshots before deletion
        self.record("api-get-task", dump(self.api.call("TaskService", "GetTask", {"id": t2})))
        self.record("api-list-messages", dump(self.api.call("MessageService", "ListMessages", {"filter": {"taskIds": t2}})))
        self.record("api-list-tasks", dump(self.api.call("TaskService", "ListTasks", {"filter": {}, "sortField": "SORT_FIELD_CREATED_AT", "sortOrder": "SORT_ORDER_ASC"})))
        self.record("api-list-tasks-page-size-0", dump(self.api.call("TaskService", "ListTasks", {"pageSize": 0})))
        self.record("api-get-task-missing", dump(self.api.call("TaskService", "GetTask", {"id": "00000000-0000-7000-8000-000000000000"})))

        self.known_ids = known_ids(self.run / "data/construct/construct.db")

        # deletion
        self.cap("message-delete", "message", "delete", m1, "--force")
        self.cap("message-list-after-delete", "message", "list", "--task", t1, "-o", "json")
        self.cap("task-delete", "task", "delete", t1, "--force")
        self.cap("task-get-deleted", "task", "get", t1)
        self.cap("task-list-after-delete", "task", "list", "--limit", "20", "-o", "json")
        self.stop_daemon()
        self.record("db-tasks", db_dump(self.run / "data/construct/construct.db", "tasks"))
        self.record("db-messages", db_dump(self.run / "data/construct/construct.db", "messages"))
        self.record("model-requests", model_requests(self.run / "mock-requests.jsonl"))


def shell_quote(s):
    return "'" + s.replace("'", "'\\''") + "'"


def dump(result):
    status, body = result
    return json.dumps({"http_status": status, "body": body}, indent=1, sort_keys=True)


def summarize_events(events):
    out = []
    for frame in events:
        event = frame.get("event") or {}
        if "end" in frame:
            out.append({"end": frame["end"]})
            continue
        if "messageChunk" in event:
            if out and out[-1].get("type") == "messageChunk":
                out[-1]["count"] += 1
            else:
                out.append({"type": "messageChunk", "count": 1})
            continue
        out.append(event)
    return out


def db_dump(path, table):
    with sqlite3.connect(path) as db:
        db.row_factory = sqlite3.Row
        rows = [dict(r) for r in db.execute(f"SELECT * FROM {table}")]
    for r in rows:
        for k, v in list(r.items()):
            if isinstance(v, bytes):
                r[k] = v.decode(errors="replace")
            if isinstance(r[k], str) and r[k][:1] in "[{":
                try:
                    r[k] = json.loads(r[k])
                except ValueError:
                    pass
    rows.sort(key=lambda r: str(r.get("create_time", "")))
    return json.dumps(rows, indent=1, sort_keys=True)


def known_ids(path):
    """Stable labels for generated IDs, by kind and creation order, so diffs do not cascade."""
    labels = {}
    with sqlite3.connect(path) as db:
        for table, label in (("model_providers", "PROVIDER"), ("models", "MODEL"), ("agents", "AGENT"),
                             ("tasks", "TASK"), ("messages", "MSG")):
            for n, (v,) in enumerate(db.execute(f"SELECT id FROM {table} ORDER BY create_time, rowid"), 1):
                v = v.decode() if isinstance(v, bytes) else str(v)
                labels[v] = f"<{label}{n}>"
    return labels


RESUME_MARKERS = ["P_EXEC_TOOLS please", "P_EXEC_TOOLS done", "P_CONTINUE again", "P_CONTINUE done",
                  "P_CONTINUE_LAST again", "P_CONTINUE_LAST done", "P_RESUME_TUI now", "P_RESUME_TUI done",
                  "Tokens:", "Cost:"]


def pane_summary(pane):
    """The Rust TUI is a ratatui port of the Bubble Tea UI; compare what it shows, not its layout."""
    status = next((" ".join(l.split()) for l in pane.splitlines() if "Tokens:" in l), "")
    return json.dumps({"visible": [m for m in RESUME_MARKERS if m in pane], "status": status}, indent=1)


COBRA_USAGE = re.compile(r"\nUsage:\n  construct .*?\nGlobal Flags:\n(?:      .*\n)*", re.S)


def merge_streams(text):
    """Capture with stdout and stderr concatenated, for stream-insensitive comparison."""
    m = re.search(r"--- stdout\n(.*)--- stderr\n(.*)\Z", text, re.S)
    return text if not m else text[:m.start()] + "--- output\n" + m.group(1) + m.group(2)


def model_requests(path):
    reqs = [json.loads(line) for line in path.read_text().splitlines()]
    reqs.sort(key=lambda r: (r["kind"], r["marker"], r["step"], len(json.dumps(r["request"]))))
    return json.dumps(reqs, indent=1, sort_keys=True)


UUID = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
TIMES = [
    re.compile(r"\d{4}-\d\d-\d\d[T ]\d\d:\d\d:\d\d(\.\d+)?( ?(Z|[+-]\d\d:?\d\d)( [A-Z]+)?)?( m=[+-][\d.]+)?"),
    re.compile(r"\b\d+ (second|minute|hour)s? ago\b"),
    re.compile(r"\b(just now|now)\b"),
    re.compile(r"\b\d\d:\d\d(:\d\d)?\b"),
]


def normalize(text, side, ids):
    text = text.replace(str(side.run), "<RUN>").replace(side.mock_url, "<MOCK>").replace(side.bins[side.cli], "construct")
    text = text.replace(side.socket, "<SOCK>")

    def uid(m):
        v = m.group(0)
        if v.startswith("00000000-0000-7000"):
            return "<MISSING-ID>"
        ids.setdefault(v, f"<ID{len(ids) + 1}>")
        return ids[v]
    text = UUID.sub(uid, text)
    for p in TIMES:
        text = p.sub("<TIME>", text)
    text = re.sub(r'"(createdAt|updatedAt|create_time|update_time|timestamp|created_at|updated_at)": "[^"]*"', r'"\1": "<TIME>"', text)
    text = re.sub(r"\b\d+(\.\d+)?(ms|µs|s)\b", "<DUR>", text)
    text = re.sub(r"<TIME> +", "<TIME> ", text)
    text = re.sub(r"[ \t]+\n", "\n", text)
    return text


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--go-bin", required=True)
    ap.add_argument("--rust-bin", required=True)
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--combos", default="all", help="comma list like go-go,rust-rust")
    a = ap.parse_args()
    bins = {"go": a.go_bin, "rust": a.rust_bin}
    root = a.out / time.strftime("run-%Y%m%d-%H%M%S")
    root.mkdir(parents=True)
    combos = COMBOS if a.combos == "all" else [tuple(c.split("-")) for c in a.combos.split(",")]
    sides = []
    for cli, daemon in combos:
        side = Side(root, cli, daemon, bins)
        print(f"=== {side.name}", flush=True)
        try:
            side.scenario()
        except Exception as e:  # keep going, report the failure as a capture
            side.record("scenario-error", traceback.format_exc())
            print(f"  scenario error: {e}", flush=True)
        finally:
            side.close()
        ids = dict(getattr(side, "known_ids", {}))
        norm = root / side.name / "normalized"
        norm.mkdir(exist_ok=True)
        side.normalized = {}
        for name in side.order:
            side.normalized[name] = normalize(side.captures[name], side, ids)
            (norm / f"{name}.txt").write_text(side.normalized[name])
        sides.append(side)
    base = sides[0]
    report = [f"# Task/message parity: baseline {base.name}", ""]
    failures = 0
    for other in sides[1:]:
        diffdir = root / f"diff-{other.name}"
        diffdir.mkdir()
        names = list(dict.fromkeys(base.order + other.order))
        same = [n for n in names if base.normalized.get(n) == other.normalized.get(n)]
        stream = [n for n in names if n not in same
                  and merge_streams(base.normalized.get(n, "")) == merge_streams(other.normalized.get(n, ""))]

        def no_usage(v):
            return merge_streams(COBRA_USAGE.sub("\n", v))
        usage = [n for n in names if n not in same and n not in stream
                 and no_usage(base.normalized.get(n, "")) == no_usage(other.normalized.get(n, ""))]
        done = set(same + stream + usage)

        def dedup(v):
            """Collapse repeated consecutive answers (lines for JSON/text, blank-line blocks for YAML)."""
            head, sep, body = no_usage(v).partition("--- output\n")
            lines = body.split("\n")
            lines = [line for i, line in enumerate(lines) if i == 0 or line != lines[i - 1]]
            blocks = "\n".join(lines).split("\n\n")
            return head + sep + "\n\n".join(b for i, b in enumerate(blocks) if i == 0 or b != blocks[i - 1])
        replay = [n for n in names if n not in done
                  and dedup(base.normalized.get(n, "")) == dedup(other.normalized.get(n, ""))]
        done.update(replay)

        def prefix(a, b):
            a, b = dedup(a).rstrip("\n").split("\n"), dedup(b).rstrip("\n").split("\n")
            return len(b) > len(a) and b[:len(a)] == a
        cont = [n for n in names if n not in done and n.startswith("exec-continue")
                and prefix(base.normalized.get(n, ""), other.normalized.get(n, ""))]
        done.update(cont)
        diff = [n for n in names if n not in done]
        for n in diff + stream + usage + replay + cont:
            (diffdir / f"{n}.diff").write_text("".join(difflib.unified_diff(
                base.normalized.get(n, "<missing>\n").splitlines(True), other.normalized.get(n, "<missing>\n").splitlines(True),
                f"{base.name}/{n}", f"{other.name}/{n}")))
        failures += len(diff)
        report += [f"## {other.name}: {len(same)}/{len(names)} captures identical", ""]
        report += [f"- STREAM {n} (same text, stdout/stderr split differs)" for n in stream]
        report += [f"- USAGE {n} (same error, cobra usage block not printed)" for n in usage]
        report += [f"- REPLAY {n} (same answers; Go daemon re-sends the replayed user message live)" for n in replay]
        report += [f"- CONTINUE {n} (Go output is a prefix; Go exec --continue exits on the replayed earlier answer)"
                   for n in cont]
        report += [f"- DIFF {n}" for n in diff] + [""]
    (root / "report.md").write_text("\n".join(report))
    print("\n".join(report))
    print(f"Artifacts: {root}")
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
