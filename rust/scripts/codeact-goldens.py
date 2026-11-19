#!/usr/bin/env python3
"""Run the real Go CodeAct interpreter without writing Go sources under backend/."""
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "rust/crates/construct-codeact/tests/fixtures/go.json"
OUT.parent.mkdir(parents=True, exist_ok=True)
with tempfile.TemporaryDirectory(prefix="construct-codeact-generator-") as temporary:
    main = Path(temporary) / "main.go"
    main.write_text((ROOT / "rust/scripts/codeact-golden.go.txt").read_text())
    subprocess.run(
        ["go", "run", str(main), str(OUT)],
        cwd=ROOT,
        env={**os.environ, "CGO_ENABLED": "0"},
        check=True,
    )
