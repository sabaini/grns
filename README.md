# Grns

Grns is a lightweight CLI task tracker for agents.

Architecture:

```text
CLI (cmd/grns) -> HTTP (internal/api.Client) -> Server (internal/server) -> Store (internal/store) -> SQLite
```

## Quickstart

```bash
# Create a task
grns create "Add auth flow" -t feature -p 1 -l auth --json

# List ready work (tasks with no open blockers)
grns ready --json

# Show details
grns show <id> --json

# Update
grns update <id> --status in_progress --json

# Close / reopen
grns close <id> --json
grns reopen <id> --json
```

## Configuration

Config files (TOML):
- Global: `$HOME/.grns.toml`
- Project: `.grns.toml` in current workspace (**loaded only when `GRNS_TRUST_PROJECT_CONFIG=true`**)

Supported config keys:
- `project_prefix` (default: `gr`)
- `api_url` (default: `http://127.0.0.1:7333`)
- `db_path` (default: `.grns.db` in workspace)

### Environment overrides

- `GRNS_API_URL`
- `GRNS_DB`
- `GRNS_HTTP_TIMEOUT` (client timeout, e.g. `30s` or `30`)
- `GRNS_LOG_LEVEL` (`debug`, `info`, `warn`, `error`; defaults to `info`)
- `GRNS_CONFIG_DIR` (override config file location; uses `$GRNS_CONFIG_DIR/.grns.toml`)
- `GRNS_TRUST_PROJECT_CONFIG=true` (opt in to loading `./.grns.toml`; CLI prints a warning when this trusted project config is used)
- `GRNS_DB_MAX_OPEN_CONNS`, `GRNS_DB_MAX_IDLE_CONNS`, `GRNS_DB_CONN_MAX_LIFETIME` (optional SQLite pool tuning)

### Security-related environment variables

- `GRNS_API_TOKEN` (Bearer auth for `/v1/*` API routes)
- `GRNS_ADMIN_TOKEN` (required for `/v1/admin/*` when set)
- `GRNS_ALLOW_REMOTE=true` (allow non-loopback server bind)

See `docs/security.md` for details.

### API error responses

Error responses are JSON with stable string and numeric codes:

```json
{
  "error": "invalid id",
  "code": "invalid_argument",
  "error_code": 1004
}
```

- `code` remains backward-compatible string classification.
- `error_code` is a systematic numeric code for machine handling.
- Numeric code ranges are documented in `docs/errcode-design.md`.

### CLI operator/user error guidance model

- CLI prints the primary error message first (user-facing cause).
- For startup/autospawn failures, CLI prints actionable hints including API URL checks and the server log path when available.
- For auth/rate-limit/internal API failures, CLI prints targeted hints (token config, retry/load, inspect server logs).
- Server `5xx` responses remain sanitized (`"internal error"`) while detailed diagnostics stay in server logs.

## Commands

```bash
grns create "Title" [flags]
grns show <id> [<id>...]
grns update <id> [<id>...] [flags]
grns list [filters]
grns ready [--limit N]
grns stale [--days N] [--status ...] [--limit N]
grns close <id> [<id>...]
grns reopen <id> [<id>...]

grns dep add <child> <parent> [--type blocks]
grns dep tree <id>

grns label add <id> [<id>...] <label>
grns label remove <id> [<id>...] <label>
grns label list <id>
grns label list-all

grns attach add <task-id> <path> --kind <kind> [--title ...] [--media-type ...] [--label ...]
grns attach add-link <task-id> --kind <kind> [--url <https://...>|--repo-path <path>] [--media-type ...] [--label ...]
grns attach list <task-id>
grns attach show <attachment-id>
grns attach rm <attachment-id>

grns import -i tasks.jsonl [--dry-run] [--dedupe skip|overwrite|error] [--orphan-handling allow|skip|strict]
grns import -i tasks.jsonl --stream   # streaming NDJSON import (recommended for large files)
grns export [-o tasks.jsonl]

grns info
grns admin cleanup --older-than N [--dry-run|--force]
grns migrate [--inspect|--dry-run]
grns config get <key>
grns config set <key> <value>
grns srv
```

## Import / Export

- `export` writes NDJSON (`application/x-ndjson`)
- `export` does **not** support `--json` (to avoid ambiguity with JSON arrays)
- `import` accepts JSONL/NDJSON task records
- `import --stream` uses chunked server-side processing for large files

Supported import modes:
- `--dedupe skip|overwrite|error`
- `--orphan-handling allow|skip|strict`
- `--atomic` (apply each import request/chunk transactionally)

Import failure semantics:
- Default import mode is **structured best-effort** with counters/messages.
- `--atomic` enables transactional apply per request (or per stream chunk).
- Import responses include `apply_mode` and `applied_chunks` checkpoint metadata.
- With `--orphan-handling strict`, orphan deps are counted as errors and affected dependency updates are skipped (task upserts may still apply).

## Build and test

```bash
just build
just test
just test-integration
just test-go-perf          # go perf regression budgets
just bench-go-perf         # go benchmark snapshot
just test-coverage-critical # critical file coverage guard
just lint
```

Global flags:
- `--json`
- `--log-level debug|info|warn|error` (overrides `GRNS_LOG_LEVEL`)

Run command help for full flags:

```bash
grns --help
grns <command> --help
```
