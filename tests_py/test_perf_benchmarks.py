import json
import os
import string
import time
from pathlib import Path

import pytest

from tests_py.helpers import json_stdout, run_grns

pytestmark = pytest.mark.perf

if os.getenv("GRNS_PYTEST_PERF", "0") != "1":
    pytest.skip("set GRNS_PYTEST_PERF=1 to run perf benchmarks", allow_module_level=True)


def _env_int(name: str, default: int) -> int:
    raw = os.getenv(name)
    if raw is None or raw.strip() == "":
        return default
    return int(raw)


def _env_float(name: str, default: float) -> float:
    raw = os.getenv(name)
    if raw is None or raw.strip() == "":
        return default
    return float(raw)


def _p95(values_ms: list[float]) -> float:
    if not values_ms:
        return 0.0
    ordered = sorted(values_ms)
    idx = max(0, min(len(ordered) - 1, int(0.95 * (len(ordered) - 1))))
    return ordered[idx]


def _base36(num: int) -> str:
    chars = string.digits + string.ascii_lowercase
    if num == 0:
        return "0"
    out = ""
    n = num
    while n > 0:
        n, rem = divmod(n, 36)
        out = chars[rem] + out
    return out


def _task_id(prefix: str, idx: int) -> str:
    return f"{prefix}-{_base36(idx).zfill(4)[-4:]}"


def _write_import_file(path: Path, count: int, *, spec_prefix: str = "PERF") -> None:
    lines = []
    for i in range(count):
        rec = {
            "id": _task_id("pf", i + 1),
            "title": f"Perf task {i + 1}",
            "status": "open",
            "type": "task",
            "priority": 2,
            "spec_id": f"{spec_prefix}-{i % 20:02d}",
        }
        lines.append(json.dumps(rec, separators=(",", ":")))
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def test_perf_batch_create_markdown(running_server, tmp_path: Path):
    env = running_server
    count = _env_int("GRNS_PERF_COUNT_BATCH", 300)
    max_seconds = _env_float("GRNS_PERF_MAX_BATCH_CREATE_SEC", 8.0)

    markdown_file = tmp_path / "perf_batch.md"
    with markdown_file.open("w", encoding="utf-8") as handle:
        handle.write("---\n")
        handle.write("type: task\n")
        handle.write("priority: 2\n")
        handle.write("labels: [perf]\n")
        handle.write("---\n")
        for i in range(count):
            handle.write(f"- Perf markdown task {i + 1}\n")

    started = time.perf_counter()
    proc = run_grns(env, "create", "-f", str(markdown_file), "--json")
    elapsed = time.perf_counter() - started

    created = json_stdout(proc)
    assert len(created) == count
    assert elapsed <= max_seconds, f"batch create took {elapsed:.3f}s > budget {max_seconds:.3f}s"


def test_perf_stream_import_throughput(running_server, tmp_path: Path):
    env = running_server
    count = _env_int("GRNS_PERF_COUNT_IMPORT", 600)
    max_seconds = _env_float("GRNS_PERF_MAX_IMPORT_STREAM_SEC", 8.0)

    import_file = tmp_path / "perf_import.jsonl"
    _write_import_file(import_file, count)

    started = time.perf_counter()
    proc = run_grns(env, "import", "-i", str(import_file), "--stream", "--json")
    elapsed = time.perf_counter() - started

    payload = json_stdout(proc)
    assert int(payload["created"]) == count
    assert elapsed <= max_seconds, f"stream import took {elapsed:.3f}s > budget {max_seconds:.3f}s"


def test_perf_list_spec_regex_p95_latency(running_server, tmp_path: Path):
    env = running_server
    count = _env_int("GRNS_PERF_COUNT_LIST", 1000)
    rounds = _env_int("GRNS_PERF_LIST_ROUNDS", 20)
    max_p95_ms = _env_float("GRNS_PERF_MAX_LIST_P95_MS", 250.0)

    import_file = tmp_path / "perf_list_seed.jsonl"
    _write_import_file(import_file, count, spec_prefix="SPEC")
    import_result = json_stdout(run_grns(env, "import", "-i", str(import_file), "--stream", "--json"))
    assert int(import_result["created"]) == count

    latencies_ms = []
    for _ in range(rounds):
        started = time.perf_counter()
        proc = run_grns(
            env,
            "list",
            "--spec",
            "^SPEC-0[0-9]$",
            "--limit",
            "50",
            "--json",
        )
        elapsed_ms = (time.perf_counter() - started) * 1000.0
        latencies_ms.append(elapsed_ms)

        listed = json_stdout(proc)
        assert 0 < len(listed) <= 50

    p95_ms = _p95(latencies_ms)
    assert p95_ms <= max_p95_ms, f"list p95 {p95_ms:.2f}ms > budget {max_p95_ms:.2f}ms"
