# API Reference (Project-Scoped)

> **Breaking pre-release API change**
>
> Grns now scopes all domain endpoints under `/v1/projects/{project}/...`.
> Backward compatibility with legacy unscoped `/v1/tasks...` routes is not provided.

Base URL: `http://127.0.0.1:7333` (default)

Auth:
- All `/v1/*` routes require `Authorization: Bearer <token>` when `GRNS_API_TOKEN` is set.
- If at least one enabled local admin user exists, `/v1/*` also supports/accepts browser session cookie auth.
- Admin routes (`/v1/admin/*`) additionally require `X-Admin-Token: <token>` when `GRNS_ADMIN_TOKEN` is set.

Timestamps are RFC3339 UTC. Error responses include `error`, `code`, and `error_code`.

---

## Conventions

### Project path segment

`{project}` is required on all non-global domain routes.

- Format: `^[a-z]{2}$`
- Example: `gr`

### Resource identity

All domain resources include a `project` field in responses, even though project is also in the route path.

### ID rules

Task IDs remain `<prefix>-<4char>` (e.g. `gr-ab12`), and the `<prefix>` must match the route `{project}`.

Cross-project references are invalid (dependencies, task attachments, git refs, etc.) and return `400`.

---

## Global Endpoints

### `GET /health`

Health check. No auth required.

**Response:** `200 OK`
```json
{ "status": "ok" }
```

### `GET /v1/info`

Global server/database metadata across all projects.

**Response (example):**
```json
{
  "schema_version": 8,
  "task_counts": { "open": 12, "closed": 4 },
  "total_tasks": 16,
  "projects": ["gr", "xy"]
}
```

### `POST /v1/auth/login`

Browser login with local admin credentials.

Request body:
```json
{ "username": "admin", "password": "..." }
```

On success returns `200` and sets an HttpOnly session cookie.

### `POST /v1/auth/logout`

Revokes current session cookie and clears it on the client.

### `GET /v1/auth/me`

Returns current auth/session state for UI bootstrap.

---

## Project-Scoped Endpoints

All endpoints below are relative to:

`/v1/projects/{project}`

---

## Tasks

### `POST /v1/projects/{project}/tasks`
Create one task.

### `GET /v1/projects/{project}/tasks`
List tasks in one project.

Supported query params are unchanged from legacy list API (`status`, `type`, `label`, `search`, `limit`, `offset`, etc.), now scoped to `{project}`.

### `GET /v1/projects/{project}/tasks/{id}`
Get one task.

### `PATCH /v1/projects/{project}/tasks/{id}`
Update one task.

### `POST /v1/projects/{project}/tasks/get`
Bulk get tasks by ID list.

### `POST /v1/projects/{project}/tasks/batch`
Batch create tasks (transactional).

### `POST /v1/projects/{project}/tasks/close`
Close tasks (optional commit annotation).

### `POST /v1/projects/{project}/tasks/reopen`
Reopen tasks.

### `GET /v1/projects/{project}/tasks/ready`
List ready tasks.

### `GET /v1/projects/{project}/tasks/stale`
List stale tasks.

---

## Labels

### `GET /v1/projects/{project}/labels`
List labels used in this project.

### `GET /v1/projects/{project}/tasks/{id}/labels`
List labels for one task.

### `POST /v1/projects/{project}/tasks/{id}/labels`
Add labels to one task.

### `DELETE /v1/projects/{project}/tasks/{id}/labels`
Remove labels from one task.

---

## Dependencies

### `POST /v1/projects/{project}/deps`
Create dependency edge between tasks in the same project.

### `GET /v1/projects/{project}/tasks/{id}/deps/tree`
Get dependency tree for one task (same project only).

---

## Attachments

### `POST /v1/projects/{project}/tasks/{id}/attachments`
Upload managed attachment (`multipart/form-data`).

### `POST /v1/projects/{project}/tasks/{id}/attachments/link`
Create link/repo-path attachment.

### `GET /v1/projects/{project}/tasks/{id}/attachments`
List task attachments.

### `GET /v1/projects/{project}/attachments/{attachment_id}`
Get attachment metadata.

### `GET /v1/projects/{project}/attachments/{attachment_id}/content`
Download managed attachment content.

### `DELETE /v1/projects/{project}/attachments/{attachment_id}`
Delete attachment metadata.

---

## Git References

### `POST /v1/projects/{project}/tasks/{id}/git-refs`
Create task git ref.

### `GET /v1/projects/{project}/tasks/{id}/git-refs`
List task git refs.

### `GET /v1/projects/{project}/git-refs/{ref_id}`
Get one git ref.

### `DELETE /v1/projects/{project}/git-refs/{ref_id}`
Delete one git ref.

---

## Import / Export

### `GET /v1/projects/{project}/export`
Export project tasks as NDJSON.

### `POST /v1/projects/{project}/import`
Import tasks from JSON payload (project-scoped).

### `POST /v1/projects/{project}/import/stream`
Streaming NDJSON import (project-scoped).

---

## Admin (Global)

### `POST /v1/admin/cleanup`
Global admin cleanup endpoint.

Supports optional `project` filter when running project-targeted cleanup.

### `POST /v1/admin/gc-blobs`
Global blob GC endpoint.

---

## Resource schema deltas

### Task (delta)
```json
{
  "project": "gr",
  "id": "gr-ab12",
  "title": "Add auth flow"
}
```

### Attachment (delta)
```json
{
  "project": "gr",
  "id": "at-ab12",
  "task_id": "gr-cd34"
}
```

### TaskGitRef (delta)
```json
{
  "project": "gr",
  "id": "gf-ab12",
  "task_id": "gr-cd34"
}
```

---

## Error format

```json
{
  "error": "invalid id",
  "code": "invalid_argument",
  "error_code": 1004
}
```

See `docs/errcode-design.md` for the full error-code catalog.
