#!/usr/bin/env python3
"""Regenerate Go tool goldens without retaining Go build inputs in the tree."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "rust/crates/construct-tools/tests/fixtures"
generated = []
try:
    base = ROOT / "backend/tool/base/golden_generate.go"
    base.write_text((ROOT / "rust/scripts/tools-golden.go.txt").read_text())
    generated.append(base)
    for package, names in [("filesystem", ["create", "read", "list", "edit"]), ("system", ["command"])]:
        for name in names:
            source = (ROOT / f"backend/tool/{package}/{name}_test.go").read_text()
            source = re.sub(r"func Test(\w+)\(", r"func TestGolden\1(", source)
            source = source.replace("t.Skip()", "").replace("setup.RunToolTests(t,", "base.WriteGoldens(t, setup,")
            for helper in ["intPtr", "seedTestFilesystem", "generateLargeContent", "seedCreateFileTestFilesystem", "generateLargeCreateFileContent"]:
                source = re.sub(r"\b" + helper + r"\b", helper + "Golden", source)
            path = ROOT / f"backend/tool/{package}/golden_{name}_test.go"
            path.write_text(source)
            generated.append(path)
    path = ROOT / "backend/tool/filesystem/golden_grep_test.go"
    path.write_text((ROOT / "rust/scripts/tools-grep-golden.go.txt").read_text())
    generated.append(path)
    path = ROOT / "backend/tool/web/golden_fetch_test.go"
    path.write_text((ROOT / "rust/scripts/tools-web-golden.go.txt").read_text())
    generated.append(path)
    env = {**os.environ, "CGO_ENABLED": "0", "CONSTRUCT_GOLDEN_DIR": str(OUT)}
    path = ROOT / "backend/tool/filesystem/golden_find_test.go"
    path.write_text((ROOT / "rust/scripts/tools-find-golden.go.txt").read_text())
    generated.append(path)
    subprocess.run(["go", "test", "-p=1", "./backend/tool/filesystem", "./backend/tool/system", "./backend/tool/web", "-run", "^TestGolden(CreateFile|ReadFile|ListFiles|EditFile|ExecuteCommand|ParseRipgrepOutput|FindFallback|Web)$", "-count=1"], cwd=ROOT, env=env, check=True)
    blobs = OUT / "blobs"
    blobs.mkdir(exist_ok=True)
    for path in OUT.glob("TestGolden*.json"):
        cases = json.loads(path.read_text())
        for case in cases:
            for entry in case["before"] + case["after"]:
                if not entry["directory"]:
                    content = bytes(entry.pop("bytes", []))
                    key = hashlib.sha256(content).hexdigest()
                    (blobs / key).write_bytes(content)
                    entry["blob"] = key
        path.write_text(json.dumps(cases, indent=2, ensure_ascii=False) + "\n")
finally:
    for path in generated:
        path.unlink(missing_ok=True)
