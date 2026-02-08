# Grns Systematic Error Codes Design

Status: Implemented
Author: Agent  
Date: 2026-02-07

## 1) Abstract

This design introduces a **systematic numeric error code** system for Grns API/CLI while preserving existing behavior.

Goals:
- machine-parseable errors (no brittle string parsing)
- backwards compatibility with existing clients
- predictable mapping from failure class to code range
- clear rollout path with low risk

Non-goals (initial phase):
- changing success payloads
- changing HTTP status semantics
- changing existing `code` string values

---

## 2) Current state

Today, API errors are returned as:

```json
{
  "error": "...",
  "code": "invalid_argument"
}
```

`code` is currently string-based and coarse (`invalid_argument`, `not_found`, `conflict`, `internal`, etc.).
Many call sites return plain `fmt.Errorf(...)` with a status inferred externally.

Impact:
- clients often must parse message text for precise handling
- difficult to enforce consistency across handlers
- limited observability for automation

---

## 3) Design principles

1. **Backward compatible**
   - Keep `error` and existing string `code`
   - Add numeric `error_code` as an optional additive field

2. **Centralized catalog**
   - Single source of truth for codes, names, and default status

3. **Layer-friendly**
   - Service returns typed API errors for domain/validation failures
   - Handler helper functions preserve typed metadata

4. **Incremental rollout**
   - Introduce framework first
   - Map high-traffic/high-value paths first
   - Expand coverage in follow-up changes

---

## 4) Error code structure

Use 4-digit integers by category:

| Range | Category | Meaning |
|---|---|---|
| 1xxx | Validation | Request malformed/invalid |
| 2xxx | Domain state | Not found/conflict/domain constraints |
| 3xxx | Auth/limits | Unauthorized/forbidden/rate-limited |
| 4xxx | Internal | Unexpected/store/runtime failures |

### Initial catalog (v1)

#### Validation (1xxx)
- `1000` ErrInvalidArgument (fallback for generic 400)
- `1001` ErrInvalidJSON
- `1002` ErrRequestTooLarge
- `1003` ErrInvalidQuery
- `1004` ErrInvalidID
- `1005` ErrInvalidStatus
- `1006` ErrInvalidType
- `1007` ErrInvalidPriority
- `1008` ErrInvalidLabel
- `1009` ErrMissingRequiredField
- `1010` ErrInvalidTimeFilter
- `1011` ErrInvalidImportMode
- `1012` ErrInvalidDependency
- `1013` ErrInvalidParentID
- `1014` ErrInvalidSearchQuery

#### Domain state (2xxx)
- `2001` ErrTaskNotFound
- `2002` ErrDependencyTaskNotFound
- `2101` ErrTaskIDExists
- `2102` ErrConflict (generic conflict fallback)

#### Auth/limits (3xxx)
- `3001` ErrUnauthorized
- `3002` ErrForbidden
- `3003` ErrResourceExhausted

#### Internal (4xxx)
- `4001` ErrInternal
- `4002` ErrStoreFailure
- `4003` ErrExportFailure
- `4004` ErrImportFailure

Notes:
- Catalog is extensible; adding new codes is non-breaking.
- Existing string code remains primary compatibility field.

---

## 5) API response changes

### Before

```json
{
  "error": "invalid id",
  "code": "invalid_argument"
}
```

### After (additive)

```json
{
  "error": "invalid id",
  "code": "invalid_argument",
  "error_code": 1004
}
```

### Type change

`internal/api/types.go`:

```go
type ErrorResponse struct {
    Error     string `json:"error"`
    Code      string `json:"code,omitempty"`
    ErrorCode int    `json:"error_code,omitempty"`
}
```

---

## 6) Server design changes

## 6.1 Central error definitions

Add a dedicated module (recommended: `internal/server/error_codes.go`) containing:
- numeric constants
- optional symbolic names
- helpers for defaults/fallbacks

## 6.2 Typed API error metadata

Extend existing `apiError` to carry numeric code:

- HTTP status
- string code (`invalid_argument`, etc.)
- numeric code (`error_code`)
- wrapped error

Provide constructors:
- `badRequestCode(err, code)`
- `notFoundCode(err, code)`
- `conflictCode(err, code)`
- preserve `badRequest/notFound/conflict` wrappers as fallbacks

## 6.3 Response writer

Update `writeErrorReq` to:
- extract string + numeric code from typed errors
- fallback by status when untyped
- keep 5xx message sanitization (`"internal error"`)
- include `error_code` in payload

## 6.4 Mapping strategy

Map precise locations first:

- `requirePathID` / `requireIDs` -> `1004`
- JSON decode failures -> `1001` (and `1002` for body-too-large where detectable)
- query parsing (`queryInt`, `queryBool`, list filter) -> `1003` / specialized 1xxx
- invalid status/type/priority/label -> dedicated 1xxx
- invalid search query detection -> `1014`
- service not found/conflict (`task not found`, unique constraint) -> `2001` / `2101`
- auth middleware -> `3001`/`3002`
- concurrency limiter 429 -> `3003`
- uncaught store/internal failures -> `4001` or `4002`

Important architectural rule remains unchanged: mutation logic stays in `TaskService`.

---

## 7) Client and CLI changes

## 7.1 API client

`internal/api/client.go`:
- parse `error_code` in `decodeError`
- return typed error (`*api.APIError`) with fields:
  - `Status`
  - `Code` (string)
  - `ErrorCode` (int)
  - `Message`

Keep `error` interface compatibility.

## 7.2 CLI output

Optionally improve human output while remaining stable:
- default: current message behavior
- enhanced format when structured code exists:
  - `E1004 invalid_argument: invalid id`

No command flag changes required for initial rollout.

---

## 8) Testing plan

### Unit tests

- `internal/server/*` tests:
  - assert `error_code` presence and expected value for representative scenarios
  - preserve existing status and message expectations
- `internal/api/client_test.go`:
  - decode typed API errors with both `code` and `error_code`

### Integration tests (BATS)

Add coverage for:
- invalid ID (400 + `1004`)
- not found (404 + `2001`)
- conflict create duplicate (409 + `2101`)
- unauthorized/forbidden (401/403 + `3001`/`3002`)
- rate-limited path (429 + `3003`)

### Regression checks

- `just test`
- `just test-integration`

---

## 9) Rollout plan (incremental)

### PR 1: Framework + docs
- add this design doc
- add `error_code` field to API type
- add central code catalog
- wire `writeErrorReq` fallbacks (no broad remapping yet)

### PR 2: Core path mappings
- map common validation/domain/auth/limit paths
- update server tests

### PR 3: Client typed errors + CLI polish
- typed decode in client
- optional CLI formatting updates
- client tests

### PR 4: Integration coverage + doc polish
- BATS coverage
- README/security docs update with brief examples

---

## 10) Compatibility and migration notes

- Existing clients that only read `error`/`code` continue to work unchanged.
- New clients should prefer `error_code` for stable branching logic.
- String `code` values remain supported for long-term compatibility.

---

## 11) Open questions

1. Should we reserve a success range (`0xxx`) now, or keep error-only in v1? (Recommendation: error-only v1)
2. Should CLI always print numeric code when present, or only in verbose/debug mode?
3. Do we want endpoint-specific subranges now, or defer until catalog grows?

---

## 12) Acceptance criteria

- API error payload includes `error_code` for mapped failures.
- Existing `code` and HTTP status behavior unchanged.
- At least one mapped code per category (1xxx/2xxx/3xxx/4xxx) covered by tests.
- Documentation includes catalog and migration guidance.
