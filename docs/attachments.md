# Grns Attachments Design (Draft)

## Summary
Grns should support task/epic attachments (specs, diagrams, artifacts, diagnostics archives) with a **hybrid storage model**:

- **SQLite stores metadata and relationships**.
- **Blob bytes live outside SQLite** in a managed blob store.
- Attachments can reference either a managed blob or an external link/repo path.

This preserves fast task queries and keeps blob growth/retention manageable.

Related follow-up:
- [Attachments Schema & Migration Plan](attachments-schema.md)

---

## Why this design
Storing large files directly in SQLite BLOB columns would increase WAL/database growth, make backups heavier, and complicate GC/retention. A metadata + blob-store split is a better fit for Grns’s architecture (CLI → REST → service layer → store).

---

## MVP goals
- First-class attachment records linked to tasks (including epics).
- Managed uploads and external references.
- Safe defaults for untrusted archives.
- Predictable CLI/API behavior with JSON output.
- Clear retention + GC lifecycle.

## Non-goals (MVP)
- Preview/render pipelines.
- Full-text indexing of binary contents.
- Cross-task sharing UI.
- Object-storage backends (S3, etc.) in MVP.
- Presigned URL workflows.

---

## Core design decisions
1. **Separate attachment reference from blob object**.
2. **Immutable managed blobs** (content-addressed by SHA-256).
3. **Service-layer orchestration** (handlers never call store/blob backends directly).
4. **Archive safety by default** (opaque storage first; optional inspection later).
5. **Keep MVP narrow**; advanced indexing and storage backends are follow-on phases.

---

## Domain model

### Attachment (user-facing reference)
Represents “what is attached to task X”.

Fields:
- `id` (`at-...`)
- `task_id`
- `kind` (`spec`, `diagram`, `artifact`, `diagnostic`, `archive`, `other`)
- `source_type` (`managed_blob`, `external_url`, `repo_path`)
- `title` (optional)
- `filename` (optional)
- `media_type` (optional)
- `blob_id` (nullable; set when `source_type=managed_blob`)
- `external_url` (nullable)
- `repo_path` (nullable)
- `meta_json` (nullable, constrained keys)
- `created_at`, `updated_at`
- `expires_at` (nullable)

Rules:
- `kind` describes domain intent, not transport/storage.
- Exactly one source path:
  - managed blob via `blob_id`, or
  - `external_url`, or
  - `repo_path`.

### Blob (internal object)
Represents immutable stored bytes.

Fields:
- `id` (`bl-...`)
- `sha256` (unique)
- `size_bytes`
- `storage_backend` (`local_cas` in MVP)
- `blob_key` (backend-specific key/path)
- `created_at`

Benefits:
- Cleaner dedupe semantics.
- Clear GC ownership model.
- Easier future backend migration.

### Suggested tables
- `attachments`
- `blobs`

Indexes:
- `attachments(task_id, created_at)`
- `attachments(kind)`
- `attachments(source_type)`
- `attachments(expires_at)`
- `blobs(sha256)` unique

---

## Service boundaries and interfaces

### Handler layer
- Parse request and content type.
- Call service methods.
- Return JSON/errors.

### Service layer
Create `AttachmentService` (in `internal/server`) for attachment workflows.
- Validate input and policy limits.
- Check task existence.
- Coordinate store + blob store.
- Map errors to API error types.

This avoids overloading `TaskService` and keeps SRP clean.

### Store interfaces
Add dedicated `AttachmentStore` interface (do not bloat `TaskStore`).

Suggested split:
- `TaskStore`: task CRUD/query only.
- `AttachmentStore`: attachment/blob metadata CRUD/query.
- `BlobStore`: byte read/write/delete abstraction.

Dependency direction:

```
handlers -> AttachmentService -> (AttachmentStore + BlobStore)
```

`BlobStore` must not depend on task or HTTP concerns.

---

## Blob storage strategy (MVP)

### Local content-addressed store
- Root: `.grns/blobs/sha256/`
- Path: `.grns/blobs/sha256/ab/cd/<full_sha256>`
- Write flow:
  1. stream request to temp file
  2. compute `sha256` + `size_bytes`
  3. enforce upload limit
  4. atomically move to CAS path if not present
  5. upsert/find `blobs` row by digest

---

## Data/control flow (explicit)

### A) Upload managed attachment
1. Handler parses multipart.
2. `AttachmentService.CreateManaged(...)` validates kind/task/policy.
3. Service calls `BlobStore.Put(stream)` -> returns `{sha256, size_bytes, blob_key}`.
4. Service upserts/fetches `blobs` row.
5. Service inserts `attachments` row referencing `blob_id`.
6. Return attachment metadata.

Failure handling:
- If blob write succeeds but DB insert fails: mark blob as orphan-candidate; GC reclaims later.
- If DB fails before attachment insert: no visible attachment record is created.

### B) Create external reference
1. Handler parses JSON.
2. `AttachmentService.CreateLink(...)` validates URL/path.
3. Insert `attachments` row with `source_type=external_url|repo_path`.

### C) Delete attachment
1. Soft-delete is **not required in MVP**; hard delete metadata row.
2. Blob bytes are not deleted synchronously.
3. GC removes unreferenced blob objects.

### D) Download managed content
1. Resolve attachment and ensure `source_type=managed_blob`.
2. Resolve blob metadata.
3. Stream bytes from `BlobStore`.

---

## Archive handling

### MVP
- Treat archives (`zip`, `tar`, `tar.gz`) as opaque managed blobs.
- No auto-extraction.
- Optional archive inspection is deferred.

### Phase 2 hardening
- Add optional manifest extraction with strict limits.
- Validation should reject dangerous paths (`..`, absolute paths, symlink escapes).

---

## API shape (proposed)

Base: `/v1`

- `POST /tasks/{id}/attachments` (multipart upload)
- `POST /tasks/{id}/attachments/link` (JSON external reference)
- `GET /tasks/{id}/attachments`
- `GET /attachments/{attachment_id}`
- `GET /attachments/{attachment_id}/content`
- `DELETE /attachments/{attachment_id}`

Rationale for split create endpoints:
- simpler validation/parsing paths
- less content-type ambiguity
- cleaner handler logic

---

## CLI shape (proposed)

- `grns attach add <task-id> <path> --kind <kind> [--title ...] [--expires-at ...]`
- `grns attach add-link <task-id> --kind <kind> --url <https://...>`
- `grns attach add-link <task-id> --kind <kind> --repo-path <path>`
- `grns attach list <task-id> [--json]`
- `grns attach show <attachment-id> [--json]`
- `grns attach get <attachment-id> -o <path>`
- `grns attach rm <attachment-id>`

---

## Config (explicit, no magic numbers)

Recommended keys:
- `attachments.max_upload_bytes` (default: `104857600` = 100 MiB)
- `attachments.allowed_archive_types` (default: `zip,tar,tar.gz`)
- `attachments.gc_interval` (default: `24h`)
- `attachments.enable_archive_inspection` (default: `false` in MVP)

Phase 2 archive-inspection limits:
- `attachments.archive.max_entries`
- `attachments.archive.max_total_unpacked_bytes`
- `attachments.archive.max_path_length`

---

## Retention and GC

### Policy
- `expires_at` allows ephemeral artifact retention.
- Expired attachments can be removed by admin command/policy.

### GC command
- `grns admin gc-blobs --dry-run`
- `grns admin gc-blobs --apply`

GC responsibilities:
- Find blob rows with zero referencing attachments.
- Delete underlying blob objects.
- Delete orphan blob metadata rows.
- Report reclaimed bytes/counts in JSON.

---

## Rollout plan

### Phase 1 (MVP)
1. Migration: add `attachments` + `blobs` tables and indexes.
2. Implement `AttachmentStore` and `BlobStore(local_cas)`.
3. Implement `AttachmentService` + handlers + CLI commands.
4. Implement GC command for unreferenced blobs.
5. Tests:
   - service validation/error mapping
   - store CRUD/list
   - upload/download/delete integration
   - GC correctness

### Phase 2
- Optional archive inspection + manifest metadata.
- Additional retention policies.

### Phase 3
- Pluggable remote blob backends (S3-compatible, etc.).

---

## Design quality check (self-assessment)
- **Understandable for new team members:** yes, with explicit service/interface boundaries and flow definitions.
- **Matches domain:** yes, separates “attachment reference” from “blob object”.
- **Complexity justified:** yes for hybrid storage; advanced features intentionally deferred.

---

## Recommendation
Proceed with the narrowed MVP above:
- explicit `AttachmentService`
- separate `attachments` and `blobs` metadata
- local CAS `BlobStore`
- explicit failure/GC lifecycle
- defer non-essential advanced features.
