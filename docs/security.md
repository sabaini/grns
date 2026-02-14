# Security Notes

This document describes runtime security controls currently implemented in Grns.

## API authentication

### Bearer token for `/v1/*`

If `GRNS_API_TOKEN` is set in the server process:
- every `/v1/*` route requires authentication
- clients may authenticate with either:
  - `Authorization: Bearer <token>`
  - a valid browser session cookie
- `/health` remains unauthenticated.

The CLI/API client automatically sends the bearer header when `GRNS_API_TOKEN` is set in the client environment.

### Browser session auth (admin users)

Grns supports local admin users with cookie-based browser sessions.

- Admin users are provisioned out-of-band (CLI/script), e.g.:
  - `grns admin user add <username> --password-stdin`
- Passwords are stored as bcrypt hashes.
- Browser login endpoints:
  - `POST /v1/auth/login`
  - `POST /v1/auth/logout`
  - `GET /v1/auth/me`
- Session cookies are `HttpOnly`, `SameSite=Lax`, path `/`, with finite expiry.
- User-driven API auth enforcement is opt-in via `GRNS_REQUIRE_AUTH_WITH_USERS=true`.

Auth enforcement behavior:
- if `GRNS_API_TOKEN` is set, `/v1/*` requires auth and accepts either:
  - `Authorization: Bearer <token>`
  - a valid browser session cookie
- if `GRNS_API_TOKEN` is not set and `GRNS_REQUIRE_AUTH_WITH_USERS=true`, `/v1/*` requires a valid browser session cookie when at least one enabled admin user exists
- if neither is true, API remains open (except optional admin-token gating)

Login hardening:
- `POST /v1/auth/login` applies server-side in-memory throttling keyed by source IP + username.
- Excess failed attempts are rejected with HTTP `429` (`resource_exhausted`).

### Admin token for `/v1/admin/*`

If `GRNS_ADMIN_TOKEN` is set in the server process:
- admin routes (`/v1/admin/*`) also require:
  - `X-Admin-Token: <token>`

If both `GRNS_API_TOKEN` and `GRNS_ADMIN_TOKEN` are set, admin routes require both headers.

The client automatically sends `X-Admin-Token` for admin requests (currently cleanup and blob GC) when `GRNS_ADMIN_TOKEN` is set.

## Config trust model

By default, Grns does **not** auto-apply project-local config (`./.grns.toml`).

- Global config (`$HOME/.grns.toml`) is loaded by default.
- If that file is missing, Grns also checks `$SNAP_COMMON/.grns.toml` (when `$SNAP_COMMON` is set), then `~/snap/grns/common/.grns.toml`.
- Project config is loaded only when:
  - `GRNS_TRUST_PROJECT_CONFIG=true`
- When a trusted project config is applied, CLI prints:
  - `warning: using trusted project config from <path>`
- `GRNS_CONFIG_DIR` remains the strongest override and loads only:
  - `$GRNS_CONFIG_DIR/.grns.toml`

This reduces risk when running `grns` inside untrusted repositories.

## Bind safety

By default, explicit non-loopback bind hosts are blocked.

- Allowed by default: `127.0.0.1`, `localhost`, other loopback IPs
- Blocked by default (unless overridden): non-loopback hosts such as `0.0.0.0` and public/private non-loopback IPs
- To allow remote bind hosts, set:
  - `GRNS_ALLOW_REMOTE=true`

Note: hostless listen forms (for example `:7333`) are currently accepted by the host guard and may bind broadly depending on environment. For loopback-only behavior, use an explicit loopback host in `GRNS_API_URL`.

## Request and transport hardening

### HTTP server timeouts

The server sets:
- `ReadHeaderTimeout`
- `ReadTimeout`
- `WriteTimeout`
- `IdleTimeout`

### Request body limits

JSON payloads are size-limited via `http.MaxBytesReader`.

Current limits:
- default JSON endpoints: 1 MiB
- `/v1/projects/{project}/tasks/batch`: 8 MiB
- `/v1/projects/{project}/import`: 64 MiB
- `/v1/projects/{project}/import/stream`: 64 MiB

### Concurrency limiting for expensive endpoints

Server-side concurrency caps are applied to:
- import (limit: 1)
- export (limit: 2)
- heavy list queries (`search` / `spec` regex) (limit: 4)

Excess concurrent requests receive HTTP `429` with `resource_exhausted` and numeric `error_code`.

### CSRF guard for cookie-authenticated mutations

For cookie-authenticated `POST`/`PUT`/`PATCH`/`DELETE` requests, server middleware enforces same-origin checks via `Origin` matching.

- Missing or mismatched `Origin` is rejected with `403`.
- Bearer-token authenticated requests are not subject to this cookie CSRF check.

## Error exposure

For server-side internal errors (`5xx`):
- API response message is sanitized to `"internal error"`
- detailed root causes are logged server-side.

## Operational recommendations

- Keep API bound to loopback unless remote access is explicitly required.
- Use strong random values for `GRNS_API_TOKEN` and `GRNS_ADMIN_TOKEN`.
- Provision admin users with strong passwords; avoid sharing one account broadly.
- If exposing remotely, run behind TLS termination/proxy.
- Rotate tokens periodically and after suspected compromise.
