# Security Notes

This document describes runtime security controls currently implemented in Grns.

## API authentication

### Bearer token for `/v1/*`

If `GRNS_API_TOKEN` is set in the server process:
- every `/v1/*` route requires:
  - `Authorization: Bearer <token>`
- `/health` remains unauthenticated.

The CLI/API client automatically sends this header when `GRNS_API_TOKEN` is set in the client environment.

### Admin token for `/v1/admin/*`

If `GRNS_ADMIN_TOKEN` is set in the server process:
- admin routes (`/v1/admin/*`) also require:
  - `X-Admin-Token: <token>`

The client automatically sends this header for admin cleanup requests when `GRNS_ADMIN_TOKEN` is set.

## Config trust model

By default, Grns does **not** auto-apply project-local config (`./.grns.toml`).

- Global config (`$HOME/.grns.toml`) is loaded by default.
- Project config is loaded only when:
  - `GRNS_TRUST_PROJECT_CONFIG=true`
- When a trusted project config is applied, CLI prints:
  - `warning: using trusted project config from <path>`
- `GRNS_CONFIG_DIR` remains the strongest override and loads only:
  - `$GRNS_CONFIG_DIR/.grns.toml`

This reduces risk when running `grns` inside untrusted repositories.

## Bind safety

By default, non-loopback bind addresses are blocked.

- Allowed by default: `127.0.0.1`, `localhost`
- To allow remote bind hosts (e.g. `0.0.0.0`), set:
  - `GRNS_ALLOW_REMOTE=true`

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

### Concurrency limiting for expensive endpoints

Server-side concurrency caps are applied to:
- import
- export
- heavy list queries (`search` / `spec` regex)

Excess concurrent requests receive HTTP `429` with `resource_exhausted` and numeric `error_code`.

## Error exposure

For server-side internal errors (`5xx`):
- API response message is sanitized to `"internal error"`
- detailed root causes are logged server-side.

## Operational recommendations

- Keep API bound to loopback unless remote access is explicitly required.
- Use strong random values for `GRNS_API_TOKEN` and `GRNS_ADMIN_TOKEN`.
- If exposing remotely, run behind TLS termination/proxy.
- Rotate tokens periodically and after suspected compromise.
