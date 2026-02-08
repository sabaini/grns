# Configuration

Grns reads config from TOML and supports runtime overrides via environment variables.

## File locations

- Global: `$HOME/.grns.toml`
- Project: `./.grns.toml` (loaded only when `GRNS_TRUST_PROJECT_CONFIG=true`)
- Override location: set `GRNS_CONFIG_DIR`, then Grns reads `$GRNS_CONFIG_DIR/.grns.toml`

## Supported keys

Top-level keys:
- `project_prefix` (default: `gr`; used as `{project}` for `/v1/projects/{project}/...` API routes)
- `api_url` (default: `http://127.0.0.1:7333`)
- `db_path` (default: `.grns.db` in workspace)

Attachment keys:
- `attachments.max_upload_bytes` (default: `104857600`)
- `attachments.multipart_max_memory` (default: `8388608`)
- `attachments.allowed_media_types` (default: empty)
- `attachments.reject_media_type_mismatch` (default: `true`)
- `attachments.gc_batch_size` (default: `500`)

## CLI examples

Read values:

```bash
grns config get project_prefix
grns config get attachments.gc_batch_size
```

Set values (project-local by default):

```bash
grns config set project_prefix ac
grns config set attachments.max_upload_bytes 209715200
grns config set attachments.gc_batch_size 1000
```

Set values globally:

```bash
grns config set --global api_url http://127.0.0.1:7333
grns config set --global attachments.reject_media_type_mismatch false
```

Set list values (comma-separated):

```bash
grns config set attachments.allowed_media_types "application/pdf,text/plain,image/png"
```

## TOML example

```toml
project_prefix = "gr"
api_url = "http://127.0.0.1:7333"
db_path = ".grns.db"

[attachments]
max_upload_bytes = 104857600
multipart_max_memory = 8388608
allowed_media_types = ["application/pdf", "text/plain"]
reject_media_type_mismatch = true
gc_batch_size = 500
```

## Environment variable overrides

General:
- `GRNS_API_URL` overrides `api_url`
- `GRNS_DB` overrides `db_path`

Attachment policy env overrides:
- `GRNS_ATTACH_ALLOWED_MEDIA_TYPES` (comma-separated)
- `GRNS_ATTACH_REJECT_MEDIA_TYPE_MISMATCH` (`true|false`)

## Notes

- Attachment server settings are applied when the server starts. Restart the server after changing attachment config.
- `attachments.allowed_media_types` values are normalized to lowercase MIME types.
