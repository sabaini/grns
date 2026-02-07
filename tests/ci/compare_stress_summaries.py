#!/usr/bin/env python3
"""Compare two GRNS stress summary JSON files.

Example:
    python3 tests/ci/compare_stress_summaries.py \
      /tmp/stress-old.json /tmp/stress-new.json

Optional regression gates:
    --fail-on-ops-drop-pct 10
    --fail-on-p95-regression-pct 25
    --fail-on-error-rate-increase 0.001
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


def _load(path: str) -> dict[str, Any]:
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise SystemExit(f"invalid summary payload (expected object): {path}")
    return data


def _pct_delta(old: float, new: float) -> float:
    if old == 0:
        return 0.0 if new == 0 else 100.0
    return ((new - old) / old) * 100.0


def _fmt_delta(value: float, unit: str = "") -> str:
    sign = "+" if value > 0 else ""
    return f"{sign}{value:.3f}{unit}"


def _as_float(data: dict[str, Any], key: str) -> float:
    value = data.get(key, 0.0)
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def _op_p95(summary: dict[str, Any], op: str) -> float:
    stats = summary.get("op_stats", {})
    if not isinstance(stats, dict):
        return 0.0
    op_stats = stats.get(op, {})
    if not isinstance(op_stats, dict):
        return 0.0
    return float(op_stats.get("p95_ms", 0.0) or 0.0)


def _op_count(summary: dict[str, Any], op: str) -> int:
    counts = summary.get("op_counts", {})
    if not isinstance(counts, dict):
        return 0
    value = counts.get(op, 0)
    try:
        return int(value)
    except (TypeError, ValueError):
        return 0


def main() -> None:
    parser = argparse.ArgumentParser(description="Compare two stress summary JSON files")
    parser.add_argument("baseline", help="Path to baseline summary JSON")
    parser.add_argument("candidate", help="Path to candidate summary JSON")
    parser.add_argument("--fail-on-ops-drop-pct", type=float, default=None)
    parser.add_argument("--fail-on-p95-regression-pct", type=float, default=None)
    parser.add_argument("--fail-on-error-rate-increase", type=float, default=None)
    args = parser.parse_args()

    baseline = _load(args.baseline)
    candidate = _load(args.candidate)

    base_ops_sec = _as_float(baseline, "ops_per_sec")
    cand_ops_sec = _as_float(candidate, "ops_per_sec")
    base_total_ops = _as_float(baseline, "total_ops")
    cand_total_ops = _as_float(candidate, "total_ops")
    base_error_rate = _as_float(baseline, "error_rate")
    cand_error_rate = _as_float(candidate, "error_rate")
    base_lock_errors = _as_float(baseline, "lock_error_count")
    cand_lock_errors = _as_float(candidate, "lock_error_count")

    print("Stress summary comparison")
    print(f"- baseline:  {args.baseline} (run_label={baseline.get('run_label', '')})")
    print(f"- candidate: {args.candidate} (run_label={candidate.get('run_label', '')})")
    print()

    ops_sec_delta = cand_ops_sec - base_ops_sec
    ops_sec_delta_pct = _pct_delta(base_ops_sec, cand_ops_sec)
    print("Overall:")
    print(
        f"  ops/sec        {base_ops_sec:.3f} -> {cand_ops_sec:.3f} "
        f"({_fmt_delta(ops_sec_delta)}, {_fmt_delta(ops_sec_delta_pct, '%')})"
    )

    total_ops_delta = cand_total_ops - base_total_ops
    total_ops_delta_pct = _pct_delta(base_total_ops, cand_total_ops)
    print(
        f"  total_ops      {base_total_ops:.0f} -> {cand_total_ops:.0f} "
        f"({_fmt_delta(total_ops_delta)}, {_fmt_delta(total_ops_delta_pct, '%')})"
    )

    error_rate_delta = cand_error_rate - base_error_rate
    print(
        f"  error_rate     {base_error_rate:.6f} -> {cand_error_rate:.6f} "
        f"({_fmt_delta(error_rate_delta)})"
    )

    lock_err_delta = cand_lock_errors - base_lock_errors
    print(
        f"  lock_errors    {int(base_lock_errors)} -> {int(cand_lock_errors)} "
        f"({_fmt_delta(lock_err_delta)})"
    )

    ops = sorted(
        set((baseline.get("op_counts") or {}).keys())
        | set((candidate.get("op_counts") or {}).keys())
    )

    print("\nPer-op p95 latency (ms):")
    for op in ops:
        base_p95 = _op_p95(baseline, op)
        cand_p95 = _op_p95(candidate, op)
        delta = cand_p95 - base_p95
        delta_pct = _pct_delta(base_p95, cand_p95)

        base_count = _op_count(baseline, op)
        cand_count = _op_count(candidate, op)
        print(
            f"  {op:8s} {base_p95:8.3f} -> {cand_p95:8.3f} "
            f"({_fmt_delta(delta, 'ms')}, {_fmt_delta(delta_pct, '%')}) "
            f"counts {base_count}->{cand_count}"
        )

    failures: list[str] = []

    if args.fail_on_ops_drop_pct is not None:
        if ops_sec_delta_pct < -abs(args.fail_on_ops_drop_pct):
            failures.append(
                f"ops/sec dropped {ops_sec_delta_pct:.3f}% "
                f"(threshold {-abs(args.fail_on_ops_drop_pct):.3f}%)"
            )

    if args.fail_on_error_rate_increase is not None:
        threshold = abs(args.fail_on_error_rate_increase)
        if error_rate_delta > threshold:
            failures.append(
                f"error_rate increased by {error_rate_delta:.6f} "
                f"(threshold {threshold:.6f})"
            )

    if args.fail_on_p95_regression_pct is not None:
        threshold = abs(args.fail_on_p95_regression_pct)
        for op in ops:
            base_p95 = _op_p95(baseline, op)
            cand_p95 = _op_p95(candidate, op)
            delta_pct = _pct_delta(base_p95, cand_p95)
            if base_p95 > 0 and delta_pct > threshold:
                failures.append(
                    f"{op} p95 regressed {delta_pct:.3f}% "
                    f"(threshold {threshold:.3f}%)"
                )

    if failures:
        print("\nRegression gate failures:")
        for failure in failures:
            print(f"- {failure}")
        raise SystemExit(1)


if __name__ == "__main__":
    main()
