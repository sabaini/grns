# Grns

Grns is a lightweight CLI task tracker for agents.

This project exists because I'm not great at multitasking (few people are), and constantly switching between different clanker ("coding agent") sessions absolutely destroys my thinking process. Grns aims to support a workflow where you spec out a larger block of work, decompose it into individual tasks (likely with clanker help), load the tasks into grns and then have one or more clankers work on tasks, hopefull requireing less handholding. 

## Workflow example (spec → parallel work → closeout)

```bash
# 1) Create an epic and three child tasks
# (capture returned IDs and reuse them in later commands)
grns create "Auth rollout" -t epic -p 1 -l auth --json
# => {"id":"gr-a1b2", ...}

grns create "Add /v1/auth/login endpoint" -t feature -p 1 -l auth --parent gr-a1b2 --json
# => {"id":"gr-c3d4", ...}

grns create "Wire login form in web UI" -t feature -p 2 -l ui --parent gr-a1b2 --json
# => {"id":"gr-e5f6", ...}

grns create "Write auth setup docs" -t task -p 2 -l docs --parent gr-a1b2 --json
# => {"id":"gr-g7h8", ...}

# 2) Make docs depend on both implementation tasks
grns dep add gr-c3d4 gr-g7h8   # API task blocks docs task
grns dep add gr-e5f6 gr-g7h8   # UI task blocks docs task

# 3) Ask agents to pull from the ready queue
grns ready --json

# 4) Agents claim and execute work in parallel
grns update gr-c3d4 --status in_progress --assignee agent-api --json
grns update gr-e5f6 --status in_progress --assignee agent-ui --json
grns close gr-c3d4 --json
grns close gr-e5f6 --json

# 5) Docs task is now unblocked and shows up in ready
grns ready --json

# 6) Finalize remaining work and inspect the full set
grns update gr-g7h8 --status in_progress --assignee agent-docs --json
grns close gr-g7h8 --json
grns show gr-a1b2 gr-c3d4 gr-e5f6 gr-g7h8 --json
```


Architecture:

```text
CLI (cmd/grns) -> HTTP (internal/api.Client) -> Server (internal/server) -> Store (internal/store) -> SQLite
```

## Quickstart

```bash
# Terminal 1: start the server
grns srv

# Terminal 2: run CLI commands

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

## Data Model

### Task IDs

Format: `<prefix>-<4char>` (e.g. `gr-ab12`). The prefix is a 2-letter project identifier (default `gr`, configurable via `project_prefix`). The suffix is 4 random base36 characters. IDs are immutable once created.

### Statuses

`open` (default), `in_progress`, `blocked`, `deferred`, `closed`, `pinned`, `tombstone`

### Types

`bug`, `feature`, `task` (default), `epic`, `chore`

### Priority

Integer 0–4. Default: `2`. Lower is higher priority.

### Labels

ASCII, non-space, case-insensitive (lowercased on store), deduplicated.

### Timestamps

RFC3339 UTC. `updated_at` changes on any mutation. `closed_at` is set on close, cleared on reopen.

### Custom fields

Arbitrary key-value metadata stored as a JSON object:

```bash
grns create "Task" --custom team=platform --custom env=staging
grns create "Task" --custom-json '{"team": "platform", "count": 42}'
grns update <id> --custom team=infra
```

Custom fields are returned in the `custom` object of task responses.

### Dependencies

Type: `blocks` (only supported type). A dependency `child blocks parent` means the parent cannot be "ready" until the child is closed.

```bash
grns dep add <child-id> <parent-id>
grns dep tree <id>
```

### Attachment kinds

`spec`, `diagram`, `artifact`, `diagnostic`, `archive`, `other`

### Git reference relations

`design_doc`, `implements`, `fix_commit`, `closed_by`, `introduced_by`, `related`, or `x-*` (custom extensions).

### Git reference object types

`commit`, `tag`, `branch`, `path`, `blob`, `tree`

For `commit`, `blob`, and `tree`, the object value must be a 40-character lowercase hex SHA. For `path`, it must be a workspace-relative path (no leading `/`, no `..`).

## Server

The CLI connects to a running `grns` server.

For local development/testing, start it manually:

```bash
grns srv
```

In snap installs, `grns.daemon` is enabled by default after install.
If needed, you can (re)start it with:

```bash
sudo snap start grns.daemon
```

- Default bind address: `127.0.0.1:7333`
- Foreground logs (`grns srv`): terminal output
- Daemon logs (service mode, e.g. snap `grns.daemon`): syslog/journald
- Stop local server process: `pkill -f "grns srv"`

The server persists between CLI invocations. To force a clean state (e.g. before integration tests), kill the server process.

### Web UI (preview)

Grns also serves a built-in web UI from the same server process.

```bash
# start API + UI server
grns srv

# then open in your browser
# http://127.0.0.1:7333/
```

Notes:
- UI routes: `/` (index) and `/ui/*` (static assets).
- API stays under `/v1/*`.
- The UI uses hash routes like `#/` and `#/tasks/<id>`.
- If you run with a custom `GRNS_API_URL`, open that server root in the browser.

To enable browser sign-in, provision at least one local admin user:

```bash
printf 'your-strong-password\n' | grns admin user add admin --password-stdin
```

User management helpers:
- `grns admin user list`
- `grns admin user disable <username>` / `grns admin user enable <username>`
- `grns admin user delete <username>`

By default, provisioning admin users enables browser sign-in endpoints but does **not** force auth for `/v1/*`.

To require auth whenever local admin users exist, start the server with:

```bash
GRNS_REQUIRE_AUTH_WITH_USERS=true grns srv
```

Notes:
- Login uses server-side HttpOnly session cookies (`/v1/auth/login`, `/v1/auth/logout`, `/v1/auth/me`).
- For remote/network use, run behind HTTPS so cookies can be marked `Secure`.
- If `GRNS_API_TOKEN` is set, `/v1/*` requires auth and accepts either:
  - `Authorization: Bearer <token>`
  - a valid browser session cookie
- If `GRNS_REQUIRE_AUTH_WITH_USERS=true` and `GRNS_API_TOKEN` is unset, `/v1/*` requires a valid browser session cookie.
- Browser bearer-token fallback is still available via local storage:
  - `localStorage.setItem('grns_api_token', '<token>')`

Current UI scope is read-heavy task list/detail with inline edits and bulk list actions.

## Configuration

Config files (TOML):
- Global: `$HOME/.grns.toml` (fallbacks when missing: `$SNAP_COMMON/.grns.toml` if `$SNAP_COMMON` is set, then `~/snap/grns/common/.grns.toml`)
- Project: `.grns.toml` in current workspace (**loaded only when `GRNS_TRUST_PROJECT_CONFIG=true`**)

Supported config keys:
- `project_prefix` (default: `gr`; used as `{project}` for `/v1/projects/{project}/...` API routes)
- `api_url` (default: `http://127.0.0.1:7333`)
- `db_path` (default: `.grns.db` in workspace)
- `log_level` (default: `debug`; valid values: `debug`, `info`, `warn`, `error`)
- `attachments.max_upload_bytes` (default: `104857600`)
- `attachments.multipart_max_memory` (default: `8388608`)
- `attachments.allowed_media_types` (default: empty)
- `attachments.reject_media_type_mismatch` (default: `true`)
- `attachments.gc_batch_size` (default: `500`)

### Environment overrides

- `GRNS_API_URL`
- `GRNS_DB`
- `GRNS_HTTP_TIMEOUT` (client timeout, e.g. `30s` or `30`)
- `GRNS_LOG_LEVEL` (`debug`, `info`, `warn`, `error`; defaults to `debug`; overrides `log_level`; applies to server/daemon logs)
- `GRNS_CONFIG_DIR` (override config file location; uses `$GRNS_CONFIG_DIR/.grns.toml`)
- `GRNS_TRUST_PROJECT_CONFIG=true` (opt in to loading `./.grns.toml`; CLI prints a warning when this trusted project config is used)
- `GRNS_DB_MAX_OPEN_CONNS`, `GRNS_DB_MAX_IDLE_CONNS`, `GRNS_DB_CONN_MAX_LIFETIME` (optional SQLite pool tuning)
- `GRNS_ATTACH_ALLOWED_MEDIA_TYPES` (comma-separated MIME types to allow for uploads)
- `GRNS_ATTACH_REJECT_MEDIA_TYPE_MISMATCH` (reject uploads where declared MIME differs from sniffed; default `true`)

Log level precedence: `--log-level` > `GRNS_LOG_LEVEL` > `log_level` in config > `debug`.

### Security-related environment variables

- `GRNS_API_TOKEN` (Bearer auth credential for `/v1/*`; when set, auth is required)
- `GRNS_ADMIN_TOKEN` (required for `/v1/admin/*` when set)
- `GRNS_REQUIRE_AUTH_WITH_USERS=true` (require auth for `/v1/*` when at least one enabled local admin user exists)

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
- For connectivity/startup failures, CLI prints actionable hints (verify `GRNS_API_URL`, start `grns srv`, or start `grns.daemon` in snap).
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
grns close <id> [<id>...] [--commit <40hexsha>] [--repo <host/owner/repo>]
grns reopen <id> [<id>...]

grns dep add <child> <parent> [--type blocks]
grns dep tree <id>

grns label add <id> [<id>...] <label>
grns label remove <id> [<id>...] <label>
grns label list <id>
grns label list-all

grns attach add <task-id> <path> --kind <kind> [--title ...] [--media-type ...] [--label ...] [--expires-at <time>]
grns attach add-link <task-id> --kind <kind> [--url <https://...>|--repo-path <path>] [--media-type ...] [--label ...] [--expires-at <time>]
grns attach list <task-id>
grns attach show <attachment-id>
grns attach get <attachment-id> -o <path> [--force]
grns attach rm <attachment-id>

grns git add <task-id> --relation <relation> --type <object_type> --value <object_value> [--repo <host/owner/repo>]
grns git ls <task-id>
grns git rm <git-ref-id>

grns import -i tasks.jsonl [--dry-run] [--dedupe skip|overwrite|error] [--orphan-handling allow|skip|strict]
grns import -i tasks.jsonl --stream   # streaming NDJSON import (recommended for large files)
grns export [-o tasks.jsonl]

grns info
grns admin cleanup --older-than N [--dry-run|--force] [--project <pp>]
grns admin gc-blobs [--dry-run|--apply] [--batch-size N]
grns admin user add <username> --password-stdin
grns admin user list
grns admin user disable <username>
grns admin user enable <username>
grns admin user delete <username>
grns migrate [--inspect|--dry-run]
grns config get <key>
grns config set <key> <value>
grns srv
```

### JSON output behavior notes

- `grns show <id> [<id>...] --json` preserves request order, including duplicate IDs.
- `grns close ... --json` returns `{ "ids": [...] }`; with `--commit`, it also includes `commit` and `annotated`.
- `grns reopen ... --json` returns `{ "ids": [...] }`.
- `grns dep add ... --json` returns `{ "child_id": ..., "parent_id": ..., "type": ... }`.
- `grns label add/remove ... --json` returns the updated label array.
- `grns attach rm ... --json` and `grns git rm ... --json` return `{ "id": ... }`.
- `grns attach add/add-link --expires-at` accepts `RFC3339` or `YYYY-MM-DD`.

### `create` flags

| Flag | Short | Description |
|------|-------|-------------|
| `--id` | | Explicit task ID (must match `<prefix>-<4char>` format) |
| `--type` | `-t` | Task type (`bug`, `feature`, `task`, `epic`, `chore`) |
| `--priority` | `-p` | Priority (0–4) |
| `--description` | `-d` | Task description |
| `--spec-id` | | Specification identifier |
| `--parent` | | Parent task ID |
| `--assignee` | | Assignee name |
| `--notes` | | Free-text notes |
| `--design` | | Design notes |
| `--acceptance` | | Acceptance criteria |
| `--source-repo` | | Source repository (`host/owner/repo`) |
| `--label` | `-l` | Label (repeatable) |
| `--labels` | | Labels (comma-separated) |
| `--deps` | | Dependencies (comma-separated task IDs) |
| `--file` | `-f` | Markdown file for batch create (see below) |
| `--custom` | | Custom field `key=value` (repeatable) |
| `--custom-json` | | Custom fields as JSON object |

### `update` flags

| Flag | Short | Description |
|------|-------|-------------|
| `--title` | | New title |
| `--status` | | New status |
| `--type` | `-t` | New type |
| `--priority` | `-p` | New priority |
| `--description` | `-d` | New description |
| `--spec-id` | | New spec ID |
| `--parent` | | New parent ID |
| `--assignee` | | New assignee |
| `--notes` | | New notes |
| `--design` | | New design |
| `--acceptance` | | New acceptance criteria |
| `--source-repo` | | New source repository |
| `--custom` | | Custom field `key=value` (repeatable) |
| `--custom-json` | | Custom fields as JSON object |

### `list` filters

| Flag | Description |
|------|-------------|
| `--status` | Filter by status (comma-separated for multiple) |
| `--priority` | Exact priority |
| `--priority-min` | Minimum priority (inclusive) |
| `--priority-max` | Maximum priority (inclusive) |
| `--type` | Filter by type |
| `--label` | Tasks with all listed labels (AND) |
| `--label-any` | Tasks with any listed label (OR) |
| `--spec` | Spec ID regex (RE2, case-insensitive) |
| `--parent` | Filter by parent ID |
| `--assignee` | Filter by assignee |
| `--no-assignee` | Unassigned tasks only |
| `--id` | Filter by IDs (comma-separated) |
| `--title-contains` | Title substring match |
| `--desc-contains` | Description substring match |
| `--notes-contains` | Notes substring match |
| `--created-after` | Created after date (RFC3339 or YYYY-MM-DD) |
| `--created-before` | Created before date |
| `--updated-after` | Updated after date |
| `--updated-before` | Updated before date |
| `--closed-after` | Closed after date |
| `--closed-before` | Closed before date |
| `--empty-description` | Tasks with no description |
| `--no-labels` | Tasks with no labels |
| `--search` | Full-text search (FTS5, see below) |
| `--limit` | Max results |
| `--offset` | Skip N results |

### Full-text search (`--search`)

Uses SQLite FTS5 match syntax. Searches across `title`, `description`, and `notes`.

```bash
grns list --search "auth"                    # simple term
grns list --search "auth login"              # both terms (implicit AND)
grns list --search '"exact phrase"'          # phrase match
grns list --search "auth*"                   # prefix match
grns list --search "auth OR oauth"           # boolean OR
grns list --search "auth NOT legacy"         # boolean NOT
```

### Batch create from markdown (`create -f`)

Create multiple tasks from a markdown file with YAML front matter defaults:

```bash
grns create -f tasks.md --json
```

File format:

```markdown
---
type: feature
priority: 1
labels: [auth, backend]
assignee: alice
---
- Add login endpoint
- Add token refresh
- Add logout handler
```

Each list item (`-` or `*`) becomes a separate task. Front matter fields are applied as defaults to all tasks.

Supported front matter keys: `type`, `priority`, `description`, `spec_id`, `status`, `parent_id`, `assignee`, `notes`, `design`, `acceptance_criteria`, `source_repo`, `labels`, `deps`.

## Import / Export

See `docs/import-export.md` for the full guide.

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

## API

See `docs/api.md` for the full REST API reference.

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
- `--log-level debug|info|warn|error` (overrides `GRNS_LOG_LEVEL` and `log_level` config)

Run command help for full flags:

```bash
grns --help
grns <command> --help
```

## Snap packaging (Ubuntu)

Build a local snap:

```bash
snapcraft
```

Install locally:

```bash
sudo snap install --dangerous ./grns_*.snap
```

- CLI command: `grns`
- Daemon service (enabled by default): `grns.daemon`

See `docs/snap.md` for daemon setup and runtime details.

## Troubleshooting

**"Connection refused" or CLI hangs:**
- Check if the server is running: `curl http://127.0.0.1:7333/health`
- Verify `GRNS_API_URL` points to the right address
- Start the server manually: `grns srv`
- In snap installs, if daemon is stopped, restart it: `sudo snap start grns.daemon`
- Check server logs:
  - local foreground (`grns srv`): terminal output (or your redirect target)
  - daemon/service mode (including snap `grns.daemon`): syslog/journald

**Port already in use:**
- Kill stale server: `pkill -f "grns srv"`
- Check what is using the port: `lsof -i :7333`

**Migration errors:**
- Inspect migration state: `grns migrate --inspect`
- Preview migrations: `grns migrate --dry-run`

**Unexpected behavior after upgrade:**
- Kill existing server (it may be running old code): `pkill -f "grns srv"`
- Rebuild: `just build`

## Documentation

| Document | Description |
|----------|-------------|
| [API Reference](docs/api.md) | REST API endpoints, request/response schemas |
| [Import/Export Guide](docs/import-export.md) | JSONL format, dedupe modes, streaming |
| [Design Doc](docs/design.md) | Architecture, data model, design decisions |
| [Attachments Design](docs/attachments.md) | Attachment domain model and storage |
| [Attachments Schema](docs/attachments-schema.md) | Schema, migration, store interfaces |
| [Git References Design](docs/git-refs.md) | Task-to-git linking model |
| [Error Codes Design](docs/errcode-design.md) | Numeric error code catalog |
| [Security](docs/security.md) | Auth, config trust, hardening |
| [Snap Packaging](docs/snap.md) | Build/install snap and default-on daemon service |
| [Messaging Design](docs/messaging.md) | Future multi-agent broker design (not implemented) |
| [Performance](docs/performance-go.md) | Benchmarks and regression budgets |
