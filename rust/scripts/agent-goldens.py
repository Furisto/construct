#!/usr/bin/env python3
"""Regenerate agent fixtures with the unchanged Go implementations."""
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "rust/crates/construct-agent/tests/fixtures"
with tempfile.TemporaryDirectory(prefix="construct-agent-goldens-") as temporary:
    main = Path(temporary) / "main.go"
    main.write_text((ROOT / "rust/scripts/agent-golden.go.txt").read_text())
    subprocess.run(
        ["go", "run", str(main), str(OUT)],
        cwd=ROOT,
        env={**os.environ, "CGO_ENABLED": "0"},
        check=True,
    )
