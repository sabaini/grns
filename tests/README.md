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
```

### Performance Suite Configuration
Use `GRNS_PERF_COUNT` to control the number of tasks created in perf tests (default: 200).

## Test Database
Tests set `GRNS_DB` to a temporary SQLite file under `$BATS_TEST_TMPDIR`.

> Note: SQLite in-memory databases do not persist across separate CLI processes, so we use a temp file for multi-command tests. Once a daemon or single-process mode exists, we can switch to true in-memory testing.

## Seed Data
Seed data lives in `tests/data/*.jsonl`.
Each JSONL entry is translated into a `grns create` call by the test helper.
