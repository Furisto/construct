#!/usr/bin/env python3
"""Windows TCP and CodeAct smoke test with a Go-created Credential Manager entry.

Build construct with cargo and compile scripts/keyring-interop.go.txt as a Go
program from the repo root, then pass both executable paths.
"""

import argparse
from contextlib import contextmanager
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.request import ProxyHandler, Request, build_opener
import uuid


@contextmanager
def process(args, log, env):
    with log.open("wb") as output, subprocess.Popen(
        args, stdin=subprocess.DEVNULL, stdout=output, stderr=output, env=env
    ) as child:
        try:
            yield child
        except Exception:
            print(log.read_text(encoding="utf-8", errors="replace"), file=sys.stderr)
            raise
        finally:
            child.terminate()
            child.wait(timeout=10)


def wait_until(child, ready):
    for _ in range(150):
        if child.poll() is not None:
            raise RuntimeError(f"process exited with status {child.returncode}")
        if result := ready():
            return result
        time.sleep(0.1)
    raise TimeoutError("process did not become ready")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("construct", type=Path)
    parser.add_argument("go_keyring", type=Path)
    args = parser.parse_args()
    binaries = args.construct.resolve(), args.go_keyring.resolve()
    assert os.name == "nt", "run on native Windows"
    env = os.environ.copy()
    opener = build_opener(ProxyHandler({}))

    def run(binary, *argv, **kwargs):
        return subprocess.run(
            [binary, *argv], env=env, stdin=subprocess.DEVNULL,
            encoding="utf-8", capture_output=True, timeout=45, **kwargs
        )

    construct, go = binaries
    name = f"windows-e2e-{uuid.uuid4()}"
    with tempfile.TemporaryDirectory(prefix="construct-windows-") as tmp:
        root = Path(tmp)
        for var, directory in {
            "XDG_CONFIG_HOME": "config", "XDG_DATA_HOME": "data",
            "XDG_STATE_HOME": "state", "XDG_RUNTIME_DIR": "runtime",
        }.items():
            env[var] = str(root / directory)
        config = root / "config" / "construct"
        config.mkdir(parents=True)
        (config / "config.yaml").write_text("secret:\n  provider: file\n")
        workspace = root / "workspace with spaces"
        workspace.mkdir()
        port_file = root / "mock.port"
        mock = [
            sys.executable, Path(__file__).with_name("e2e-mock.py"),
            "--workspace", workspace, "--port-file", port_file,
            "--requests", root / "requests.jsonl", "--windows-shell",
        ]
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        endpoint = f"http://127.0.0.1:{port}"

        def healthy():
            try:
                with opener.open(f"{endpoint}/healthz", timeout=0.5) as response:
                    return response.status == 200
            except URLError:
                return False

        def rpc(procedure, body, token=None):
            headers = {"Content-Type": "application/json", "Connect-Protocol-Version": "1"}
            if token:
                headers["Authorization"] = f"Bearer {token}"
            request = Request(f"{endpoint}/api/construct.v1.{procedure}",
                              json.dumps(body).encode(), headers)
            with opener.open(request, timeout=5) as response:
                return json.load(response)

        daemon = [construct, "daemon", "run", "--listen-http", f"127.0.0.1:{port}"]
        with process(mock, root / "mock.log", env) as model, process(
            daemon, root / "daemon.log", env
        ) as host:
            model_port = wait_until(model, lambda: port_file.read_text() if port_file.exists() else "")
            wait_until(host, healthy)
            try:
                rpc("AgentService/ListAgents", {})
                raise AssertionError("unauthenticated TCP request accepted")
            except HTTPError as error:
                assert error.code == 401, error
            issued = run(construct, "daemon", "token", "create", name, "--local", "-o", "json")
            assert issued.returncode == 0, issued.stderr
            token = json.loads(issued.stdout)["token"]
            try:
                assert run(go, "set", name, token).returncode == 0
                context_file = config / "context.yaml"
                context_file.write_text(json.dumps({
                    "current": name, "contexts": {name: {
                        "address": endpoint, "kind": "http",
                        "auth": {"type": "token", "token-ref": f"keyring://construct/{name}"},
                    }},
                }))
                rpc("ModelProviderService/CreateModelProvider", {
                    "name": "mock", "providerType": "MODEL_PROVIDER_TYPE_ANTHROPIC",
                    "apiKey": "mock-only", "url": f"http://127.0.0.1:{model_port}",
                }, token)
                try:
                    rpc("AuthService/CreateToken", {"name": "must-not-create"}, token)
                    raise AssertionError("bearer token granted admin access")
                except HTTPError as error:
                    assert error.code == 403, error
                commands = [
                    ("model", "create", "mock-model", "--provider", "mock", "--context-window", "128000"),
                    ("agent", "create", "mock-agent", "--model", "mock-model", "--prompt", "Use the tools."),
                    ("exec", "--agent", "mock-agent", "--workspace", workspace, "RUST_TCP"),
                ]
                for command in commands:
                    result = run(construct, *command)
                    assert result.returncode == 0, result.stderr
                assert "MOCK_REPLY RUST_TCP tool-ok" in result.stdout, result.stdout
                assert (workspace / "proof-RUST_TCP.txt").read_text() == "CODEACT_OK RUST_TCP"
                assert (workspace / "shell-proof.txt").read_text() == "POWERSHELL_OK"
                assert token not in context_file.read_text()
                removed = run(construct, "context", "remove", name, "--force")
                assert removed.returncode == 0, removed.stderr
                assert run(go, "get", name).returncode != 0, "Rust did not delete the Go entry"
            finally:
                run(go, "delete", name)
    print("PASS: native TCP auth and local enrollment; Go credential read/deleted by Rust; mock reply and CodeAct proof")


if __name__ == "__main__":
    main()
