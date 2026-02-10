# Plan: Fix High + Medium Review Findings (feat/ui)

## Objective
Address all **high** and **medium** findings from the review in a way that:
- restores expected CLI usability,
- makes auth behavior explicit and testable,
- hardens login endpoints,
- and closes test/documentation gaps.

---

## Issues in scope

| ID | Severity | Problem summary |
|---|---|---|
| CR-1 | High | Adding an admin user can lock out normal CLI usage (`/v1/*` requires auth, CLI has no session flow). |
| CR-2 | Medium | Username validator unintentionally rejects 2-character usernames. |
| CR-3 | Medium | UI “Sign out” does not clear localStorage bearer fallback token. |
| CR-4 | Medium | Docs/auth policy mismatch (API token behavior vs session acceptance). |
| PF-1 | Medium | Extra auth-required DB check on every `/v1/*` request; duplicate check in `/v1/auth/me`. |
| SC-1 | Medium | No brute-force/rate limiting on `/v1/auth/login`. |
| TQ-1 | Medium | Missing regression tests for auth policy + CLI/bearer/session behavior. |

---

## Implementation plan

## Phase 1 — Auth policy + CLI compatibility (CR-1, CR-4, PF-1, TQ-1)

### 1) Make auth enforcement with admin users explicit and opt-in
**Goal:** prevent accidental CLI lockout after `grns admin user add`.

- Add new server env flag: `GRNS_REQUIRE_AUTH_WITH_USERS` (default: `false`).
- Update `apiAuthRequired()` logic (`internal/server/routes.go`):
  1. If `GRNS_API_TOKEN` is set => auth required.
  2. Else if `GRNS_REQUIRE_AUTH_WITH_USERS=true` and enabled users exist => auth required.
  3. Else => auth not required.

**Why:** preserves backward-compatible CLI behavior by default while still allowing strict user/session mode when explicitly enabled.

### 2) Unify and document auth method policy
**Policy to codify:** when auth is required, accept **either** valid bearer token **or** valid session cookie.

- Keep current implementation behavior (bearer OR session).
- Update docs (`README.md`, `docs/security.md`, `docs/api.md`) to match this exactly.
- Add explicit examples for both modes:
  - token-only workflows,
  - user-session strict mode (`GRNS_REQUIRE_AUTH_WITH_USERS=true`).

### 3) Remove duplicate auth-required checks per request
- Add request-context field for resolved `authRequired` in middleware.
- `handleAuthMe` should read request-scoped value instead of recalculating.
- Add tiny TTL cache for enabled-user count inside auth service/middleware path (e.g., 2–5s) to reduce DB query frequency when strict user mode is enabled.

### 4) Tests for policy and compatibility
Add/extend tests in `internal/server/*_test.go`:
- Default mode + admin users + no API token => `/v1/info` remains accessible (CLI compatibility).
- `GRNS_REQUIRE_AUTH_WITH_USERS=true` + admin users => `/v1/info` requires auth.
- With auth required, bearer succeeds.
- With auth required, session cookie succeeds.
- `/v1/auth/me` does not perform duplicate auth-required store checks (unit/mocked assertion if practical).

---

## Phase 2 — Validation + UI sign-out correctness (CR-2, CR-3, TQ-1)

### 5) Fix username validation edge case
**File:** `internal/auth/password.go`

- Adjust username validation so 2-character usernames are accepted.
- Prefer explicit min/max length checks plus simpler pattern to avoid hidden regex edge behavior.

**Add tests (`internal/auth/password_test.go`):**
- valid 2-char username (`ab`),
- max-length boundary,
- invalid leading/trailing punctuation,
- invalid characters.

### 6) Make sign-out fully sign out bearer fallback users
**Files:** `ui/src/App.jsx`, `ui/src/api.js`

- On sign-out, clear `localStorage['grns_api_token']` in addition to calling `/v1/auth/logout`.
- Keep logout flow resilient even if server logout fails.

**Add UI tests (`ui/src/App.test.jsx`):**
- with `grns_api_token` present, clicking “Sign out” removes token,
- subsequent API calls are unauthenticated (mock assertion).

---

## Phase 3 — Login hardening (SC-1, TQ-1)

### 7) Add login rate limiting / backoff
**Files:** `internal/server/handlers_auth.go`, `internal/server/auth_service.go` (or dedicated limiter file)

Implement lightweight in-memory protection for `POST /v1/auth/login`:
- key by `(normalized username, client IP)`,
- cap failed attempts within a rolling window,
- return `429` with structured error on limit exceeded,
- reset counters on successful login,
- opportunistic cleanup of stale limiter entries.

### 8) Add auth hardening tests
- repeated bad logins hit 429,
- successful login resets limiter for that key,
- limiter does not leak details about account existence.

---

## Documentation updates

Update these docs after implementation:
- `README.md` auth section (including strict mode env var and examples),
- `docs/security.md` auth enforcement behavior,
- `docs/api.md` auth semantics,
- optional: `docs/ui.md` runtime auth mode notes.

---

## Verification checklist

- `go test ./...` passes.
- `cd ui && npm test` passes.
- Manual smoke:
  1. No token, add admin user, default mode => CLI still works.
  2. Enable `GRNS_REQUIRE_AUTH_WITH_USERS=true` => unauthenticated `/v1/*` blocked.
  3. Session login works in browser; logout clears cookie + local token fallback.
  4. Repeated failed logins trigger rate limit.

---

## Suggested commit sequence

1. `fix(auth): make user-based auth enforcement explicit via GRNS_REQUIRE_AUTH_WITH_USERS`
2. `perf(auth): avoid duplicate auth-required checks and add short TTL cache`
3. `fix(auth): align docs with bearer-or-session policy`
4. `fix(auth): accept 2-char usernames and add boundary tests`
5. `fix(ui): clear local bearer token on sign out`
6. `fix(security): add login rate limiting/backoff`
7. `test(auth): add regression tests for CLI/session/bearer and limiter behavior`
