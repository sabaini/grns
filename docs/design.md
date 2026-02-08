# Grns Design Doc (Draft)

## Summary
Grns is a lightweight issue tracker and memory system for agents, focused on durable tasks, dependencies, and recency. It exposes a CLI-first interface with machine-readable I/O and uses a REST API from the start: the CLI talks to a local server for single-user mode and the same server scales to multi-user deployments.

Related docs:
- [API Reference](api.md)
- [Import/Export Guide](import-export.md)
- [Attachments Design](attachments.md)
- [Messaging Design](messaging.md) (not yet implemented)
- [Task ↔ Git References Design](git-refs.md)

## Problem Statement
We need a fast, CLI-oriented task tracker that supports dependency modeling and recency queries while remaining simple enough for local use, yet extensible enough for multi-user collaboration.

## Goals
- Support **single-user** installs with an embedded datastore.
- Support **multi-user** deployments via a client–server architecture.
- Provide **core task properties** plus **user-configurable fields** (implemented: `custom` JSON column).
- Deliver **fast** operations for common workflows (create/update/ready/stale queries).
- Provide **machine-readable I/O** (JSON; YAML/TOML deferred).
- Work on **Linux** and **macOS**.

## MVP Scope
- Core commands: `create`, `update`, `show`, `list`, `ready`, `stale`, `close`, `reopen`, `dep add`, `label add/remove/list`.
- REST API is used from day one: CLI → local `grns` server → embedded SQLite.
- `create -f` accepts markdown with front matter and list items to emit multiple issues.
- `--deps` supports multiple dependencies (comma-separated) with `blocks` as the default type.
- `list --spec` uses regex matching.

## Non-Goals (Initial)
- Building a full web UI (CLI-first).
- Complex workflow engines or heavy project management features.
- Windows support in the initial release.
- Advanced maintenance features (duplicates/merge, admin cleanup, daemon management, migrations) are post-MVP.

## Requirements
### Functional
- Create, update, and show tasks.
- Track dependencies (blocks/unblocks/parent-child).
- Query “ready” tasks (no open blockers).
- Query “stale” tasks by last-updated threshold.
- Output results in JSON. Full-database import/export via NDJSON (implemented).
- Custom per-task fields via `custom` JSON column (implemented).

### Non-Functional
- Scale to ~100k tasks.
- Read latency target < 150ms; write latency target < 400ms.
- Fast local operations for typical queries.
- Compatible with Linux and macOS.
- Predictable CLI UX with stable output schema.

## Data Model
### Core Task Fields (initial)
- `id` (stable unique identifier)
- `title`
- `status` (e.g., open, in_progress, blocked, deferred, closed, pinned, tombstone)
- `type` (e.g., bug, feature, task, epic, chore)
- `priority` (0–4)
- `labels` (case-insensitive ASCII list)
- `description` (optional)
- `spec_id` (optional)
- `parent_id` (optional)
- `created_at`, `updated_at`, `closed_at` (optional)
- `dependencies`: references to other tasks

### ID Format
- Canonical IDs are immutable and non-hierarchical: `<pp>-<hash>` where `pp` is a two-letter project prefix (configured per workspace; default `gr`) and `hash` is a short, lowercase base36 string (4 chars).
- Parent/child relationships are represented via `parent_id` and dependencies, not via ID structure.
- Explicit IDs are allowed via `--id` but must match the format.

### Defaults
- `status`: `open`
- `priority`: `2`
- `type`: `task`
- `labels`: empty list

### Timestamps
- RFC3339 UTC.
- `updated_at` changes on any mutation.
- `closed_at` is set on close and cleared on reopen.

### Custom Fields (Implemented)
- A `custom` JSON column per task stores user-defined key-value fields.
- Free-form mode is the current implementation; schema-validated fields may follow.

## Architecture
### Single-User Mode (Embedded)
- CLI communicates with a local `grns` server over REST (default `http://127.0.0.1:7333`).
- CLI auto-spawns the local server if not running.
- Server uses an embedded SQLite datastore (WAL mode).
- No external services required.

### Multi-User Mode (Client–Server)
- CLI connects to a shared `grns` server over REST.
- Server owns datastore and concurrency control.
- Storage backend: SQLite (MVP), PostgreSQL (planned).
- Assume trusted users initially (no auth/ACLs); revisit later.
- Must meet latency targets (reads < 150ms, writes < 400ms).
- Supports multiple concurrent clients.

## Storage & Serialization
- **Internal storage (recommended):** SQLite in WAL mode with JSON1 for `custom` fields and FTS5 for text search (title/description/notes).
- Tables: `tasks`, `task_labels`, `task_deps`; indexed by `status`, `priority`, `updated_at`, `spec_id`, `parent_id`, plus join indexes for labels/deps.
- Regex filters (e.g., `list --spec`) are applied in the service layer using Go's `regexp` (RE2), case-insensitive by default, unless SQLite REGEXP is enabled.
- **External I/O:** JSON for CLI output; NDJSON import/export (implemented).
- **Attachments:** hybrid model—metadata in SQLite, blob bytes in managed blob storage; see [Attachments Design](attachments.md).

### Storage Rationale
- **Embedded + server-ready:** SQLite is zero-config for single-user installs and works unchanged in server mode.
- **Performance at target scale:** WAL mode supports concurrent readers/writers and keeps latency low for ~100k items.
- **Extensibility:** JSON1 enables flexible `custom` fields without schema churn; FTS5 provides fast text search.
- **Portability:** Cross-platform, single-file storage simplifies backup, sync, and migrations.
- **Tradeoffs:** Regex filters may run in the service layer; high write concurrency is limited, but acceptable for MVP scale.

## MVP REST API Surface (v1)
- Base path: `/v1` (JSON only, no auth in MVP).
- `POST /tasks` – create a task (supports optional `id`, `parent`, `deps`, `labels`, `custom`).
- `POST /tasks/batch` – batch create (used by `create -f` for markdown imports), applied transactionally (all-or-nothing).
- `GET /tasks/{id}` – show task.
- `POST /tasks/get` – bulk show by IDs (returns full task payloads, used by multi-id `show`).
- `PATCH /tasks/{id}` – update fields (partial).
- `POST /tasks/close` – close one or more tasks (`ids[]`, `reason`).
- `POST /tasks/reopen` – reopen tasks (`ids[]`, `reason`).
- `GET /tasks` – list/filter (all CLI filters, regex for `spec`, pagination via `limit`/`offset`).
- `GET /tasks/ready` – ready queue (no open blockers).
- `GET /tasks/stale` – stale query (`days`, optional `status`, `limit`).
- `POST /deps` – add dependency (`child_id`, `parent_id`, `type`, default `blocks`).
- `POST /tasks/{id}/labels` – add labels (`labels[]`).
- `DELETE /tasks/{id}/labels` – remove labels (`labels[]`).
- `GET /tasks/{id}/labels` – list labels for task.
- `GET /labels` – list all labels.

## CLI UX
- Commands: `create`, `update`, `show`, `list`, `ready`, `stale`, `close`, `reopen`, `dep add`, `label add/remove/list`.
- Output flags: `--json` (YAML/TOML post‑MVP).
- Default output: human-readable with stable formatting.
- CLI auto-spawns the local server when needed.
- `create -f` accepts markdown input (front matter + list items) for batch issue creation.
- `--deps` supports multiple dependencies (comma-separated) with `blocks` as the default type.
- `list --spec` uses regex matching (case-insensitive RE2).
- `GRNS_DB` env var overrides the database path.
- `GRNS_API_URL` env var overrides the server base URL (default `http://127.0.0.1:7333`).

## Config
- Global config: `$HOME/.grns.toml`
- Project config: `.grns.toml` in the workspace root (overrides global).

### Config Keys (MVP)
- `project_prefix` (string, two letters, default `gr`)
- `api_url` (string, default `http://127.0.0.1:7333`)
- `db_path` (string, default derived from workspace)

## Performance Considerations
- Index by status, updated_at, and dependency relationships.
- Avoid full scans for ready/stale queries.

## Extensibility
- **Custom fields:** `custom` JSON column (implemented, free-form). Schema registry for validated per‑team fields is future work.
- **Storage backends:** `Store` interface to support SQLite (current) and PostgreSQL later.
- **Lifecycle hooks:** pre/post create/update/close events (exec or webhook) — not yet implemented.
- **Output formats:** JSON (current). YAML/TOML deferred.
- **Query plugins:** registry for custom filters — not yet implemented.
- **CLI plugins:** external `grns-<cmd>` binaries discovered on PATH — not yet implemented.
- **Auth middleware:** Bearer token and admin token auth (implemented). Pluggable ACL strategies for multi‑user servers are future work.
- **Import/Export:** NDJSON import/export with streaming mode (implemented).

## Query Semantics
- **Ready**: tasks with no blockers in statuses `open`, `in_progress`, `blocked`, `deferred`, or `pinned`.
- **Stale**: default `--days 30`; uses `updated_at` and excludes `closed`/`tombstone` by default (unless `--status` is provided).

## MVP Implementation Plan
### Stack
- Language: Go
- CLI: Cobra
- Server: Go HTTP (net/http)
- Front matter: YAML
- Storage: SQLite (WAL mode, JSON1, FTS5 if needed)
- Config: `.grns.toml` per project, `$HOME/.grns.toml` global

### Storage Schema (MVP)
- `tasks`: `id`, `title`, `status`, `type`, `priority`, `description`, `spec_id`, `parent_id`, `created_at`, `updated_at`, `closed_at`, `custom` (JSON).
- `task_labels`: `task_id`, `label`.
- `task_deps`: `child_id`, `parent_id`, `type`.
- `custom` stores user-defined JSON fields (exposed via CLI `--custom` and API).

### Indexes
- `tasks(status, updated_at)`
- `tasks(spec_id)`
- `tasks(parent_id)`
- `task_labels(label)`
- `task_deps(child_id)`
- `task_deps(parent_id)`

### Boot & Migrations
- Set WAL mode on open.
- Versioned migration framework with `schema_migrations` table (implemented, currently at v6).

### CLI Implementation Order
1. `create` / `show`
2. `update` / `close` / `reopen`
3. `list` (filters: status/priority/type/spec regex/labels)
4. `dep add`
5. `ready` (no open blockers)
6. `stale` (updated_at threshold)
7. `label add/remove/list`
8. `create -f` (markdown front matter + list items)

### Output Formats
- JSON first; YAML/TOML later.

### Markdown Import (`create -f`)
- Parse front matter into default fields. Allowed keys: `type`, `priority`, `labels`, `spec_id`, `description`, `status`, `parent_id`, `deps`.
- Each list item becomes a task title (inherits front matter fields).

### Blackbox Tests
- Wire `GRNS_DB` env var to DB path.
- Ensure CLI output matches test expectations:
  - `--json` returns objects/arrays as in tests.
  - `label list --json` returns array of strings.
  - `create -f` returns array of created tasks.

## Post‑MVP Questions
1. What schema format should define validated custom fields (e.g., JSON Schema), and how strict should validation be?
2. Should free-form custom fields remain as an opt-in escape hatch, or be disallowed?

