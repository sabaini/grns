# Import / Export Guide

Grns supports NDJSON (newline-delimited JSON) for bulk data exchange.

## Export

```bash
# Export to stdout
grns export

# Export to file
grns export -o tasks.jsonl
```

Output is NDJSON — one JSON object per line. Each line is a full task record with labels and dependencies. The `--json` flag is not supported (export always emits NDJSON).

### Export record format

Each line is a `TaskResponse`:

```json
{"project":"gr","id":"gr-ab12","title":"Add auth flow","status":"open","type":"feature","priority":1,"description":"","spec_id":"","parent_id":"","assignee":"alice","notes":"","design":"","acceptance_criteria":"","source_repo":"","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T12:00:00Z","labels":["auth","backend"],"deps":[{"parent_id":"gr-0001","type":"blocks"}]}
```

Fields included: `project`, `id`, `title`, `status`, `type`, `priority`, `description`, `spec_id`, `parent_id`, `assignee`, `notes`, `design`, `acceptance_criteria`, `source_repo`, `custom`, `created_at`, `updated_at`, `closed_at`, `labels`, `deps`.

## Import

```bash
# Basic import
grns import -i tasks.jsonl

# Preview without changes
grns import -i tasks.jsonl --dry-run

# Overwrite existing tasks
grns import -i tasks.jsonl --dedupe overwrite

# Streaming import for large files
grns import -i tasks.jsonl --stream

# Transactional import
grns import -i tasks.jsonl --atomic
```

### Import record format

Each line in the JSONL file should be a task record. The minimum required field is `title`. If `id` is provided and already exists, the dedupe mode controls behavior.

Import is project-scoped to your configured `project_prefix` (or explicit project route when calling HTTP directly):
- if `id` is present, its prefix must match the target project
- if `project` is present, it must match the target project
- `parent_id`/dependency `parent_id` values must stay within the same project

```json
{"project":"gr","id":"gr-ab12","title":"Add auth flow","status":"open","type":"feature","priority":1,"labels":["auth"],"deps":[{"parent_id":"gr-0001","type":"blocks"}]}
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-i`, `--input` | (required) | Path to JSONL input file |
| `--dry-run` | false | Preview import without making changes |
| `--dedupe` | `skip` | How to handle existing task IDs |
| `--orphan-handling` | `allow` | How to handle deps referencing missing tasks |
| `--atomic` | false | Apply transactionally (all-or-nothing per chunk) |
| `--stream` | false | Use streaming endpoint (recommended for large files) |

### Dedupe modes

| Mode | Behavior |
|------|----------|
| `skip` | If a task with the same ID exists, skip the import record. |
| `overwrite` | If a task with the same ID exists, update it with the import data. |
| `error` | If a task with the same ID exists, count it as an error. |

### Orphan handling

Controls what happens when an imported dependency references a task ID that doesn't exist.

| Mode | Behavior |
|------|----------|
| `allow` | Create the dependency anyway (the target may be imported later). |
| `skip` | Silently skip the orphan dependency; import the task itself. |
| `strict` | Count orphan deps as errors; skip the dependency update (task upsert may still apply). |

### Import process

Import uses a two-pass approach:

1. **Pass 1 (tasks):** Upsert task records according to dedupe mode.
2. **Pass 2 (deps):** Create dependency relationships. This ordering ensures FK constraints are satisfied.

### Streaming vs default mode

- **Default** (`grns import -i file.jsonl`): The CLI reads the entire file into memory, sends it as a single JSON request. Good for files up to a few thousand records.
- **Streaming** (`grns import -i file.jsonl --stream`): The CLI streams the NDJSON body directly to the server. The server processes records in chunks. Recommended for large files (10k+ records) to avoid memory pressure.

### Atomic mode

When `--atomic` is set:
- Default mode: the entire import request is applied in a single database transaction.
- Streaming mode: each received chunk is applied in a separate transaction.

Without `--atomic`, records are applied individually (best-effort). Failures in one record do not prevent other records from being imported.

### Import response

```json
{
  "created": 5,
  "updated": 2,
  "skipped": 1,
  "errors": 0,
  "dry_run": false,
  "task_ids": ["gr-ab12", "gr-cd34", "..."],
  "messages": [],
  "apply_mode": "atomic",
  "applied_chunks": 1
}
```

- `created` — number of new tasks created
- `updated` — number of existing tasks updated (overwrite mode)
- `skipped` — number of records skipped (skip mode or dry-run)
- `errors` — number of records that failed
- `messages` — per-record error/warning messages (if any)
- `apply_mode` — `atomic` or empty for best-effort
- `applied_chunks` — number of transactional chunks applied (streaming + atomic)

## Round-trip example

```bash
# Export current state
grns export -o backup.jsonl

# ... make changes, or move to another machine ...

# Import into a fresh instance
grns import -i backup.jsonl --dedupe skip

# Or overwrite to sync
grns import -i backup.jsonl --dedupe overwrite
```
