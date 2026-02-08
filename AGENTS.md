# AGENTS.md

## Project

Grns is a lightweight CLI issue tracker for agents. Go 1.24, pure-Go SQLite (`modernc.org/sqlite`), Cobra CLI, REST API.

## Architecture

```
CLI (cmd/grns) → HTTP (api.Client) → Server (internal/server) → Store (internal/store) → SQLite
```

- **CLI** auto-spawns a local server process if none is running (`client.go`).
- **Server** uses Go 1.22+ `ServeMux` patterns (`"GET /v1/projects/{project}/tasks/{id}"`). One handler per route+method.
- **TaskService** (`task_service.go`) owns validation and business logic. Handlers must not call store directly.
- **TaskStore** interface (`store/interface.go`) abstracts persistence. The only implementation is SQLite.
- **Migrations** run automatically on `store.Open()`. Add new migrations to `var migrations` in `migrations.go`.

## Build & Test

```
just build                  # build to bin/grns (version via ldflags)
just test                   # go test ./...
just tests-all              # run ALL checks: tidy, fmt, contracts, unit, coverage, integration, py, hypothesis
just test-contracts         # go test ./cmd/grns -run '^TestReadme' (readme contract tests)
just tidy-check             # go mod tidy -diff (verify go.mod/go.sum are tidy)
just fmt                    # gofmt -w on all .go files
just fmt-check              # verify all .go files are formatted
just test-integration       # build + bats tests/
just test-smoke             # build + bats subset (autospawn, create_show, admin_cleanup)
just test-py                # build + uv run pytest tests_py
just test-py-concurrency    # build + pytest concurrency & stress tests
just test-py-stress         # build + GRNS_STRESS=1 pytest stress mixed workload
just test-py-hypothesis     # build + uv run pytest hypothesis tests
just test-py-perf           # build + GRNS_PYTEST_PERF=1 pytest perf-marked tests
just bench-go-perf          # go benchmark: BenchmarkTaskService
just test-go-perf           # GRNS_PERF_ENFORCE=1 go test performance budgets
just test-coverage-critical # ./tests/ci/check_critical_coverage.sh
just compare-stress <b> <c> # compare two stress-test summary files
just lint                   # golangci-lint run ./...
just clean                  # rm -rf bin/
just install                # go install with ldflags
```

Kill stale servers before integration tests: `pkill -f "grns srv"`.

## Code Conventions

- **Imports**: stdlib, blank line, external, blank line, internal.
- **Errors**: return `badRequest(fmt.Errorf(...))` / `notFound(...)` / `conflict(...)` from service layer. Wrap with `%w` when propagating.
- **Tests**: table-driven with `t.Run`. Helpers call `t.Helper()`. Store tests use temp file DBs (not `:memory:`).
- **Naming**: `dbFormatTime`/`dbParseTime` for store-internal time serialization (RFC3339Nano). CLI `formatTime` uses RFC3339.
- **Validation**: IDs match `^[a-z]{2}-[0-9a-z]{4}$`. Statuses, types, labels normalized to lowercase.
- **No CGO**. The sqlite driver is pure Go.
- **commits**: use conventional commits. use prefixes feat: and fix: and if applicable build:, chore:, ci:, docs:, style:, refactor:, perf:, test: as well

## Key Rules

- All task mutations (create, update, close, reopen) go through `TaskService`, never directly to store from handlers.
- `decodeJSON` does **not** use `DisallowUnknownFields` — clients may send newer fields.
- Integration tests are BATS (bash). Helpers in `tests/helpers.bash`, seed data in `tests/data/`.
- Server logs to `$TMPDIR/grns/server.log` when auto-spawned.

## File Hotspots

| File | Role |
|------|------|
| `internal/store/tasks.go` | All SQL queries, CRUD, filters |
| `internal/server/handlers.go` | HTTP handlers, request parsing |
| `internal/server/task_service.go` | Business logic, validation |
| `internal/store/interface.go` | `TaskStore` contract |
| `internal/server/routes.go` | Route registration |
