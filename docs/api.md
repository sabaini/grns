# API Reference

Base URL: `http://127.0.0.1:7333` (default)

All `/v1/*` routes require `Authorization: Bearer <token>` when `GRNS_API_TOKEN` is set.
Admin routes (`/v1/admin/*`) additionally require `X-Admin-Token: <token>` when `GRNS_ADMIN_TOKEN` is set.

Timestamps are RFC3339 UTC. Error responses include `error`, `code`, and `error_code` fields.

---

## Health & Info

### `GET /health`

Health check. No auth required.

**Response:** `200 OK`
```json
{ "status": "ok" }
```

### `GET /v1/info`

Server metadata and task counts.

**Response:**
```json
{
  "project_prefix": "gr",
  "schema_version": 6,
  "task_counts": { "open": 5, "closed": 3 },
  "total_tasks": 8
}
```

---

## Tasks

### `POST /v1/tasks`

Create a single task.

**Request:**
```json
{
  "id": "gr-ab12",
  "title": "Add auth flow",
  "status": "open",
  "type": "feature",
  "priority": 1,
  "description": "...",
  "spec_id": "SPEC-001",
  "parent_id": "gr-0001",
  "assignee": "alice",
  "notes": "...",
  "design": "...",
  "acceptance_criteria": "...",
  "source_repo": "github.com/org/repo",
  "custom": { "team": "platform" },
  "labels": ["auth", "backend"],
  "deps": [{ "parent_id": "gr-0001", "type": "blocks" }]
}
```

All fields except `title` are optional. Defaults: `status=open`, `priority=2`, `type=task`.

**Response:** `201 Created` — full `TaskResponse`.

### `GET /v1/tasks`

List tasks with filters. All filters are query parameters.

| Parameter | Type | Description |
|-----------|------|-------------|
| `status` | string | Filter by status (comma-separated for multiple) |
| `priority` | string | Exact priority |
| `priority_min` | string | Minimum priority (inclusive) |
| `priority_max` | string | Maximum priority (inclusive) |
| `type` | string | Filter by type |
| `label` | string | Tasks with all listed labels (comma-separated, AND) |
| `label_any` | string | Tasks with any listed label (comma-separated, OR) |
| `spec` | string | Spec ID regex (RE2, case-insensitive) |
| `parent_id` | string | Filter by parent task ID |
| `assignee` | string | Filter by assignee |
| `no_assignee` | bool | Only unassigned tasks |
| `id` | string | Filter by IDs (comma-separated) |
| `title_contains` | string | Title substring match |
| `desc_contains` | string | Description substring match |
| `notes_contains` | string | Notes substring match |
| `created_after` | string | Created after (RFC3339 or YYYY-MM-DD) |
| `created_before` | string | Created before |
| `updated_after` | string | Updated after |
| `updated_before` | string | Updated before |
| `closed_after` | string | Closed after |
| `closed_before` | string | Closed before |
| `empty_description` | bool | Tasks with no description |
| `no_labels` | bool | Tasks with no labels |
| `search` | string | FTS5 full-text search query |
| `limit` | int | Max results |
| `offset` | int | Skip N results |

**Response:** `200 OK` — array of `TaskResponse`.

### `GET /v1/tasks/{id}`

Get a single task by ID.

**Response:** `200 OK` — `TaskResponse`.

### `PATCH /v1/tasks/{id}`

Update task fields. Only provided fields are changed.

**Request:**
```json
{
  "title": "New title",
  "status": "in_progress",
  "priority": 1,
  "custom": { "team": "infra" }
}
```

**Response:** `200 OK` — updated `TaskResponse`.

### `POST /v1/tasks/get`

Bulk get tasks by IDs.

**Request:**
```json
{ "ids": ["gr-ab12", "gr-cd34"] }
```

**Response:** `200 OK` — array of `TaskResponse`.

### `POST /v1/tasks/batch`

Batch create tasks (transactional, all-or-nothing).

**Request:** array of `TaskCreateRequest`.

**Response:** `201 Created` — array of `TaskResponse`.

### `POST /v1/tasks/close`

Close one or more tasks. Optionally annotate with a git commit.

**Request:**
```json
{
  "ids": ["gr-ab12"],
  "commit": "a3f0...40hex",
  "repo": "github.com/org/repo"
}
```

`commit` and `repo` are optional. When provided, a `closed_by` git ref is created.

**Response:** `200 OK` — array of closed `TaskResponse`.

### `POST /v1/tasks/reopen`

Reopen closed tasks.

**Request:**
```json
{ "ids": ["gr-ab12"] }
```

**Response:** `200 OK` — array of reopened `TaskResponse`.

### `GET /v1/tasks/ready`

Tasks with no open blockers. Query params: `limit`, `offset`.

**Response:** `200 OK` — array of `TaskResponse`.

### `GET /v1/tasks/stale`

Tasks not updated recently. Query params: `days` (default 30), `status`, `limit`.

**Response:** `200 OK` — array of `TaskResponse`.

---

## Task Response Schema

```json
{
  "id": "gr-ab12",
  "title": "Add auth flow",
  "status": "open",
  "type": "feature",
  "priority": 1,
  "description": "...",
  "spec_id": "",
  "parent_id": "",
  "assignee": "",
  "notes": "",
  "design": "",
  "acceptance_criteria": "",
  "source_repo": "",
  "custom": {},
  "created_at": "2026-01-15T10:00:00Z",
  "updated_at": "2026-01-15T10:00:00Z",
  "closed_at": null,
  "labels": ["auth"],
  "deps": [{ "parent_id": "gr-0001", "type": "blocks" }]
}
```

---

## Labels

### `GET /v1/tasks/{id}/labels`

List labels for a task.

**Response:** `200 OK` — array of strings.

### `POST /v1/tasks/{id}/labels`

Add labels to a task.

**Request:**
```json
{ "labels": ["backend", "urgent"] }
```

**Response:** `204 No Content`.

### `DELETE /v1/tasks/{id}/labels`

Remove labels from a task.

**Request:**
```json
{ "labels": ["urgent"] }
```

**Response:** `204 No Content`.

### `GET /v1/labels`

List all labels across all tasks.

**Response:** `200 OK` — array of strings.

---

## Dependencies

### `POST /v1/deps`

Add a dependency between tasks.

**Request:**
```json
{
  "child_id": "gr-ab12",
  "parent_id": "gr-cd34",
  "type": "blocks"
}
```

`type` defaults to `blocks` if omitted.

**Response:** `201 Created`.

### `GET /v1/tasks/{id}/deps/tree`

Get the full dependency tree for a task. Max depth: 50.

**Response:**
```json
{
  "root_id": "gr-ab12",
  "nodes": [
    {
      "id": "gr-cd34",
      "title": "Blocker task",
      "status": "open",
      "type": "task",
      "depth": 1,
      "direction": "upstream",
      "dep_type": "blocks"
    }
  ]
}
```

---

## Attachments

### `POST /v1/tasks/{id}/attachments`

Upload a managed file attachment. Content-Type: `multipart/form-data`.

**Form fields:**
- `file` — the file to upload (required)
- `kind` — attachment kind: `spec`, `diagram`, `artifact`, `diagnostic`, `archive`, `other` (required)
- `title` — display title (optional)
- `media_type` — declared MIME type (optional; validated against sniffed type)
- `labels` — comma-separated labels (optional)

**Response:** `201 Created` — `Attachment`.

### `POST /v1/tasks/{id}/attachments/link`

Create a link or repo path attachment.

**Request:**
```json
{
  "kind": "spec",
  "title": "Design doc",
  "external_url": "https://example.com/doc.pdf",
  "labels": ["design"]
}
```

Exactly one of `external_url` or `repo_path` must be provided.

- `external_url`: absolute `http` or `https` URL
- `repo_path`: workspace-relative path (no leading `/`, no `..`)

**Response:** `201 Created` — `Attachment`.

### `GET /v1/tasks/{id}/attachments`

List attachments for a task.

**Response:** `200 OK` — array of `Attachment`.

### `GET /v1/attachments/{attachment_id}`

Get attachment metadata.

**Response:** `200 OK` — `Attachment`.

### `GET /v1/attachments/{attachment_id}/content`

Download attachment content (managed blobs only).

**Response:** `200 OK` — raw bytes with appropriate Content-Type.

### `DELETE /v1/attachments/{attachment_id}`

Delete an attachment (metadata only; blob bytes reclaimed by GC).

**Response:** `204 No Content`.

### Attachment Schema

```json
{
  "id": "at-ab12",
  "task_id": "gr-cd34",
  "kind": "spec",
  "source_type": "managed_blob",
  "title": "Design doc",
  "filename": "design.pdf",
  "media_type": "application/pdf",
  "media_type_source": "sniffed",
  "blob_id": "bl-ef56",
  "external_url": "",
  "repo_path": "",
  "meta": {},
  "labels": ["design"],
  "created_at": "2026-01-15T10:00:00Z",
  "updated_at": "2026-01-15T10:00:00Z",
  "expires_at": null
}
```

---

## Git References

### `POST /v1/tasks/{id}/git-refs`

Create a git reference linking a task to a git object.

**Request:**
```json
{
  "repo": "github.com/org/repo",
  "relation": "design_doc",
  "object_type": "path",
  "object_value": "docs/design.md",
  "resolved_commit": "a3f0...40hex",
  "note": "Primary design source"
}
```

- `repo` is optional if the task has `source_repo` set.
- `relation` values: `design_doc`, `implements`, `fix_commit`, `closed_by`, `introduced_by`, `related`, or `x-*` for extensions.
- `object_type` values: `commit`, `tag`, `branch`, `path`, `blob`, `tree`.
- For `commit`/`blob`/`tree`, `object_value` must be a 40-char lowercase hex SHA.
- `resolved_commit` is optional but recommended for mutable refs.

**Response:** `201 Created` — `TaskGitRef`.

### `GET /v1/tasks/{id}/git-refs`

List git references for a task.

**Response:** `200 OK` — array of `TaskGitRef`.

### `GET /v1/git-refs/{ref_id}`

Get a single git reference.

**Response:** `200 OK` — `TaskGitRef`.

### `DELETE /v1/git-refs/{ref_id}`

Delete a git reference.

**Response:** `204 No Content`.

### TaskGitRef Schema

```json
{
  "id": "gf-ab12",
  "task_id": "gr-cd34",
  "repo_id": "rp-ef56",
  "repo": "github.com/org/repo",
  "relation": "design_doc",
  "object_type": "path",
  "object_value": "docs/design.md",
  "resolved_commit": "",
  "note": "",
  "meta": {},
  "created_at": "2026-01-15T10:00:00Z",
  "updated_at": "2026-01-15T10:00:00Z"
}
```

---

## Import / Export

### `GET /v1/export`

Export all tasks as NDJSON (one JSON object per line).

**Response:** `200 OK`, `Content-Type: application/x-ndjson` — streamed `TaskResponse` per line.

### `POST /v1/import`

Import tasks from JSON body.

**Request:**
```json
{
  "tasks": [
    {
      "id": "gr-ab12",
      "title": "Imported task",
      "status": "open",
      "type": "task",
      "priority": 2,
      "labels": ["imported"],
      "deps": []
    }
  ],
  "dry_run": false,
  "dedupe": "skip",
  "orphan_handling": "allow",
  "atomic": false
}
```

**Response:**
```json
{
  "created": 1,
  "updated": 0,
  "skipped": 0,
  "errors": 0,
  "dry_run": false,
  "task_ids": ["gr-ab12"],
  "messages": [],
  "apply_mode": "",
  "applied_chunks": 0
}
```

### `POST /v1/import/stream`

Streaming NDJSON import for large files. Body is raw NDJSON.

Query params: `dry_run`, `dedupe`, `orphan_handling`, `atomic`.

**Response:** same as `POST /v1/import`.

---

## Admin

### `POST /v1/admin/cleanup`

Delete old closed/tombstone tasks. Requires admin token and `X-Confirm: true` header for non-dry-run.

**Request:**
```json
{
  "older_than_days": 90,
  "dry_run": true
}
```

**Response:**
```json
{
  "task_ids": ["gr-ab12"],
  "count": 1,
  "dry_run": true
}
```

### `POST /v1/admin/gc-blobs`

Garbage-collect unreferenced blobs. Requires admin token.

**Request:**
```json
{
  "dry_run": true,
  "batch_size": 500
}
```

**Response:**
```json
{
  "candidate_count": 10,
  "deleted_count": 10,
  "failed_count": 0,
  "reclaimed_bytes": 123456,
  "dry_run": false
}
```

---

## Error Responses

All errors follow this format:

```json
{
  "error": "invalid id",
  "code": "invalid_argument",
  "error_code": 1004
}
```

| Range | Category |
|-------|----------|
| 1xxx | Validation (bad request) |
| 2xxx | Domain state (not found, conflict) |
| 3xxx | Auth/limits (unauthorized, forbidden, rate-limited) |
| 4xxx | Internal (server errors) |

See `docs/errcode-design.md` for the full catalog.
