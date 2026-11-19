#!/usr/bin/env python3
"""Go vs Rust daemon parity for skills: install, update, delete, list, discovery.

Runs one scenario against the Go daemon, then the same scenario against the Rust
daemon in the same isolated HOME, and diffs RPC responses, on-disk state
(skills/ tree, skills.lock.json) and the <available_skills> system-prompt block.
Everything is loopback: SKILL.md URLs come from a local HTTP fixture, git
installs use a file:// fixture repo (seeded through the lock file, then
installed via UpdateSkill), and model turns hit a mock Anthropic endpoint.
Set SKILLS_PARITY_GITHUB=1 to also install from github.com (network).
Run through skills-parity.sh, which builds both binaries and starts D-Bus.
"""
import argparse, difflib, hashlib, http.client, json, os, re, shutil, socket
import sqlite3, subprocess, sys, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

# Response differences that are deliberate in the Rust port, keyed by step name.
# On-disk state after these steps must still match.
ALLOWED = {
    "url-missing-name": "Go joins an empty name onto the skills dir (\"skill \"\" already exists\"); "
                        "Rust rejects it as an invalid name",
    "delete-traversal": "Go joins the raw name onto the skills dir (\"../x\" reaches outside it); "
                        "Rust rejects names that are not one path component",
    "update-git-traversal": "Go searches outside the clone for a lock path like \"../..\"; "
                            "Rust reports the path as not found",
    "url-bad-yaml": "YAML syntax error wording comes from libyaml (Go) vs yaml-rust2 (Rust)",
    "url-no-scheme": "HTTP transport error wording comes from net/http (Go) vs reqwest (Rust)",
    "update-git-missing-ref": "git error wording comes from go-git (Go) vs gix (Rust)",
    "github-missing-repo": "git error wording comes from go-git (Go) vs gix (Rust)",
}

TS = re.compile(r'"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)"')


def skill_md(name, description, extra=""):
    return f"---\nname: {name}\ndescription: {description}\n{extra}---\n# {name}\n\nBody.\n"


URL_FIXTURES = {
    "/basic/SKILL.md": skill_md("url-basic", "Basic URL skill"),
    "/crlf/SKILL.md": skill_md("url-crlf", "CRLF skill").replace("\n", "\r\n"),
    "/block/SKILL.md": "---\nname: url-block\ndescription: |\n  Line one <b>\n  \"quoted\" & line two\nlicense: MIT\nmetadata:\n  nested: [1, 2]\n---\nbody\n",
    "/quoted/SKILL.md": "---\nname: \"url-quoted\"\ndescription: 'It''s quoted: yes'\n---\n",
    "/numeric/SKILL.md": "---\nname: 123\ndescription: 4.50\n---\n",
    "/seq-name/SKILL.md": "---\nname: [a, b]\ndescription: x\n---\n",
    "/bad-yaml/SKILL.md": "---\nname: [unclosed\ndescription: x\n---\n",
    "/no-frontmatter/SKILL.md": "# just markdown\n",
    "/empty-frontmatter/SKILL.md": "---\n---\nbody\n",
    "/no-close/SKILL.md": "---\nname: x\ndescription: y\n",
    "/invalid-name/SKILL.md": skill_md("Upper Case", "Invalid name"),
    "/missing-name/SKILL.md": "---\ndescription: no name\n---\n",
    "/missing-desc/SKILL.md": "---\nname: url-nodesc\n---\n",
    "/dup-key/SKILL.md": "---\nname: url-a\nname: url-b\ndescription: dup\n---\n",
    "/lower/skill.md": skill_md("url-lower", "Lowercase file name"),
    "/update/SKILL.md": skill_md("url-update", "Version 1"),
}


class Fixtures:
    def __init__(self):
        self.files = dict(URL_FIXTURES)
        fixtures = self

        class H(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_GET(self):
                body = fixtures.files.get(self.path)
                if body is None:
                    self.send_error(404)
                    return
                data = body.encode()
                self.send_response(200)
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), H)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.base = f"http://127.0.0.1:{self.server.server_port}"


class MockModel:
    """Anthropic SSE endpoint that records system prompts and always ends the turn."""

    def __init__(self):
        self.systems = []
        mock = self

        class H(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *_):
                pass

            def do_POST(self):
                req = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                system = req.get("system", "")
                if isinstance(system, list):
                    system = "".join(b.get("text", "") for b in system)
                if req.get("tools"):
                    mock.systems.append(system)
                text = "MOCK_OK"
                chunks = [
                    {"type": "message_start", "message": {"id": "m", "type": "message", "role": "assistant", "model": req["model"], "content": [], "stop_reason": None, "stop_sequence": None, "usage": {"input_tokens": 1, "output_tokens": 0}}},
                    {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}},
                    {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": text}},
                    {"type": "content_block_stop", "index": 0},
                    {"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence": None}, "usage": {"output_tokens": 1}},
                    {"type": "message_stop"},
                ]
                body = "".join(f"event: {c['type']}\ndata: {json.dumps(c)}\n\n" for c in chunks).encode()
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), H)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.server.server_port}/"


class UnixConn(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost", timeout=120)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(120)
        self.sock.connect(self.path)


def git(repo, *args):
    subprocess.run(["git", "-C", str(repo), "-c", "user.name=t", "-c", "user.email=t@t",
                    "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", *args],
                   check=True, capture_output=True)


def write(path, content):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


class Run:
    def __init__(self, args):
        self.args = args
        self.root = Path(args.run).resolve()
        self.home = self.root / "home"
        self.config = self.root / "config" / "construct"
        self.skills = self.config / "skills"
        self.lock = self.config / "skills.lock.json"
        self.sock = str(self.root / "daemon.sock")
        self.origin = self.root / "origin"
        self.ws = self.root / "workspace"
        self.fx = Fixtures()
        self.mock = MockModel()
        self.daemon = None
        self.env = dict(os.environ, HOME=str(self.home), XDG_CONFIG_HOME=str(self.root / "config"),
                        XDG_DATA_HOME=str(self.root / "data"), XDG_STATE_HOME=str(self.root / "state"),
                        XDG_RUNTIME_DIR=str(self.root / "runtime"), ANTHROPIC_BASE_URL=self.mock.url,
                        TMPDIR=str(self.root / "tmp"))
        for d in [self.home, self.config, self.root / "runtime", self.root / "tmp"]:
            d.mkdir(parents=True, exist_ok=True)
        (self.root / "runtime").chmod(0o700)
        (self.config / "config.yaml").write_text("secret:\n  provider: file\n")

    # ---- daemon / RPC -------------------------------------------------------
    def start(self, impl):
        binary = self.args.go if impl == "go" else self.args.rust
        log = open(self.root / f"{impl}-daemon.log", "a")
        self.daemon = subprocess.Popen([binary, "daemon", "run", "--listen-unix", self.sock],
                                       env=self.env, stdout=log, stderr=subprocess.STDOUT)
        for _ in range(300):
            try:
                c = UnixConn(self.sock)
                c.request("GET", "/healthz")
                if c.getresponse().status == 200:
                    return
            except OSError:
                pass
            if self.daemon.poll() is not None:
                sys.exit(f"{impl} daemon exited; see {log.name}")
            time.sleep(0.1)
        sys.exit(f"{impl} daemon not ready")

    def stop(self):
        self.daemon.terminate()
        self.daemon.wait(30)
        self.daemon = None

    def rpc(self, method, body):
        c = UnixConn(self.sock)
        c.request("POST", f"/api/construct.v1.SkillService/{method}", json.dumps(body),
                  {"Content-Type": "application/json"})
        r = c.getresponse()
        data = json.loads(r.read() or b"{}")
        return {"http": r.status, "body": data}

    def cli(self, *args):
        out = subprocess.run([self.args.go, *args], env=self.env, capture_output=True, text=True)
        if out.returncode:
            sys.exit(f"go {' '.join(args)} failed: {out.stdout}{out.stderr}")
        return (out.stdout + out.stderr).strip()

    def cli_both(self, argv):
        """Run the Go and the Rust CLI against the running daemon."""
        out = {}
        for impl, binary in (("go_cli", self.args.go), ("rust_cli", self.args.rust)):
            r = subprocess.run([binary, "--context", "local", *argv], env=self.env,
                               capture_output=True, text=True, stdin=subprocess.DEVNULL)
            out[impl] = {"rc": r.returncode, "stdout": CLI_TIME.sub("<time>", r.stdout),
                         "stderr": CLI_TIME.sub("<time>", r.stderr)}
        return out

    # ---- fixtures -----------------------------------------------------------
    def make_origin(self):
        if self.origin.exists():
            shutil.rmtree(self.origin)
        self.origin.mkdir()
        git(self.origin, "init", "-q", "-b", "main")
        write(self.origin / "skills/alpha/SKILL.md", skill_md("alpha", "Alpha v1"))
        write(self.origin / "skills/alpha/scripts/run.sh", "echo alpha\n")
        write(self.origin / "skills/alpha/README.md", "excluded\n")
        write(self.origin / "skills/alpha/metadata.json", "{}\n")
        write(self.origin / "skills/alpha/_private/x.md", "excluded\n")
        write(self.origin / "skills/alpha/references/doc.md", "ref\n")
        write(self.origin / "skills/beta/SKILL.md", skill_md("beta", "Beta"))
        write(self.origin / "skills/_draft/SKILL.md", skill_md("draft", "Draft"))
        write(self.origin / "skills/Invalid/SKILL.md", skill_md("Invalid", "Bad name"))
        write(self.origin / "nested/x/.hidden/hid/SKILL.md", skill_md("hid", "Hidden"))
        write(self.origin / "nested/x/y/deep/SKILL.md", skill_md("deep", "Deep"))
        write(self.origin / "nested/x/y/deep/assets/t.txt", "asset\n")
        git(self.origin, "add", "-A")
        git(self.origin, "commit", "-q", "-m", "init")
        git(self.origin, "checkout", "-q", "-b", "release")
        write(self.origin / "skills/gamma/SKILL.md", skill_md("gamma", "Tagged gamma"))
        git(self.origin, "add", "-A")
        git(self.origin, "commit", "-q", "-m", "gamma")
        git(self.origin, "tag", "v1")
        git(self.origin, "checkout", "-q", "main")
        git(self.origin, "branch", "-q", "-D", "release")

    def seed_git_lock(self):
        """Lock entries a prior install would leave, pointing at the file:// fixture."""
        url = f"file://{self.origin}"
        at = "2024-01-02T03:04:05Z"
        lock = json.loads(self.lock.read_text()) if self.lock.exists() else {"version": 1, "skills": {}}
        for name, path, ref in [("alpha", "skills/alpha", "main"), ("beta", "", "main"),
                                ("gamma", "skills/gamma", "v1"), ("deep", "nested", "main"),
                                ("missing-path", "does/not/exist", "main"),
                                ("missing-ref", "", "no-such-ref"), ("traversal", "../..", "main")]:
            lock["skills"][name] = {"installed_at": at, "updated_at": at, "git": {
                "provider": "github", "clone_url": url, "path": path, "ref": ref, "tree_hash": "seed"}}
        self.lock.write_text(json.dumps(lock, indent=2))

    def drop_lock_entries(self, *names):
        lock = json.loads(self.lock.read_text())
        for n in names:
            lock["skills"].pop(n, None)
        self.lock.write_text(json.dumps(lock, indent=2))

    def make_workspace(self):
        if self.ws.exists():
            shutil.rmtree(self.ws)
        (self.ws / "sub").mkdir(parents=True)
        git(self.ws, "init", "-q")
        write(self.ws / ".construct/skills/repo-one/SKILL.md", skill_md("repo-one", "Repo skill <one> & \"quoted\""))
        write(self.ws / ".codex/skills/shared-name/SKILL.md", skill_md("shared-name", "Repo copy wins"))
        write(self.ws / ".claude/skills/dir with space/SKILL.md", skill_md("spaced", "Directory with spaces"))
        write(self.ws / ".claude/skills/multi/SKILL.md", "---\nname: multi\ndescription: >\n  folded\n  description\n---\n")
        write(self.ws / ".github/skills/bad/SKILL.md", skill_md("Bad_Name", "Invalid"))
        write(self.ws / ".github/skills/nofm/SKILL.md", "no frontmatter\n")
        write(self.ws / ".github/skills/nodesc/SKILL.md", "---\nname: nodesc\n---\n")
        write(self.home / ".claude/skills/shared-name/SKILL.md", skill_md("shared-name", "Home copy loses"))
        write(self.home / ".codex/skills/home-codex/SKILL.md", skill_md("home-codex", "Codex home skill"))
        write(self.home / ".config/construct/skills/home-config/SKILL.md", skill_md("home-config", "Home config skill"))

    def reset_skills(self):
        shutil.rmtree(self.skills, ignore_errors=True)
        for p in self.config.glob("skills.lock*"):
            p.unlink()
        self.fx.files = dict(URL_FIXTURES)
        self.make_origin()

    # ---- snapshots ----------------------------------------------------------
    def tree(self):
        out = {}
        if self.skills.exists():
            for p in sorted(self.skills.rglob("*")):
                rel = str(p.relative_to(self.skills))
                mode = oct(p.stat().st_mode & 0o777)
                out[rel] = mode if p.is_dir() else f"{mode} {hashlib.sha256(p.read_bytes()).hexdigest()[:12]}"
        return out

    def state(self):
        lock = self.lock.read_text() if self.lock.exists() else None
        return {
            "tree": self.tree(),
            "lock": TS.sub('"<ts>"', lock) if lock else None,
            "config_files": sorted(p.name for p in self.config.iterdir() if p.name != "config.yaml"),
            "temp_leftovers": sorted(p.name for p in (self.root / "tmp").iterdir()),
        }


def normalize(resp):
    """Mask timestamps (recorded separately) and sort Go's map-ordered lists."""
    body = resp["body"]
    for key in ("installedSkills", "updatedSkills", "skills"):
        if key in body:
            body[key] = sorted(body[key], key=lambda s: s.get("name", ""))
            for s in body[key]:
                for t in ("installedAt", "updatedAt"):
                    if t in s:
                        s[t] = "<ts>"
    return resp


def timestamps(resp):
    body = resp["body"]
    out = {}
    for key in ("installedSkills", "updatedSkills", "skills"):
        for s in body.get(key, []):
            out[s["name"]] = (s.get("installedAt"), s.get("updatedAt"))
    return out


CLI_CASES = [
    ("cli-list", ["skill", "list"]),
    ("cli-list-wide", ["skill", "list", "--wide"]),
    ("cli-list-json", ["skill", "ls", "-o", "json"]),
    ("cli-list-yaml", ["skill", "list", "-o", "yaml"]),
    ("cli-update-manual", ["skill", "update", "manual-invalid"]),
    ("cli-delete-unknown", ["skill", "delete", "nope", "--force"]),
    ("cli-install-invalid", ["skill", "install", "not-a-source"]),
]
CLI_TIME = re.compile(r"\d{4}-\d\d-\d\dT[0-9:.]+(?:Z|[+-]\d\d:\d\d)|\d+ (?:seconds?|minutes?) ago|less than a minute ago")


def scenario(run, impl):
    steps = []
    base = run.fx.base

    def step(name, method, body, snapshot=False):
        resp = run.rpc(method, body)
        entry = {"step": name, "method": method, "request": body, "response": None}
        entry["raw_ts"] = timestamps(resp)
        entry["response"] = normalize(resp)
        if snapshot:
            entry["state"] = run.state()
        steps.append(entry)
        return entry

    def install(name, source, **kw):
        return step(name, "InstallSkill", {"source": source, **kw}, snapshot=True)

    step("list-empty", "ListSkills", {}, snapshot=True)
    install("source-empty", "   ")
    install("source-unrecognized", "just-a-name")
    install("source-scheme", "https://example.com/owner/repo")
    for path in URL_FIXTURES:
        install("url" + path.replace("/SKILL.md", "").replace("/skill.md", "-lower").replace("/", "-"), base + path)
    install("url-404", base + "/nope/SKILL.md")
    install("url-exists", base + "/basic/SKILL.md")
    install("url-force", base + "/basic/SKILL.md", force=True)
    install("url-filter-miss", base + "/crlf/SKILL.md", skillNames=["other"], force=True)
    install("url-filter-hit", base + "/crlf/SKILL.md", skillNames=["url-crlf"], force=True)
    install("url-no-scheme", base.removeprefix("http://") + "/basic/SKILL.md", force=True)
    # Manually placed skills (no lock entry) and broken dirs.
    write(run.skills / "manual-one/SKILL.md", skill_md("manual-one", "Manual"))
    write(run.skills / "manual-invalid/SKILL.md", skill_md("Manual_Invalid", "List parses, does not validate"))
    write(run.skills / "manual-broken/SKILL.md", "no frontmatter")
    (run.skills / "manual-empty").mkdir(exist_ok=True)
    write(run.skills / "stray-file.md", "x")
    step("list-after-url", "ListSkills", {})

    # Git installs through UpdateSkill against the file:// fixture.
    run.seed_git_lock()
    step("update-git-alpha", "UpdateSkill", {"name": "alpha"}, snapshot=True)
    step("update-git-beta-root", "UpdateSkill", {"name": "beta"}, snapshot=True)
    step("update-git-tag", "UpdateSkill", {"name": "gamma"}, snapshot=True)
    step("update-git-recursive", "UpdateSkill", {"name": "deep"}, snapshot=True)
    step("update-git-missing-path", "UpdateSkill", {"name": "missing-path"}, snapshot=True)
    step("update-git-missing-ref", "UpdateSkill", {"name": "missing-ref"}, snapshot=True)
    step("update-git-traversal", "UpdateSkill", {"name": "traversal"}, snapshot=True)
    run.drop_lock_entries("missing-path", "missing-ref", "traversal")
    step("update-manual", "UpdateSkill", {"name": "manual-one"})
    step("update-unknown", "UpdateSkill", {"name": "nope"})
    # Upstream changes, then update one and all.
    write(run.origin / "skills/alpha/SKILL.md", skill_md("alpha", "Alpha v2"))
    (run.origin / "skills/alpha/references/doc.md").unlink()
    write(run.origin / "skills/alpha/references/new.md", "new\n")
    git(run.origin, "add", "-A")
    git(run.origin, "commit", "-q", "-m", "alpha v2")
    run.fx.files["/update/SKILL.md"] = skill_md("url-update", "Version 2")
    before = {k: v for k, v in step("list-before-update", "ListSkills", {})["raw_ts"].items()}
    run.fx.files.pop("/block/SKILL.md")
    step("update-url-404", "UpdateSkill", {"name": "url-block"}, snapshot=True)
    run.fx.files["/block/SKILL.md"] = URL_FIXTURES["/block/SKILL.md"]
    step("update-alpha-v2", "UpdateSkill", {"name": "alpha"}, snapshot=True)
    upd = step("update-all", "UpdateSkill", {}, snapshot=True)
    after = step("list-after-update", "ListSkills", {})["raw_ts"]
    upd["invariants"] = {
        "installed_at_preserved": all(after[n][0] == before[n][0] for n in upd["raw_ts"] if n in before),
        "updated_at_advanced": all(after[n][1] != before[n][1] for n in upd["raw_ts"] if n in before),
    }

    # Deletes.
    step("delete-url", "DeleteSkill", {"name": "url-basic"}, snapshot=True)
    step("delete-again", "DeleteSkill", {"name": "url-basic"})
    step("delete-manual", "DeleteSkill", {"name": "manual-one"}, snapshot=True)
    step("delete-empty-dir", "DeleteSkill", {"name": "manual-empty"}, snapshot=True)
    step("delete-traversal", "DeleteSkill", {"name": "../not-there"}, snapshot=True)
    step("delete-unknown", "DeleteSkill", {"name": "nope"})
    step("list-final", "ListSkills", {}, snapshot=True)

    for name, argv in CLI_CASES:
        steps.append({"step": name, "cli": run.cli_both(argv)})

    if os.environ.get("SKILLS_PARITY_GITHUB") == "1":
        install("github-path", "anthropics/skills/skills/pdf", force=True)
        install("github-filter", "github.com/anthropics/skills", skillNames=["docx", "xlsx"], force=True)
        install("github-missing-repo", "furisto/definitely-not-a-repo-xyz")
    step("list-end", "ListSkills", {})
    return steps


def discovery(run, agent, impl):
    run.make_workspace()
    task = run.cli("--context", "local", "task", "create", "--agent", agent, "--workspace", str(run.ws / "sub"))
    seen = len(run.mock.systems)
    run.cli("--context", "local", "message", "create", task, f"DISCOVER_{impl}")
    for _ in range(300):
        if len(run.mock.systems) > seen:
            break
        time.sleep(0.1)
    else:
        sys.exit(f"{impl}: no model request")
    system = run.mock.systems[seen]
    m = re.search(r"<available_skills>.*?</available_skills>", system, re.S)
    if not m:
        sys.exit(f"{impl}: no <available_skills> block in the system prompt")
    block = m.group(0)
    # Go orders discovered skills by map iteration; compare the header and the sorted entries.
    entries = re.findall(r"  <skill name=.*?\n  </skill>\n", block, re.S)
    header = block.split("  <skill ", 1)[0]
    return {"header": header, "skills": sorted(entries), "raw": block}


def setup_agent(run):
    run.start("go")
    run.cli("context", "add", "local", "--endpoint", run.sock)
    run.cli("context", "use", "local")
    provider = run.cli("modelprovider", "create", "mock", "--type", "anthropic", "--api-key", "fake-key")
    run.stop()
    with sqlite3.connect(run.root / "data/construct/construct.db") as db:
        db.execute("UPDATE model_providers SET url = ? WHERE id = ?", (run.mock.url.rstrip("/"), provider))
    run.start("go")
    model = run.cli("model", "create", "mock-model", "--provider", provider, "--context-window", "128000")
    agent = run.cli("agent", "create", "mock-agent", "--model", model, "--prompt", "Answer.\n{{ .AvailableSkills }}")
    run.stop()
    return agent


def diff(a, b):
    return "\n".join(difflib.unified_diff(json.dumps(a, indent=1, sort_keys=True).splitlines(),
                                          json.dumps(b, indent=1, sort_keys=True).splitlines(),
                                          "go", "rust", lineterm="", n=2))


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--go", required=True)
    p.add_argument("--rust", required=True)
    p.add_argument("--run", required=True)
    args = p.parse_args()
    run = Run(args)
    agent = setup_agent(run)
    results, cross = {}, {}
    for impl, other in [("go", "rust"), ("rust", "go")]:
        run.reset_skills()
        run.start(impl)
        steps = scenario(run, impl)
        steps.append({"step": "discovery", "discovery": discovery(run, agent, impl)})
        run.stop()
        # Cross-read: the other implementation lists this one's final state.
        run.start(other)
        cross[f"{other}-reads-{impl}"] = normalize(run.rpc("ListSkills", {}))
        run.stop()
        results[impl] = steps
        (run.root / f"{impl}-transcript.json").write_text(json.dumps(steps, indent=1))

    report, failures = [], 0
    for g, r in zip(results["go"], results["rust"]):
        name = g["step"]
        gv = {k: v for k, v in g.items() if k not in ("raw_ts",)}
        rv = {k: v for k, v in r.items() if k not in ("raw_ts",)}
        if name == "discovery":
            gv = {k: v for k, v in g["discovery"].items() if k != "raw"}
            rv = {k: v for k, v in r["discovery"].items() if k != "raw"}
        if gv == rv:
            report.append(f"MATCH  {name}")
        elif name in ALLOWED and {k: v for k, v in gv.items() if k != "response"} == \
                {k: v for k, v in rv.items() if k != "response"}:
            report.append(f"ALLOWED {name}: {ALLOWED[name]}\n{diff(gv, rv)}")
        else:
            failures += 1
            report.append(f"DIFF   {name}\n{diff(gv, rv)}")
    for impl in ("go", "rust"):
        for st in results[impl]:
            if "cli" in st:
                ok = st["cli"]["go_cli"] == st["cli"]["rust_cli"]
                failures += not ok
                report.append(f"{'MATCH ' if ok else 'DIFF  '} {st['step']} (Go CLI vs Rust CLI, {impl} daemon)"
                              + ("" if ok else "\n" + diff(st["cli"]["go_cli"], st["cli"]["rust_cli"])))
        own = next(st for st in results[impl] if st["step"] == "list-end")["response"]
        reader = "rust" if impl == "go" else "go"
        key = f"{reader}-reads-{impl}"
        ok = cross[key]["body"] == own["body"]
        failures += not ok
        report.append(f"{'MATCH ' if ok else 'DIFF  '} {key}" + ("" if ok else "\n" + diff(own, cross[key])))
    text = "\n".join(report)
    (run.root / "report.txt").write_text(text + "\n")
    print(text)
    print(f"\n{failures} unexpected difference(s); artifacts in {run.root}")
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
