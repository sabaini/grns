import json
import subprocess
import urllib.request
from pathlib import Path


def run_grns(env: dict[str, str], *args: str, check: bool = True) -> subprocess.CompletedProcess:
    proc = subprocess.run(
        [env["GRNS_BIN"], *args],
        text=True,
        capture_output=True,
        env=env,
        cwd=env.get("GRNS_REPO_ROOT"),
    )
    if check and proc.returncode != 0:
        raise AssertionError(
            f"command failed ({proc.returncode}): {' '.join(args)}\nstdout={proc.stdout}\nstderr={proc.stderr}"
        )
    return proc


def json_stdout(proc: subprocess.CompletedProcess):
    return json.loads(proc.stdout)


def api_post(env: dict[str, str], path: str, body: dict) -> dict:
    """POST JSON to the running server and return parsed response."""
    url = env["GRNS_API_URL"] + path
    data = json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read())


def api_get(env: dict[str, str], path: str) -> dict:
    """GET from the running server and return parsed response."""
    url = env["GRNS_API_URL"] + path
    with urllib.request.urlopen(url) as resp:
        return json.loads(resp.read())


def api_patch(env: dict[str, str], path: str, body: dict) -> dict:
    """PATCH JSON to the running server and return parsed response."""
    url = env["GRNS_API_URL"] + path
    data = json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, method="PATCH", headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read())


def run_grns_fail(env: dict[str, str], *args: str) -> subprocess.CompletedProcess:
    """Run CLI expecting failure; return CompletedProcess without raising."""
    return run_grns(env, *args, check=False)


def seed_db(env: dict[str, str], seed_file: str | Path | None = None) -> None:
    """Seed the database by creating tasks from a JSONL file.

    The seed file uses a simplified format (title, type, priority, labels, etc.)
    that is fed to 'grns create' rather than 'grns import'.
    """
    if seed_file is None:
        seed_file = Path(env["GRNS_REPO_ROOT"]) / "tests" / "data" / "seed.jsonl"

    with open(seed_file, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            data = json.loads(line)
            args = ["create", data["title"], "--json"]
            if data.get("type"):
                args += ["-t", data["type"]]
            if data.get("priority") is not None:
                args += ["-p", str(data["priority"])]
            if data.get("labels"):
                args += ["-l", ",".join(data["labels"])]
            if data.get("spec_id"):
                args += ["--spec-id", data["spec_id"]]
            if data.get("description"):
                args += ["-d", data["description"]]
            run_grns(env, *args)
