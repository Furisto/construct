#!/usr/bin/env python3
"""Byte-level stdout/stderr/status parity. Artifacts persist under rust/target/parity.

Only UUIDs in successful-create comparisons are normalized: independent creates
allocate different IDs. Reads and errors are compared without normalization.
No JSON sorting, whitespace, ANSI, timestamp, or error normalization.
Run with --strict to fail on any difference; default inventories known differences.
"""
import difflib
import json
import os
from pathlib import Path
import re
import sqlite3
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "rust/target/parity" / str(time.time_ns())
OUT.mkdir(parents=True)
GO = ROOT / "rust/target/construct-go"
RUST = ROOT / "rust/target/debug/construct"


def normalize(s):
    return re.sub(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", "<UUID>", s)


def phase(server):
    directory = OUT / server
    directory.mkdir()
    env = dict(os.environ)
    for key, name in [("HOME", "home"), ("XDG_CONFIG_HOME", "config"), ("XDG_DATA_HOME", "data"), ("XDG_STATE_HOME", "state"), ("XDG_RUNTIME_DIR", "runtime")]:
        env[key] = str(directory / name)
        Path(env[key]).mkdir()
    config = Path(env["XDG_CONFIG_HOME"]) / "construct"
    config.mkdir()
    (config / "config.yaml").write_text("secret:\n  provider: file\n")
    workspace = directory / "workspace"
    workspace.mkdir()
    mock = subprocess.Popen([sys.executable, str(ROOT / "rust/scripts/e2e-mock.py"), "--workspace", str(workspace), "--port-file", str(directory / "port"), "--requests", str(directory / "requests.jsonl")], stdout=(directory / "mock.log").open("w"), stderr=subprocess.STDOUT)
    daemon = None
    results = []

    def run(binary, args):
        p = subprocess.run([str(binary), *args], env=env, capture_output=True, text=True, timeout=30)
        return p.returncode, p.stdout, p.stderr

    def seed(args):
        code, out, err = run(GO, args)
        if code:
            raise RuntimeError((args, code, out, err))
        return (out or err).strip()

    def compare(label, args, before=None, after=None):
        if before:
            before()
        go_args = args() if callable(args) else args
        go = run(GO, go_args)
        if after:
            after()
        if before:
            before()
        rust_args = args() if callable(args) else args
        rust = run(RUST, rust_args)
        if after:
            after()
        clean = normalize if "-create" in label and go[0] == rust[0] == 0 else str
        equal = tuple(map(clean, map(str, go))) == tuple(map(clean, map(str, rust)))
        results.append((label, equal))
        (directory / (label + ".json")).write_text(json.dumps({"args": [go_args, rust_args], "go": go, "rust": rust}, indent=2))
        if not equal:
            (directory / (label + ".diff")).write_text("".join(difflib.unified_diff(str(tuple(map(clean, map(str, go)))).splitlines(True), str(tuple(map(clean, map(str, rust)))).splitlines(True), fromfile="go", tofile="rust")))
        print(f"{server} {label}: {'MATCH' if equal else 'DIFFERS'}", flush=True)

    try:
        for _ in range(100):
            if (directory / "port").exists():
                break
            time.sleep(.1)
        env["ANTHROPIC_BASE_URL"] = "http://127.0.0.1:" + (directory / "port").read_text() + "/"
        sock = directory / "daemon.sock"
        # Unix socket paths are bounded to 108 bytes on Linux.
        sock = Path(env["XDG_RUNTIME_DIR"]) / "d.sock"
        daemon = subprocess.Popen([str(GO), "daemon", "run", "--listen-unix", str(sock)], env=env, stdout=(directory / "daemon.log").open("w"), stderr=subprocess.STDOUT)
        for _ in range(150):
            if sock.exists():
                break
            if daemon.poll() is not None:
                raise RuntimeError("daemon exited: " + (directory / "daemon.log").read_text())
            time.sleep(.1)
        seed(["context", "add", "local", "--endpoint", str(sock)])
        seed(["context", "use", "local"])
        provider = seed(["modelprovider", "create", "parity", "--type", "anthropic", "--api-key", "fake"])
        model = seed(["model", "create", "parity", "--provider", provider, "--context-window", "128000"])
        agent = seed(["agent", "create", "parity", "--model", model, "--prompt", "Answer briefly"])
        task = seed(["task", "create", "--agent", agent, "--workspace", str(workspace)])
        seed(["message", "create", task, "GO_INITIAL"])
        for _ in range(150):
            messages = json.loads(seed(["message", "list", "--task", task, "-o", "json", "--wide"]))
            if "MOCK_REPLY" in json.dumps(messages):
                break
            time.sleep(.2)
        else:
            raise RuntimeError("mock model reply timed out")
        if server == "rust":
            daemon.terminate()
            daemon.wait(timeout=15)
            # Rust honors the stored URL, not the Go SDK's environment override.
            with sqlite3.connect(Path(env["XDG_DATA_HOME"]) / "construct/construct.db") as db:
                db.execute("UPDATE model_providers SET url = ? WHERE id = ?", (env["ANTHROPIC_BASE_URL"].rstrip("/"), provider))
            daemon = subprocess.Popen([str(RUST), "daemon", "run", "--listen-unix", str(sock)], env=env, stdout=(directory / "rust-daemon.log").open("w"), stderr=subprocess.STDOUT)
            for _ in range(150):
                if run(GO, ["agent", "list"])[0] == 0:
                    break
                time.sleep(.1)
            else:
                raise RuntimeError("Rust daemon readiness timed out")
        ids = dict(modelprovider=provider, model=model, agent=agent, task=task, message=messages[0]["id"])
        for resource, identifier in ids.items():
            for fmt in ["default", "table", "json", "yaml", "card", "markdown"]:
                flags = [] if fmt == "default" else ["-o", fmt]
                extra = ["--limit", "100"] if resource == "task" else ["--task", task] if resource == "message" else []
                compare(f"{resource}-list-{fmt}", [resource, "list", *extra, *flags])
                compare(f"{resource}-get-{fmt}", [resource, "get", identifier, *flags])
            for name, args in [("missing", ["get", "00000000-0000-0000-0000-000000000000"]), ("bad-args", ["get"]), ("unknown-flag", ["list", "--unknown"]), ("invalid-format", ["list", "-o", "invalid"])]:
                compare(f"{resource}-{name}", [resource, *args])
        compare("missing-required", ["model", "create", "bad"])
        creates = {
            "modelprovider": ["temporary", "--type", "anthropic", "--api-key", "fake"],
            "model": ["temporary", "--provider", provider, "--context-window", "1000"],
            "agent": ["temporary", "--model", model, "--prompt", "Answer briefly"],
            "task": ["--agent", agent, "--workspace", str(workspace)],
        }
        for resource, args in creates.items():
            def remove_created():
                data = json.loads(seed([resource, "list", "-o", "json", *(["--limit", "100"] if resource == "task" else [])]))
                identifier = next(item["id"] for item in data if item.get("name") == "temporary" or (resource == "task" and item["id"] != task))
                seed([resource, "delete", identifier, "--force"])

            compare(f"{resource}-create", [resource, "create", *args], after=remove_created)
        for resource in ["modelprovider", "model", "agent"]:
            compare(f"{resource}-delete", [resource, "delete", "temporary", "--force"], before=lambda r=resource: seed([r, "create", *creates[r]]))
        pending = []

        def create_empty_task():
            pending[:] = [seed(["task", "create", *creates["task"]])]

        compare("task-delete", lambda: ["task", "delete", pending[0], "--force"], before=create_empty_task)

        def create_message():
            create_empty_task()
            task_id = pending[0]
            message_id = seed(["message", "create", task_id, "GO_INITIAL"])
            for _ in range(150):
                data = seed(["message", "list", "--task", task_id, "-o", "json", "--wide"])
                if "MOCK_REPLY" in data:
                    pending[:] = [message_id]
                    return
                time.sleep(.2)
            raise RuntimeError("delete fixture model reply timed out")

        compare("message-delete", lambda: ["message", "delete", pending[0], "--force"], before=create_message)

        def wait_created_message():
            for _ in range(150):
                data = seed(["message", "list", "--task", pending[0], "-o", "json", "--wide"])
                if "MOCK_REPLY" in data:
                    return
                time.sleep(.2)
            raise RuntimeError("create fixture model reply timed out")

        for fmt in ["default", "table", "json", "yaml", "card", "markdown"]:
            flags = [] if fmt == "default" else ["-o", fmt]
            compare(f"message-create-{fmt}", lambda: ["message", "create", pending[0], "GO_INITIAL", *flags], before=create_empty_task, after=wait_created_message)
            compare(f"message-empty-{fmt}", ["message", "list", "--task", "00000000-0000-0000-0000-000000000000", *flags])
            for label, args in [("skill-list", ["skill", "list"]), ("context-list", ["context", "list"]), ("info", ["info"]), ("token-list", ["daemon", "token", "list"])]:
                compare(f"{label}-{fmt}", [*args, *flags])
        compare("config-list", ["config", "list"])
        compare("config-get", ["config", "get", "secret.provider"])
        compare("context-current", ["context", "current"])
        # A failed create is repeatable on the same daemon; successful creates
        # need distinct names and comparison of only their returned ID.
        compare("model-create-duplicate", ["model", "create", "parity", "--provider", provider, "--context-window", "1000"])
        for resource in ids:
            compare(f"{resource}-delete-missing", [resource, "delete", "00000000-0000-0000-0000-000000000000", "--force"])
        daemon.terminate()
        daemon.wait(timeout=15)
        compare("unreachable", ["agent", "list"])
    finally:
        for process in [daemon, mock]:
            if process and process.poll() is None:
                process.terminate()
                process.wait(timeout=15)
    return results


all_results = phase("go") + phase("rust")
matches = sum(ok for _, ok in all_results)
print(f"SUMMARY: {matches}/{len(all_results)} match; {len(all_results)-matches} differ")
print(f"Artifacts: {OUT}")
sys.exit(1 if "--strict" in sys.argv and matches != len(all_results) else 0)
