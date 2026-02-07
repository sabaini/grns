# Go Performance Benchmarks and Budgets

This project now includes Go-native performance coverage for core hot paths in `internal/server`.

## Benchmarks

Run microbenchmarks:

```bash
just bench-go-perf
# or:
go test ./internal/server -run '^$' -bench 'BenchmarkTaskService' -benchmem -count=1
```

Benchmarks currently cover:

- `BenchmarkTaskServiceListFiltered`
- `BenchmarkTaskServiceListSearch`
- `BenchmarkTaskServiceBatchCreate`
- `BenchmarkTaskServiceImportOverwrite`

## Regression budgets (fail-on-regression)

Budget checks are implemented in `TestPerformanceBudgets` and are **gated by env**:

```bash
GRNS_PERF_ENFORCE=1 go test ./internal/server -run '^TestPerformanceBudgets$' -count=1 -v
# or:
just test-go-perf
```

Default per-op budgets:

- list_filtered: `40ms`
- list_search: `45ms`
- batch_create: `110ms`
- import_overwrite: `170ms`

All budget thresholds and loop sizes are configurable via `GRNS_PERF_GO_*` env vars (see `internal/server/perf_budget_test.go`).

## CI

CI includes a required `perf-go` job that:

1. Runs `TestPerformanceBudgets` with `GRNS_PERF_ENFORCE=1`
2. Emits a benchmark snapshot (`-benchtime=1x`) for observability

## Current local baseline snapshot (2026-02-07, dev host)

From `GRNS_PERF_ENFORCE=1 go test ./internal/server -run '^TestPerformanceBudgets$' -v`:

- list_filtered: ~`4.43ms/op`
- list_search: ~`4.25ms/op`
- batch_create: ~`15.88ms/op`
- import_overwrite: ~`121.21ms/op`

Treat this as directional; CI hardware and load can vary.
