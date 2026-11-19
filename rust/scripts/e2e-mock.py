#!/usr/bin/env python3
"""Loopback-only Anthropic SSE fixture for daemon smoke tests; no real credentials."""
import argparse
import json
import re
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workspace", type=Path, required=True)
    parser.add_argument("--port-file", type=Path, required=True)
    parser.add_argument("--requests", type=Path, required=True)
    parser.add_argument("--windows-shell", action="store_true")
    args = parser.parse_args()
    lock = threading.Lock()

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, *_):
            pass

        def do_POST(self):
            if self.path != "/v1/messages":
                self.send_error(404)
                return
            request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            messages = request["messages"]
            latest = max((i for i, m in enumerate(messages) if m["role"] == "user" and "tool_result" not in json.dumps(m["content"])), default=0)
            user = json.dumps(messages[latest].get("content", ""))
            marker = next(iter(re.findall(r"\b(?:GO_INITIAL|RUST_TCP|GO_RESUMED|TUI_MESSAGE)\b", user)), "TITLE")
            has_result = any("tool_result" in json.dumps(m["content"]) for m in messages[latest + 1:])
            if not request.get("tools"):
                text = '"Mock conversation"'
                finish = "end_turn"
            elif not has_result:
                script = f'create_file({json.dumps(str(args.workspace / ("proof-" + marker + ".txt")))}, {json.dumps("CODEACT_OK " + marker)}); print({json.dumps("CODEACT_OK " + marker)});'
                if args.windows_shell:
                    script += 'execute_command("[IO.File]::WriteAllText((Join-Path (Get-Location).Path shell-proof.txt), \'POWERSHELL_OK\')");'
                text = None
                finish = "tool_use"
            else:
                text = "MOCK_REPLY " + marker + " tool-ok"
                finish = "end_turn"
            with lock, args.requests.open("a") as log:
                log.write(json.dumps({"marker": marker, "has_result": has_result, "request": request}) + "\n")
            chunks = [{"type": "message_start", "message": {"id": "msg_e2e", "type": "message", "role": "assistant", "model": request["model"], "content": [], "stop_reason": None, "stop_sequence": None, "usage": {"input_tokens": 30, "output_tokens": 0}}}]
            if text is None:
                chunks += [
                    {"type": "content_block_start", "index": 0, "content_block": {"type": "tool_use", "id": "call_" + marker, "name": "code_interpreter", "input": {}}},
                    {"type": "content_block_delta", "index": 0, "delta": {"type": "input_json_delta", "partial_json": json.dumps({"script": script})}},
                ]
            else:
                chunks += [
                    {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}},
                    {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": text}},
                ]
            chunks += [
                {"type": "content_block_stop", "index": 0},
                {"type": "message_delta", "delta": {"stop_reason": finish, "stop_sequence": None}, "usage": {"output_tokens": 15}},
                {"type": "message_stop"},
            ]
            body = "".join("event: " + chunk["type"] + "\ndata: " + json.dumps(chunk) + "\n\n" for chunk in chunks).encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    args.port_file.write_text(str(server.server_port))
    server.serve_forever()


if __name__ == "__main__":
    main()
