# Grns CLI Blackbox Tests

These tests exercise the CLI as a blackbox using [Bats](https://github.com/bats-core/bats-core).

## Requirements
- `bats` on PATH
- `python3` on PATH (used for JSON parsing in assertions)

## Running
```bash
# Fast/default suite
bats tests

# Performance/stress suite
bats tests/perf

# Snap-in-LXC integration test (optional; auto-discovers grns_*.snap in repo root)
GRNS_RUN_SNAP_LXC_TEST=1 bats tests/cli_snap_lxc.bats

# or pass an explicit snap file
GRNS_RUN_SNAP_LXC_TEST=1 GRNS_SNAP_FILE="$(ls -1 grns_*.snap | head -n 1)" bats tests/cli_snap_lxc.bats

# Integration/concurrency pytest suite (optional)
python3 -m pytest -q tests_py

# Pytest performance benchmarks (optional, skipped unless enabled)
GRNS_PYTEST_PERF=1 python3 -m pytest -q -m perf tests_py

# Mixed-workload stress test (optional, skipped unless enabled)
GRNS_STRESS=1 python3 -m pytest -q -s -m stress tests_py/test_stress_mixed_workload.py
```

### Performance Suite Configuration
BATS perf test knob:
- `GRNS_PERF_COUNT` (default: 200)

Pytest perf knobs:
- `GRNS_PYTEST_PERF=1` (required to run perf tests)
- `GRNS_PERF_COUNT_BATCH` (default: 300)
- `GRNS_PERF_COUNT_IMPORT` (default: 600)
- `GRNS_PERF_COUNT_LIST` (default: 1000)
- `GRNS_PERF_LIST_ROUNDS` (default: 20)
- `GRNS_PERF_MAX_BATCH_CREATE_SEC` (default: 8.0)
- `GRNS_PERF_MAX_IMPORT_STREAM_SEC` (default: 8.0)
- `GRNS_PERF_MAX_LIST_P95_MS` (default: 250.0)

Pytest mixed stress knobs:
- `GRNS_STRESS=1` (required to run stress test)
- `GRNS_STRESS_WORKERS` (default: 16)
- `GRNS_STRESS_DURATION_SEC` (default: 20)
- `GRNS_STRESS_INITIAL_TASKS` (default: 30)
- `GRNS_STRESS_SEED` (default: 1337)
- `GRNS_STRESS_MAX_ERROR_RATE` (default: 0.0)
- `GRNS_STRESS_MAX_P95_MS` (default: disabled)
- `GRNS_STRESS_SUMMARY_PATH` (optional: write JSON summary artifact to this path)

Snap-in-LXC knobs:
- `GRNS_RUN_SNAP_LXC_TEST=1` (required; test is skipped by default)
- `GRNS_SNAP_FILE` (path to `grns_*.snap`; when unset, test auto-discovers one in repo root)
- `GRNS_LXC_IMAGE` (default: `ubuntu:24.04`)

The stress test emits a single-line `STRESS_SUMMARY ...` JSON log at the end of each run.

Compare two stress summaries:
```bash
python3 tests/ci/compare_stress_summaries.py /path/baseline.json /path/candidate.json

# with regression gates (optional)
python3 tests/ci/compare_stress_summaries.py baseline.json candidate.json \
  --fail-on-ops-drop-pct 10 \
  --fail-on-p95-regression-pct 25 \
  --fail-on-error-rate-increase 0.001

# same via just
just compare-stress baseline.json candidate.json
```

## Test Database
Tests set `GRNS_DB` to a temporary SQLite file under `$BATS_TEST_TMPDIR`.

> Note: SQLite in-memory databases do not persist across separate CLI processes, so we use a temp file for multi-command tests. The BATS helpers start a real server process per test and point it at that temp file.

## Seed Data
Seed data lives in `tests/data/*.jsonl`.
Each JSONL entry is translated into a `grns create` call by the test helper.

## HTTP/API test helpers
For tests that directly hit HTTP endpoints, use `tests/helpers_http.bash`:
- `start_grns_server` starts a real grns server and waits for `/health`
- `wait_for_file` waits for synchronization markers in concurrency tests
- `hold_import_limiter_slot` holds one `/v1/projects/gr/import/stream` slot to exercise `429 resource_exhausted`

This keeps API-focused BATS tests consistent and reduces duplicate server bootstrap logic.

## Test Layer Ownership
- **Go unit tests** (`internal/**/_test.go`): service/store logic and edge semantics.
- **BATS** (`tests/*.bats`): CLI surface and user-facing behavior checks.
- **pytest** (`tests_py/`): orchestration, concurrency, and failure-mode integration checks.

## Ordering Assertion Policy
- Prefer **order-insensitive assertions** unless output ordering is part of the contract.
- For JSON arrays in BATS, use sorted extraction helpers (e.g. `json_array_field_sorted`) before comparing expected values.
- Only assert positional order when a command/API explicitly guarantees ordering (e.g., sort key documented).

## Critical File Coverage Guard
A critical-file coverage check runs in CI and can be run locally:

```bash
just test-coverage-critical
```

Default guarded files:
- `internal/server/importer.go`
- `internal/server/list_query.go`
- `internal/server/task_mapper.go`
- `internal/server/task_service.go`
- `internal/store/tasks.go`
