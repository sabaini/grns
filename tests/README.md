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

# Integration/concurrency pytest suite (optional)
python3 -m pytest -q tests_py

# Pytest performance benchmarks (optional, skipped unless enabled)
GRNS_PYTEST_PERF=1 python3 -m pytest -q -m perf tests_py
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

## Test Database
Tests set `GRNS_DB` to a temporary SQLite file under `$BATS_TEST_TMPDIR`.

> Note: SQLite in-memory databases do not persist across separate CLI processes, so we use a temp file for multi-command tests. Once a daemon or single-process mode exists, we can switch to true in-memory testing.

## Seed Data
Seed data lives in `tests/data/*.jsonl`.
Each JSONL entry is translated into a `grns create` call by the test helper.

## Test Layer Ownership
- **Go unit tests** (`internal/**/_test.go`): service/store logic and edge semantics.
- **BATS** (`tests/*.bats`): CLI surface and user-facing behavior checks.
- **pytest** (`tests_py/`): orchestration, concurrency, and failure-mode integration checks.
