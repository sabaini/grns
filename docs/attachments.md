# Grns Attachments MVP Design

## Purpose
This document defines the **final MVP design** for attachments. It is intentionally narrow and removes open design choices.

Related: [Attachments Schema & Migration Plan](attachments-schema.md)

---

## MVP scope
Grns supports task attachments with a hybrid model:

- SQLite stores attachment/blob metadata and relationships.
- Blob bytes are stored outside SQLite in a local content-addressed store (CAS).
- Attachments can be either:
  - managed uploads (`managed_blob`), or
  - external references (`external_url`, `repo_path`).

### Non-goals (MVP)
- Archive extraction/inspection
- Preview/render/indexing
- Remote object stores (S3, etc.)
- Cross-task shared attachment UX
- Background GC daemon

---

## Domain model

### Attachment (user-facing reference)
Represents “this task has this artifact”.

Core fields:
- `id` (`at-xxxx`)
- `task_id`
- `kind` (`spec|diagram|artifact|diagnostic|archive|other`)
- `source_type` (`managed_blob|external_url|repo_path`)
- `title`, `filename`
- `media_type` (normalized lowercase MIME)
- `media_type_source` (`sniffed|declared|inferred|unknown` in MVP)
- exactly one source payload:
  - `blob_id` OR `external_url` OR `repo_path`
- `meta_json` (opaque JSON object)
- `labels[]` (normalized lowercase, deduped)
- `created_at`, `updated_at`, `expires_at`

### Blob (internal immutable object)
Represents raw bytes.

Fields:
- `id` (`bl-xxxx`)
- `sha256` (64-char lowercase hex, unique)
- `size_bytes`
- `storage_backend` (`local_cas`)
- `blob_key` (`sha256/ab/cd/<full_sha256>`)
- `created_at`

---

## Naming and boundary decisions (locked)

1. **`kind` is domain intent**, not media format.
2. **`media_type` is technical MIME**, not workflow meaning.
3. Keep task and attachment concerns separate:
   - handlers call `AttachmentService`
   - `AttachmentService` calls metadata store + blob store
4. **One metadata interface in MVP** (`AttachmentStore`) is acceptable for simplicity, but raw bytes stay behind a separate `BlobStore`.
5. No source dedupe for links in MVP (same URL/path can be attached multiple times).

Dependency flow:

```
HTTP handlers -> AttachmentService -> (AttachmentStore + BlobStore)
```

No handler may call store/blobstore directly.

---

## Storage strategy (MVP)

### Local CAS layout
- Root: `.grns/blobs/`
- Key format: `sha256/ab/cd/<sha256>`
- Absolute filesystem path is derived at runtime (not stored in DB).

### Managed upload flow
1. Validate task id/kind/labels/policy.
2. Verify task exists.
3. Stream content to blob store (`Put`) while hashing.
4. In DB transaction:
   - upsert blob metadata by SHA-256
   - insert attachment row
   - insert labels
5. Return hydrated attachment.

If blob write succeeds but DB transaction fails, blob is an orphan candidate and is cleaned by GC.

---

## MIME policy (MVP, locked)

### Managed uploads
- Sniff MIME from bytes (`http.DetectContentType` behavior).
- If client did not provide `media_type`:
  - store sniffed MIME
  - `media_type_source=sniffed`
- If client provided `media_type`:
  - normalize lowercase, strip params for comparison
  - if sniffed type is not `application/octet-stream` and differs, reject (`badRequest`)
  - otherwise accept declared MIME
  - `media_type_source=declared`

### Links/repo paths
- No sniffing.
- If `media_type` provided, store as lowercase and mark `declared`.
- If omitted, leave null and mark `unknown`.

Allowed-media policy (if configured) applies to final normalized `media_type` when non-empty.

---

## Validation policy (MVP, locked)

- Task IDs: existing task regex (`^[a-z]{2}-[0-9a-z]{4}$`).
- Attachment IDs: `^at-[0-9a-z]{4}$`.
- Blob IDs: `^bl-[0-9a-z]{4}$`.
- Labels: reuse existing `normalizeLabels` rules (ASCII non-space, lowercase, deduped).
- `repo_path` must be workspace-relative:
  - not absolute
  - must not escape via `..`
- `external_url` must be valid absolute `http` or `https` URL.

---

## API (MVP)

- `POST /v1/tasks/{id}/attachments` (multipart managed upload)
- `POST /v1/tasks/{id}/attachments/link` (JSON link/path)
- `GET /v1/tasks/{id}/attachments`
- `GET /v1/attachments/{attachment_id}`
- `GET /v1/attachments/{attachment_id}/content` (managed only)
- `DELETE /v1/attachments/{attachment_id}`
- `POST /v1/admin/gc-blobs` (admin; dry-run/apply)

Create endpoints stay split to avoid content-type ambiguity.

---

## CLI (MVP)

- `grns attach add <task-id> <path> --kind ...`
- `grns attach add-link <task-id> --kind ... --url ...|--repo-path ...`
- `grns attach list <task-id>`
- `grns attach show <attachment-id>`
- `grns attach get <attachment-id> -o <path>`
- `grns attach rm <attachment-id>`
- `grns admin gc-blobs --dry-run|--apply`

---

## Config defaults (with rationale)

- `attachments.max_upload_bytes = 104857600` (100 MiB)
  - caps abuse and large accidental uploads
- `attachments.multipart_max_memory = 8388608` (8 MiB)
  - bounds in-memory multipart buffering
- `attachments.allowed_media_types = []` (allow all)
- `attachments.reject_media_type_mismatch = true`
- `attachments.gc_batch_size = 500`
  - avoids long DB/file-system lock windows

No `gc_interval` in MVP because GC is command-driven only.

---

## Retention + GC

- Deleting attachment removes metadata row only.
- Blob bytes are reclaimed later by GC when unreferenced.
- `expires_at` is metadata only in MVP (no automatic expiry worker).

GC command behavior:
1. Query unreferenced blobs.
2. Delete bytes by `blob_key`.
3. Delete blob metadata row.
4. Return summary: `candidate_count`, `deleted_count`, `failed_count`, `reclaimed_bytes`, `dry_run`.

---

## Why this complexity is justified

- Keeps task queries fast and DB small.
- Gives deterministic dedupe via SHA-256.
- Makes failures recoverable via explicit GC.
- Keeps advanced features out of MVP.
