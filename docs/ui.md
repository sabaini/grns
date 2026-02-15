# Web UI — Implementable Architecture & Plan

Related docs:
- [Design](design.md) — overall grns architecture
- [API Reference](api.md) — REST endpoints
- [Security](security.md) — auth and runtime hardening

## 1. Goals

Build a **read-heavy, search-first** web UI for grns with minimal server changes.

| Goal | Detail |
|------|--------|
| Search-first | FTS (`search`) + filters are first-class in list view |
| Simple edits | Create/update/close/reopen/tombstone, label add/remove |
| Single binary | `grns srv` serves API + static UI |
| No runtime Node | Built assets are committed and embedded in Go binary |
| API parity | UI uses existing `/v1/projects/{project}/...` endpoints for task/label/deps |
| Network auth | Browser login via admin users + server-side sessions |
| Lightweight | Small JS bundle, minimal dependencies |

## 2. Explicit Scope

### In scope (v1)
- Task list with filters and pagination
- Task detail with inline edits
- Create task form
- Close/reopen actions (single + bulk)
- Mark task as `tombstone` (via PATCH status)
- Ready/stale preset views

### Out of scope (v1)
- Server-side rendering/templates
- Websocket/SSE live updates
- Attachment and git-ref CRUD UI
- Web self-registration / password reset flows
- Theme system/branding

---

## 3. Runtime Architecture (final)

```
┌──────────────────────────────────────────────────┐
│                  grns binary                     │
│                                                  │
│  GET /              -> index.html               │
│  GET /ui/*          -> static assets            │
│  GET /health        -> health                   │
│  /v1/*              -> existing REST API        │
└──────────────────────────────────────────────────┘
                 │
               SQLite
```

Key decisions:
1. **Hash routing only** (`#/`, `#/tasks/{id}`, `#/create`).
2. **No SPA server fallback middleware** needed.
3. **No new task-domain endpoints** required for UI features.
4. **Auth extension adds `/v1/auth/*` endpoints** for browser session login.

### 3.1 How users run it

- Start server explicitly: `grns srv`
- Open: `http://127.0.0.1:7333/` (or configured `GRNS_API_URL` host)

Note: UI requires an explicitly running server (`grns srv` or a managed daemon).

---

## 4. Source Layout (implementable)

```text
ui/
├── src/
│   ├── index.jsx
│   ├── api.js
│   ├── routes.jsx
│   ├── components/
│   └── util/
├── package.json
└── esbuild.config.mjs      # outdir -> ../internal/server/uiassets/dist

internal/server/
├── ui.go                   # embed + handlers
└── uiassets/
    └── dist/               # committed build output
        ├── index.html
        ├── index.<hash>.js
        └── app.<hash>.css
```

Why this layout: `go:embed` paths are package-relative, so assets must be under `internal/server` (or another package that owns embedding).

---

## 5. Server Integration

### 5.1 Embedding

```go
// internal/server/ui.go
package server

import "embed"

//go:embed uiassets/dist/*
var uiFS embed.FS
```

### 5.2 Routes

Register UI routes **after** API/health routes:
- `GET /` -> serve `index.html`
- `GET /ui/` subtree -> static assets

No catch-all route and no rewrite middleware.

### 5.3 Auth behavior

- Static UI routes (`/`, `/ui/*`) are always readable.
- API auth modes:
  - **Bearer token** (existing): `Authorization: Bearer <token>`
  - **Browser session cookie** (new): set via login endpoint
- UI auth preference order:
  1. valid session cookie (`/v1/auth/me`)
  2. fallback bearer token from localStorage (legacy/dev)
- Admin users are created out-of-band (CLI/script), not via web registration.

---

## 6. Frontend Stack

| Layer | Choice |
|-------|--------|
| View | Preact |
| Routing | `preact-router` with hash routes |
| State | local component state + small shared store |
| HTTP | `fetch` wrapper |
| Build | esbuild |
| CSS | single file, no preprocessor |

Bundle target: small (`index.html` + one JS + one CSS).

---

## 7. API Contract Mapping (must-match)

| UI feature | Endpoint(s) | Notes |
|-----------|-------------|-------|
| List tasks | `GET /v1/projects/{project}/tasks` | Supports current query params only |
| Search | same (`search`) | Debounce 300 ms |
| Filters | same (`status,type,label,label_any,priority*,assignee,...`) | URL-synced |
| Pagination | same (`limit,offset`) | Page size default 50 |
| Task detail | `GET /tasks/{id}` | Includes labels/deps in response |
| Inline edit | `PATCH /tasks/{id}` | Field-level PATCH |
| Create | `POST /tasks` | Redirect to detail |
| Close | `POST /tasks/close` | `ids[]` payload |
| Reopen | `POST /tasks/reopen` | `ids[]` payload |
| Tombstone | `PATCH /tasks/{id}` with `status:"tombstone"` | No delete endpoint |
| Labels list | `GET /labels` | project-scoped list |
| Add labels | `POST /tasks/{id}/labels` | per-task |
| Remove labels | `DELETE /tasks/{id}/labels` | per-task |
| Dep tree | `GET /tasks/{id}/deps/tree` | UI filters nodes to `depth==1` for summary |
| Ready preset | `GET /tasks/ready?limit=` | Ready endpoint params are limited |
| Stale preset | `GET /tasks/stale?days=&status=&limit=` | |
| Bootstrap | `GET /v1/info` | use `project_prefix` |
| Session bootstrap | `GET /v1/auth/me` | returns current user/session or 401 |
| Login | `POST /v1/auth/login` | username + password; sets HttpOnly cookie |
| Logout | `POST /v1/auth/logout` | revokes session and clears cookie |

### 7.1 Auth extension contract (browser login)

New API surface:
- `POST /v1/auth/login`
- `POST /v1/auth/logout`
- `GET /v1/auth/me`

Server-side data model additions:
- `users` (admin-only accounts): `id`, `username`, `password_hash`, `role`, `disabled`, timestamps
- `sessions`: `id`, `user_id`, `token_hash`, `expires_at`, `revoked_at`, timestamps

Operational provisioning (out-of-band):
- `grns admin user add <username> --password-stdin`
- optional: list/disable/enable/delete commands or equivalent script wrappers

Important constraints:
- **No server sort param** today.
- Default ordering is backend-defined (non-search: `updated_at DESC`; search: FTS rank).
- UI should not claim global resorting across paginated data.

---

## 8. Screen Behavior

### 8.1 Task list (`#/`)

- Search input (`search`) with debounce
- Status/type/priority/label/assignee/date filters
- Ready/stale presets as explicit mode buttons
- Pagination via `limit/offset`
- Row click opens detail
- Bulk actions:
  - close/reopen: batch endpoints
  - add label: per-task calls with partial-failure summary

Default filter state: `status=open,in_progress,blocked,deferred,pinned`.

### 8.2 Task detail (`#/tasks/{id}`)

Editable controls:
- `status`, `type`, `priority`, `assignee`
- text blocks: `description`, `notes`, `acceptance_criteria`, `design`
- labels add/remove

Actions:
- Close (batch close endpoint with one ID)
- Reopen
- Mark tombstone (`PATCH status=tombstone`)

Dependencies:
- Fetch tree once
- Show immediate upstream/downstream (`depth == 1`) with links

### 8.3 Create (`#/create`)

Fields:
- title (required)
- type, priority, assignee
- labels
- parent_id (optional)
- description (optional)

On success: navigate to created task detail.

---

## 9. URL State + Project Resolution

- All list filters are stored in hash query for shareability:
  - `#/?search=auth&status=open,blocked&priority_min=2`
- Project resolution order:
  1. `project` from hash query if present
  2. `grns_ui_project` from localStorage
  3. `project_prefix` from `/v1/info`

v1 is single active project per browser tab.

---

## 10. Auth UX (admin login over network)

### 10.1 Browser session flow (primary)

1. UI boots and calls `/v1/auth/me`.
2. If authenticated, continue loading app data.
3. If `401`, render login screen (username/password).
4. Submit login to `POST /v1/auth/login`.
5. Server sets HttpOnly session cookie.
6. UI retries `/v1/auth/me` and proceeds.
7. Logout calls `POST /v1/auth/logout` and returns to login screen.

### 10.2 Legacy token fallback (optional/dev)

If auth endpoints are unavailable or explicitly bypassed, UI may still use:
- `localStorage['grns_api_token']` -> bearer token header.

Token is never embedded in HTML/assets.

### 10.3 Cookie and CSRF requirements

- Session cookie: `HttpOnly`, `SameSite=Lax`, `Path=/`, finite expiry.
- Set `Secure` in production (TLS).
- For cookie-authenticated mutating requests (`POST/PATCH/DELETE`), enforce same-origin checks (e.g., validate `Origin`) to prevent CSRF.

---

## 11. Build & CI Integration

### 11.1 Local commands

```bash
# install deps once
cd ui && npm ci

# build assets into internal/server/uiassets/dist
cd ui && npm run build
```

### 11.2 Go build

`just build` remains Go-only (no Node requirement) because embedded assets are committed.

### 11.3 Suggested just targets

```makefile
ui-build:
    cd ui && npm run build

ui-verify:
    just ui-build
    git diff --exit-code -- internal/server/uiassets/dist
```

---

## 12. Implementation Plan

### Phase 1 — Serve UI + read-only list/detail
1. Add `internal/server/ui.go` with embed + handlers.
2. Register `GET /` and `GET /ui/` routes.
3. Scaffold Preact app + hash router.
4. Implement list view (`search`, status filters, pagination).
5. Implement detail view (read-only).

### Phase 2 — Full filters + presets
6. Add type/priority/label/assignee/date filters.
7. Add URL-hash state sync.
8. Add ready/stale presets.

### Phase 3 — Mutations
9. Inline PATCH edits on detail.
10. Create task form.
11. Label add/remove.
12. Close/reopen single + bulk.
13. Tombstone action.

### Phase 4 — Polish
14. Keyboard shortcuts (`/`, `j/k`, `Enter`, `Esc`).
15. Loading/error UI states.
16. Dependency summary (`depth == 1`).

### Phase 5 — Browser auth (admin users)
17. Add `users` and `sessions` migrations.
18. Add admin user provisioning command(s) / script (`grns admin user add ...`).
19. Add auth endpoints: `POST /v1/auth/login`, `POST /v1/auth/logout`, `GET /v1/auth/me`.
20. Extend auth middleware: accept bearer token or valid session cookie.
21. Add same-origin CSRF checks for cookie-authenticated mutating requests.
22. Add UI login/logout screens and session bootstrap flow.
23. Keep optional legacy token fallback for local/dev operation.

---

## 13. Minimal Test Plan

- **Go handler tests**
  - `GET /` serves HTML
  - `GET /ui/{asset}` serves static asset (asset name taken from built index.html)
  - `/v1/*` routes still win over UI routes
- **Frontend unit/component tests**
  - hash parsing + list state utilities
  - list keyboard interactions and bulk actions
  - detail inline edit flows
- **Frontend E2E smoke**
  - open list, navigate to detail, verify task data visible
- **Auth tests (Go + UI)**
  - login success/failure
  - session cookie accepted on `/v1/*`
  - logout revokes session
  - CSRF origin guard for mutating cookie-auth requests
- **API integration smoke** (BATS)
  - list/detail/create via UI-called endpoints
- **Asset sync check**
  - CI `ui-verify` to ensure committed dist matches source

---

## 14. Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Search load under rapid input | 300 ms debounce; existing server limiter |
| Committed dist drift | `ui-verify` in CI |
| Stale browser assets | versioned asset names (hash) in build output |
| Feature mismatch vs API | maintain section 7 mapping as contract |
| Session misuse over plaintext transport | document TLS requirement; set `Secure` cookies in production |
| CSRF on cookie-auth mutations | enforce same-origin (`Origin`) checks for unsafe methods |
| Admin credential sprawl | out-of-band provisioning + disable/delete support + audit logs |
