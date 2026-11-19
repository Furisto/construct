#!/usr/bin/env python3
"""Deterministic loopback Anthropic SSE mock for parity-tasks.py; no real credentials.

Each user prompt carries a P_* marker that selects a script. A script is a list of
assistant turns; the turn index is the number of tool results after the latest
user prompt. Requests without tools are title generation requests.
"""
import argparse
import json
import re
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


def scripts(ws):
    q = json.dumps
    notes = q(str(ws / "notes.txt"))
    return {
        "P_EXEC_TOOLS": [
            [("text", "Let me look at the workspace."), ("code", f'create_file({notes}, "alpha\\nbeta\\n");\nprint(read_file({notes}));\nprint(list_files({q(str(ws))}, false).entries.map(e => e.n).sort());\nprint(execute_command("echo parity-cmd"));')],
            [("text", "P_EXEC_TOOLS done <ok> & \"quoted\"")],
        ],
        "P_EXEC_TEXT": [[("text", "P_EXEC_TEXT answer\nsecond line")]],
        "P_YAML": [[("text", "P_YAML answer: with colon")]],
        "P_CONTINUE": [
            [("code", f"print(read_file({notes}));")],
            [("text", "P_CONTINUE done")],
        ],
        "P_CONTINUE_LAST": [[("text", "P_CONTINUE_LAST done")]],
        "P_TOOL_ERROR": [
            [("code", f'read_file({q(str(ws / "missing.txt"))});')],
            [("text", "P_TOOL_ERROR recovered")],
        ],
        "P_FILES": [[("text", "P_FILES done")]],
        "P_MESSAGE": [
            [("code", 'print("from message create");')],
            [("text", "P_MESSAGE done")],
        ],
        "P_RESUME_TUI": [
            [("code", 'print("from resume");')],
            [("text", "P_RESUME_TUI done")],
        ],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workspace", type=Path, required=True)
    parser.add_argument("--port-file", type=Path, required=True)
    parser.add_argument("--requests", type=Path, required=True)
    args = parser.parse_args()
    lock = threading.Lock()
    table = scripts(args.workspace)
    pattern = re.compile(r"\b(" + "|".join(sorted(table, key=len, reverse=True)) + r")\b")

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
            def is_result(m):
                return isinstance(m["content"], list) and any(b.get("type") == "tool_result" for b in m["content"])
            prompts = [i for i, m in enumerate(messages) if m["role"] == "user" and not is_result(m)]
            latest = prompts[-1] if prompts else 0
            found = pattern.findall(json.dumps(messages[latest].get("content", "")))
            marker = found[-1] if found else "UNKNOWN"
            step = sum(1 for m in messages[latest + 1:] if m["role"] == "user" and is_result(m))
            if not request.get("tools"):
                kind, blocks, finish = "title", [("text", "Parity title")], "end_turn"
            else:
                turns = table.get(marker, [[("text", "UNKNOWN marker")]])
                blocks = turns[min(step, len(turns) - 1)]
                kind = "turn"
                finish = "tool_use" if any(b[0] == "code" for b in blocks) else "end_turn"
            with lock, args.requests.open("a") as log:
                log.write(json.dumps({"kind": kind, "marker": marker, "step": step, "request": request}) + "\n")
            if kind == "turn":
                # Give exec time to read the replayed prompt before model usage can advance.
                time.sleep(0.25)
            usage = {"input_tokens": 30, "output_tokens": 0, "cache_creation_input_tokens": 5, "cache_read_input_tokens": 7}
            chunks = [{"type": "message_start", "message": {"id": "msg_parity", "type": "message", "role": "assistant", "model": request["model"], "content": [], "stop_reason": None, "stop_sequence": None, "usage": usage}}]
            for index, (block, value) in enumerate(blocks):
                if block == "text":
                    chunks += [
                        {"type": "content_block_start", "index": index, "content_block": {"type": "text", "text": ""}},
                        {"type": "content_block_delta", "index": index, "delta": {"type": "text_delta", "text": value}},
                    ]
                else:
                    chunks += [
                        {"type": "content_block_start", "index": index, "content_block": {"type": "tool_use", "id": f"toolu_{marker}_{step}", "name": "code_interpreter", "input": {}}},
                        {"type": "content_block_delta", "index": index, "delta": {"type": "input_json_delta", "partial_json": json.dumps({"script": value})}},
                    ]
                chunks.append({"type": "content_block_stop", "index": index})
            chunks += [
                {"type": "message_delta", "delta": {"stop_reason": finish, "stop_sequence": None}, "usage": {"output_tokens": 15}},
                {"type": "message_stop"},
            ]
            body = "".join("event: " + c["type"] + "\ndata: " + json.dumps(c) + "\n\n" for c in chunks).encode()
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
