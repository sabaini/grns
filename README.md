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
- Project: `.grns.toml` in current workspace

Supported config keys:
- `project_prefix` (default: `gr`)
- `api_url` (default: `http://127.0.0.1:7333`)
- `db_path` (default: `.grns.db` in workspace)

### Environment overrides

- `GRNS_API_URL`
- `GRNS_DB`
- `GRNS_HTTP_TIMEOUT` (client timeout, e.g. `30s` or `30`)

### Security-related environment variables

- `GRNS_API_TOKEN` (Bearer auth for `/v1/*` API routes)
- `GRNS_ADMIN_TOKEN` (required for `/v1/admin/*` when set)
- `GRNS_ALLOW_REMOTE=true` (allow non-loopback server bind)

See `docs/security.md` for details.

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
- `import` accepts JSONL/NDJSON task records
- `import --stream` uses chunked server-side processing for large files

Supported import modes:
- `--dedupe skip|overwrite|error`
- `--orphan-handling allow|skip|strict`

## Build and test

```bash
just build
just test
just test-integration
just lint
```

Run command help for full flags:

```bash
grns --help
grns <command> --help
```
