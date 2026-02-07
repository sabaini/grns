import os
import socket
import subprocess
import time
from pathlib import Path

import pytest


REPO_ROOT = Path(__file__).resolve().parents[1]


def _free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def _wait_for_health(url: str, timeout_seconds: float = 5.0) -> None:
    import urllib.request

    deadline = time.time() + timeout_seconds
    last_error = None
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(url + "/health", timeout=0.25):
                return
        except Exception as exc:  # pragma: no cover - best effort diagnostics
            last_error = exc
            time.sleep(0.05)
    raise RuntimeError(f"server did not become healthy at {url}: {last_error}")


@pytest.fixture(scope="session")
def grns_bin() -> str:
    bin_path = REPO_ROOT / "bin" / "grns"
    if not bin_path.exists():
        subprocess.run(
            ["go", "build", "-o", str(bin_path), "./cmd/grns"],
            cwd=REPO_ROOT,
            check=True,
        )
    return str(bin_path)


@pytest.fixture
def grns_env(tmp_path: Path, grns_bin: str) -> dict[str, str]:
    port = _free_port()
    env = os.environ.copy()
    env.update(
        {
            "GRNS_BIN": grns_bin,
            "GRNS_API_URL": f"http://127.0.0.1:{port}",
            "GRNS_DB": str(tmp_path / "pytest.db"),
            "GRNS_REPO_ROOT": str(REPO_ROOT),
        }
    )
    return env


@pytest.fixture
def running_server(grns_env: dict[str, str], grns_bin: str):
    proc = subprocess.Popen(
        [grns_bin, "srv"],
        cwd=REPO_ROOT,
        env=grns_env,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    try:
        _wait_for_health(grns_env["GRNS_API_URL"], timeout_seconds=8.0)
        yield grns_env
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            proc.kill()

