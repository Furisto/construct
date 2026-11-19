"""Drive a real controlling terminal without exposing supplied credentials."""
import errno
import json
import os
import pty
import select
import signal
import sys
import termios
import time


def run(request):
    pid, master = pty.fork()
    if pid == 0:
        os.execv(request["command"][0], request["command"])
    output = bytearray()
    status = None

    def collect(duration):
        nonlocal status
        deadline = time.monotonic() + duration
        while time.monotonic() < deadline:
            if select.select([master], [], [], min(0.02, max(0, deadline - time.monotonic())))[0]:
                try:
                    data = os.read(master, 65536)
                except OSError as error:
                    if error.errno != errno.EIO:
                        raise
                    data = b""
                output.extend(data)
            if status is None:
                finished, child_status = os.waitpid(pid, os.WNOHANG)
                if finished:
                    status = os.waitstatus_to_exitcode(child_status)
            if status is not None:
                return

    try:
        deadline = time.monotonic() + 15
        while request["prompt"].encode() not in output:
            collect(0.02)
            if status is not None or time.monotonic() >= deadline:
                raise RuntimeError("process did not reach the requested prompt")
        while termios.tcgetattr(master)[3] & termios.ECHO:
            collect(0.01)
            if status is not None or time.monotonic() >= deadline:
                raise RuntimeError("password prompt did not disable terminal echo")
        hidden = not bool(termios.tcgetattr(master)[3] & termios.ECHO)
        value = request["input"].encode()
        os.write(master, value)
        collect(0.1)
        echoed = bool(value and value in output)
        os.write(master, b"\x03" if request.get("interrupt") else b"\r")
        collect(0.3)
        needed_enter = request.get("interrupt", False) and status is None
        if needed_enter:
            os.write(master, b"\r")
        collect(15)
        if status is None:
            raise RuntimeError("process did not finish after Enter")
        restored = bool(termios.tcgetattr(master)[3] & termios.ECHO)
        transcript = output.decode(errors="replace").replace("\r\n", "\n")
        if request["input"]:
            transcript = transcript.replace(request["input"], "[REDACTED]")
        return dict(status=status, hidden=hidden, echoed=echoed, restored=restored,
                    needed_enter_after_interrupt=needed_enter, transcript=transcript)
    finally:
        if status is None:
            os.kill(pid, signal.SIGKILL)
            os.waitpid(pid, 0)
        os.close(master)


if __name__ == "__main__":
    print(json.dumps(run(json.load(sys.stdin))))
