# Grns Attachments Schema & Migration Plan (MVP)

This is the concrete schema/contract companion to [attachments.md](attachments.md).

## MVP decisions captured here
- Migration is additive (`v5`).
- Managed bytes are **not** stored in SQLite.
- DB enforces valid source shape.
- No automatic backfill from `spec_id`.
- GC is command-driven (`dry-run` or `apply`).

---

## Migration v5

- **Version:** `5`
- **Description:** `attachments: add blobs, attachments, and attachment_labels tables with constraints and indexes`

### SQL

```sql
CREATE TABLE IF NOT EXISTS blobs (
  id TEXT PRIMARY KEY,
  sha256 TEXT NOT NULL UNIQUE,
  size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
  storage_backend TEXT NOT NULL,
  blob_key TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS attachments (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  source_type TEXT NOT NULL,
  title TEXT,
  filename TEXT,
  media_type TEXT,
  media_type_source TEXT NOT NULL DEFAULT 'unknown',

  blob_id TEXT,
  external_url TEXT,
  repo_path TEXT,

  meta_json TEXT,

  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  expires_at TEXT,

  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  FOREIGN KEY (blob_id) REFERENCES blobs(id) ON DELETE RESTRICT,

  CHECK (kind IN ('spec', 'diagram', 'artifact', 'diagnostic', 'archive', 'other')),
  CHECK (source_type IN ('managed_blob', 'external_url', 'repo_path')),
  CHECK (media_type_source IN ('sniffed', 'declared', 'inferred', 'unknown')),

  CHECK (
    (source_type = 'managed_blob' AND blob_id IS NOT NULL AND external_url IS NULL AND repo_path IS NULL) OR
    (source_type = 'external_url' AND blob_id IS NULL AND external_url IS NOT NULL AND repo_path IS NULL) OR
    (source_type = 'repo_path'    AND blob_id IS NULL AND external_url IS NULL AND repo_path IS NOT NULL)
  ),

  CHECK (expires_at IS NULL OR expires_at >= created_at)
);

CREATE TABLE IF NOT EXISTS attachment_labels (
  attachment_id TEXT NOT NULL,
  label TEXT NOT NULL,
  PRIMARY KEY (attachment_id, label),
  FOREIGN KEY (attachment_id) REFERENCES attachments(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_attachments_task_created
  ON attachments(task_id, created_at);

CREATE INDEX IF NOT EXISTS idx_attachments_kind
  ON attachments(kind);

CREATE INDEX IF NOT EXISTS idx_attachments_media_type
  ON attachments(media_type);

CREATE INDEX IF NOT EXISTS idx_attachments_source_type
  ON attachments(source_type);

CREATE INDEX IF NOT EXISTS idx_attachments_blob_id
  ON attachments(blob_id);

CREATE INDEX IF NOT EXISTS idx_attachments_expires_at
  ON attachments(expires_at);

CREATE INDEX IF NOT EXISTS idx_attachment_labels_label
  ON attachment_labels(label);
```

### Timestamp format requirement
`created_at`, `updated_at`, and `expires_at` must use UTC RFC3339Nano text (`dbFormatTime` / `dbParseTime`) so lexical checks remain correct.

---

## ID and format rules

- Attachment ID: `at-[0-9a-z]{4}`
- Blob ID: `bl-[0-9a-z]{4}`
- SHA-256: 64-char lowercase hex

IDs are random (non-deterministic) and generated in service/store helpers.

---

## Store interfaces (MVP)

Keep one metadata interface for SQLite simplicity:

```go
type AttachmentStore interface {
    // Attachments + labels
    CreateAttachment(ctx context.Context, attachment *models.Attachment) error
    GetAttachment(ctx context.Context, id string) (*models.Attachment, error)
    ListAttachmentsByTask(ctx context.Context, taskID string) ([]models.Attachment, error)
    DeleteAttachment(ctx context.Context, id string) error

    ReplaceAttachmentLabels(ctx context.Context, attachmentID string, labels []string) error
    ListAttachmentLabels(ctx context.Context, attachmentID string) ([]string, error)

    // Blob metadata
    UpsertBlob(ctx context.Context, blob *models.Blob) (*models.Blob, error)
    GetBlob(ctx context.Context, id string) (*models.Blob, error)
    GetBlobBySHA256(ctx context.Context, sha string) (*models.Blob, error)
    ListUnreferencedBlobs(ctx context.Context, limit int) ([]models.Blob, error)
    DeleteBlob(ctx context.Context, id string) error
}
```

Raw bytes are handled by a separate interface:

```go
type BlobStore interface {
    Put(ctx context.Context, r io.Reader) (BlobPutResult, error)
    Open(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
}

type BlobPutResult struct {
    SHA256    string
    SizeBytes int64
    BlobKey   string
}
```

---

## Transaction and consistency contract

### Create managed attachment
1. Validate request/task existence.
2. `BlobStore.Put` writes bytes and returns digest/key.
3. In a single DB transaction:
   - `UpsertBlob` (by SHA)
   - insert attachment row
   - insert labels
4. Commit and return hydrated attachment.

If step 3 fails, created blob bytes are allowed to exist temporarily (orphan candidate).

### Delete attachment
- Delete attachment metadata row only.
- Blob row/bytes remain until GC confirms no references.

---

## Exact unreferenced-blob query (for GC)

Use this shape to avoid NULL-join ambiguity:

```sql
SELECT b.id, b.sha256, b.size_bytes, b.storage_backend, b.blob_key, b.created_at
FROM blobs b
LEFT JOIN attachments a ON a.blob_id = b.id
WHERE a.id IS NULL
ORDER BY b.created_at ASC
LIMIT ?;
```

---

## API contract (MVP)

- `POST /v1/tasks/{id}/attachments` (multipart)
- `POST /v1/tasks/{id}/attachments/link` (JSON)
- `GET /v1/tasks/{id}/attachments`
- `GET /v1/attachments/{attachment_id}`
- `GET /v1/attachments/{attachment_id}/content`
- `DELETE /v1/attachments/{attachment_id}`
- `POST /v1/admin/gc-blobs`

`POST /v1/admin/gc-blobs` request:

```json
{ "dry_run": true, "batch_size": 500 }
```

Response:

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

## Error mapping

- invalid input/policy mismatch -> `badRequest(...)`
- missing task/attachment/blob -> `notFound(...)`
- uniqueness conflicts -> `conflict(...)`

---

## Backward compatibility

- Existing task APIs unchanged.
- `spec_id` continues to work as-is.
- No migration backfill into attachments in v5.
