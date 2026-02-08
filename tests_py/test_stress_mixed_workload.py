from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed
import json
import os
from pathlib import Path
import random
import threading
import time
import urllib.error
import urllib.request

import pytest

from tests_py.helpers import scoped_api_path

pytestmark = pytest.mark.stress

if os.getenv("GRNS_STRESS", "0") != "1":
    pytest.skip("set GRNS_STRESS=1 to run mixed workload stress test", allow_module_level=True)


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


def _percentile(values_ms: list[float], q: float) -> float:
    if not values_ms:
        return 0.0
    ordered = sorted(values_ms)
    idx = max(0, min(len(ordered) - 1, int(q * (len(ordered) - 1))))
    return ordered[idx]


def _p95(values_ms: list[float]) -> float:
    return _percentile(values_ms, 0.95)


def _api_json_request(env: dict[str, str], method: str, path: str, body: dict | None = None):
    url = env["GRNS_API_URL"] + scoped_api_path(env, path)
    data = None
    headers = {}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"

    req = urllib.request.Request(url, data=data, method=method, headers=headers)
    with urllib.request.urlopen(req, timeout=5.0) as resp:
        return json.loads(resp.read())


def _list_all_by_label(env: dict[str, str], label: str, limit: int = 250) -> list[dict]:
    items: list[dict] = []
    offset = 0
    while True:
        chunk = _api_json_request(
            env,
            "GET",
            f"/v1/projects/gr/tasks?label={label}&limit={limit}&offset={offset}",
        )
        if not chunk:
            break
        items.extend(chunk)
        if len(chunk) < limit:
            break
        offset += limit
    return items


def _build_summary(
    *,
    run_label: str,
    workers: int,
    duration_configured_sec: int,
    duration_actual_sec: float,
    seed: int,
    op_counts: dict[str, int],
    op_latencies_ms: dict[str, list[float]],
    op_errors: list[tuple[str, str]],
) -> dict:
    total_ops = sum(op_counts.values())
    error_count = len(op_errors)
    error_rate = (error_count / float(total_ops)) if total_ops > 0 else 0.0

    op_stats: dict[str, dict] = {}
    for op in sorted(op_counts.keys()):
        values = op_latencies_ms.get(op, [])
        op_stats[op] = {
            "count": op_counts[op],
            "p50_ms": round(_percentile(values, 0.50), 3),
            "p95_ms": round(_percentile(values, 0.95), 3),
            "max_ms": round(max(values) if values else 0.0, 3),
        }

    lock_error_count = 0
    for _op, err in op_errors:
        lowered = err.lower()
        if "database is locked" in lowered or "busy" in lowered:
            lock_error_count += 1

    return {
        "run_label": run_label,
        "seed": seed,
        "workers": workers,
        "duration_sec_configured": duration_configured_sec,
        "duration_sec_actual": round(duration_actual_sec, 3),
        "total_ops": total_ops,
        "ops_per_sec": round((total_ops / duration_actual_sec) if duration_actual_sec > 0 else 0.0, 3),
        "error_count": error_count,
        "error_rate": round(error_rate, 6),
        "lock_error_count": lock_error_count,
        "op_counts": dict(sorted(op_counts.items())),
        "op_stats": op_stats,
        "error_samples": [{"op": op, "error": err} for op, err in op_errors[:12]],
    }


def _emit_summary(summary: dict) -> None:
    print("STRESS_SUMMARY " + json.dumps(summary, sort_keys=True))

    summary_path = os.getenv("GRNS_STRESS_SUMMARY_PATH", "").strip()
    if not summary_path:
        return

    out = Path(summary_path)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def test_stress_mixed_workload_invariants(running_server):
    env = running_server

    workers = _env_int("GRNS_STRESS_WORKERS", 16)
    duration_sec = _env_int("GRNS_STRESS_DURATION_SEC", 20)
    initial_tasks = _env_int("GRNS_STRESS_INITIAL_TASKS", 30)
    seed = _env_int("GRNS_STRESS_SEED", 1337)
    max_error_rate = _env_float("GRNS_STRESS_MAX_ERROR_RATE", 0.0)
    max_p95_ms = _env_float("GRNS_STRESS_MAX_P95_MS", 0.0)

    run_label = f"stress-{seed}-{time.time_ns()}"

    task_ids: list[str] = []
    mutex = threading.Lock()
    op_counts: dict[str, int] = defaultdict(int)
    op_latencies_ms: dict[str, list[float]] = defaultdict(list)
    op_errors: list[tuple[str, str]] = []

    run_started = time.perf_counter()

    # Seed a baseline pool so update/toggle/label ops have IDs immediately.
    for i in range(initial_tasks):
        created = _api_json_request(
            env,
            "POST",
            "/v1/projects/gr/tasks",
            {
                "title": f"Stress seed {i}",
                "labels": [run_label, "stress"],
                "priority": i % 5,
            },
        )
        task_ids.append(created["id"])

    deadline = time.time() + duration_sec

    def pick_id(rng: random.Random) -> str:
        with mutex:
            return rng.choice(task_ids)

    def record(op: str, started: float, err: str | None = None):
        elapsed_ms = (time.perf_counter() - started) * 1000.0
        with mutex:
            op_counts[op] += 1
            op_latencies_ms[op].append(elapsed_ms)
            if err is not None:
                op_errors.append((op, err))

    ops = ["create", "update", "list", "label", "toggle"]
    weights = [22, 26, 24, 14, 14]

    def worker(worker_idx: int):
        rng = random.Random(seed + worker_idx * 7919)
        n = 0

        while time.time() < deadline:
            op = rng.choices(ops, weights=weights, k=1)[0]
            started = time.perf_counter()
            try:
                if op == "create":
                    created = _api_json_request(
                        env,
                        "POST",
                        "/v1/projects/gr/tasks",
                        {
                            "title": f"Stress task {worker_idx}-{n}",
                            "labels": [run_label, "stress"],
                            "priority": rng.randrange(0, 5),
                        },
                    )
                    with mutex:
                        task_ids.append(created["id"])

                elif op == "update":
                    task_id = pick_id(rng)
                    if rng.random() < 0.5:
                        payload = {"priority": rng.randrange(0, 5)}
                    else:
                        payload = {"description": f"desc-{worker_idx}-{n}"}
                    _api_json_request(env, "PATCH", f"/v1/projects/gr/tasks/{task_id}", payload)

                elif op == "list":
                    _api_json_request(env, "GET", f"/v1/projects/gr/tasks?label={run_label}&limit=80")

                elif op == "label":
                    task_id = pick_id(rng)
                    label = f"wk-{worker_idx % 4}"
                    if rng.random() < 0.65:
                        _api_json_request(env, "POST", f"/v1/projects/gr/tasks/{task_id}/labels", {"labels": [label]})
                    else:
                        _api_json_request(env, "DELETE", f"/v1/projects/gr/tasks/{task_id}/labels", {"labels": [label]})

                else:  # toggle
                    task_id = pick_id(rng)
                    path = "/v1/projects/gr/tasks/close" if rng.random() < 0.5 else "/v1/projects/gr/tasks/reopen"
                    _api_json_request(env, "POST", path, {"ids": [task_id]})

            except urllib.error.HTTPError as exc:
                body = exc.read().decode("utf-8", errors="replace")
                record(op, started, f"http {exc.code}: {body}")
            except Exception as exc:  # pragma: no cover - diagnostic path
                record(op, started, str(exc))
            else:
                record(op, started)

            n += 1

    with ThreadPoolExecutor(max_workers=workers) as pool:
        futures = [pool.submit(worker, i) for i in range(workers)]
        for future in as_completed(futures):
            future.result()

    total_ops = sum(op_counts.values())
    duration_actual_sec = time.perf_counter() - run_started

    summary = _build_summary(
        run_label=run_label,
        workers=workers,
        duration_configured_sec=duration_sec,
        duration_actual_sec=duration_actual_sec,
        seed=seed,
        op_counts=dict(op_counts),
        op_latencies_ms=dict(op_latencies_ms),
        op_errors=list(op_errors),
    )
    _emit_summary(summary)

    assert total_ops > 0

    error_rate = len(op_errors) / float(total_ops)
    if max_error_rate <= 0.0:
        assert op_errors == [], f"unexpected errors ({len(op_errors)}): {op_errors[:8]}"
    else:
        assert error_rate <= max_error_rate, (
            f"error_rate={error_rate:.4f} > max_error_rate={max_error_rate:.4f}; "
            f"sample={op_errors[:8]}"
        )

    if max_p95_ms > 0.0:
        for op, values in op_latencies_ms.items():
            assert _p95(values) <= max_p95_ms, (
                f"{op} p95={_p95(values):.2f}ms > budget={max_p95_ms:.2f}ms"
            )

    final_tasks = _list_all_by_label(env, run_label)
    final_ids = [task["id"] for task in final_tasks]

    with mutex:
        expected_ids = set(task_ids)

    assert len(final_ids) == len(set(final_ids))
    assert set(final_ids) == expected_ids

    for task in final_tasks:
        status = task["status"]
        closed_at = task.get("closed_at")
        if status == "closed":
            assert closed_at is not None
        else:
            assert closed_at is None

        labels = task.get("labels", [])
        assert labels == sorted(set(labels))
