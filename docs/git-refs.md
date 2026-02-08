# Grns Task ↔ Git References Design (Draft)

## Summary
Grns should model Git relationships as first-class, typed references attached to tasks.

Use a dedicated `task_git_refs` table (task-only scope), plus a small `git_repos` catalog table for canonical repository identity.

This supports workflows like:
- linking a task to a design doc path in a repo,
- recording the commit that implemented or closed a task,
- querying all tasks tied to a given commit/branch/path.

---

## Why this design
String fields like `notes`, `design`, or ad-hoc `custom` keys are not queryable or consistent enough for provenance.

A typed edge model gives:
- clear semantics (`relation`),
- stable object identity (`object_type` + `object_value`),
- fast querying and indexing,
- compatibility with current architecture (service-layer validation, store abstraction).

---

## Goals (MVP)
1. Model task-to-git links explicitly and queryably.
2. Support common object types: commit, tag, branch, path, blob, tree.
3. Keep task list/read performance unchanged unless git refs are explicitly requested.
4. Preserve backward compatibility with existing `tasks.source_repo`.
5. Keep handlers thin; validation/orchestration in service layer.

## Non-goals (MVP)
- Generic polymorphic refs for all entity types (`item_git_refs`).
- Git object existence verification against remote hosts.
- PR/MR native objects (can be represented as `external_url` attachment or metadata initially).
- Automatic sync with repository history.

---

## Domain model

### 1) Repository catalog (`git_repos`)
Canonical identity for a repository.

Fields:
- `id` (`rp-xxxx`, 4-char base36)
- `slug` (unique canonical form, e.g. `github.com/org/repo`)
- `default_branch` (optional)
- `created_at`, `updated_at`

Notes:
- Canonicalization occurs in service layer.
- `tasks.source_repo` remains as task-level default context.

### 2) Task Git reference (`task_git_refs`)
One typed edge from a task to a git object.

Fields:
- `id` (`gf-...`)
- `task_id` (FK -> `tasks.id`)
- `repo_id` (FK -> `git_repos.id`)
- `relation` (domain meaning)
- `object_type` (`commit`, `tag`, `branch`, `path`, `blob`, `tree`)
- `object_value` (sha/ref/path)
- `resolved_commit` (optional immutable commit snapshot; required for strong reproducibility when object is mutable)
- `note` (optional free text)
- `meta_json` (optional JSON map)
- `created_at`, `updated_at`

### Relation vocabulary (initial)
Recommended built-ins:
- `design_doc`
- `implements`
- `fix_commit`
- `closed_by`
- `introduced_by`
- `related`

Validation approach:
- Service accepts built-ins and optionally `x-<token>` for team-specific extension.
- Store keeps relation as text (no DB enum check for relation) to avoid schema churn.

---

## Interaction with existing task fields

### `tasks.source_repo`
Keep it. It remains the **default repo context** for a task.

When creating a git ref:
- if `repo` is provided -> use it,
- else if task has `source_repo` -> use that,
- else reject (`repo is required`).

### Attachments (`source_type=repo_path`)
Keep attachments for rich artifact metadata/lifecycle (labels/media/expires/etc.).

Use `task_git_refs` for lightweight provenance/traceability edges.

Guideline:
- Need only a link/provenance edge -> `task_git_refs`.
- Need artifact metadata and attachment UX -> `attachments`.

---

## Schema and migration plan

## Migration version
- **Version:** `6`
- **Description:** `git refs: add git_repos and task_git_refs tables`

### SQL (proposed)
```sql
CREATE TABLE IF NOT EXISTS git_repos (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  default_branch TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS task_git_refs (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  repo_id TEXT NOT NULL,
  relation TEXT NOT NULL,
  object_type TEXT NOT NULL,
  object_value TEXT NOT NULL,
  resolved_commit TEXT,
  note TEXT,
  meta_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,

  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  FOREIGN KEY (repo_id) REFERENCES git_repos(id) ON DELETE RESTRICT,

  CHECK (object_type IN ('commit', 'tag', 'branch', 'path', 'blob', 'tree')),
  CHECK (length(trim(object_value)) > 0),
  CHECK (
    resolved_commit IS NULL OR
    (length(resolved_commit) = 40 AND resolved_commit GLOB '[0-9a-f]*')
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_task_git_refs_dedupe
  ON task_git_refs(task_id, repo_id, relation, object_type, object_value, ifnull(resolved_commit, ''));

CREATE INDEX IF NOT EXISTS idx_task_git_refs_task_created
  ON task_git_refs(task_id, created_at);

CREATE INDEX IF NOT EXISTS idx_task_git_refs_repo_object
  ON task_git_refs(repo_id, object_type, object_value);

CREATE INDEX IF NOT EXISTS idx_task_git_refs_relation
  ON task_git_refs(relation);
```

---

## Service boundaries

Add `TaskGitRefService` in `internal/server`.

Responsibilities:
- validate task existence,
- canonicalize repo slug,
- resolve repo default from `task.source_repo`,
- validate object-type/object-value shape,
- apply relation policy,
- orchestrate repo upsert + ref insert,
- map errors to `badRequest` / `notFound` / `conflict`.

Handlers must call service, not store directly.

---

## Store interfaces (proposed)

`internal/store/git_refs_interface.go`:

```go
type GitRefStore interface {
    UpsertGitRepo(ctx context.Context, repo *models.GitRepo) (*models.GitRepo, error)
    GetGitRepoBySlug(ctx context.Context, slug string) (*models.GitRepo, error)

    CreateTaskGitRef(ctx context.Context, ref *models.TaskGitRef) error
    ListTaskGitRefs(ctx context.Context, taskID string) ([]models.TaskGitRef, error)
    DeleteTaskGitRef(ctx context.Context, id string) error
    GetTaskGitRef(ctx context.Context, id string) (*models.TaskGitRef, error)
}
```

---

## API surface (proposed)

Base `/v1`:
- `POST /tasks/{id}/git-refs`
- `GET /tasks/{id}/git-refs`
- `GET /git-refs/{ref_id}`
- `DELETE /git-refs/{ref_id}`

### Create request
```json
{
  "repo": "github.com/org/repo",
  "relation": "design_doc",
  "object_type": "path",
  "object_value": "docs/design.md",
  "resolved_commit": "a3f0...40hex",
  "note": "Primary design source"
}
```

Rules:
- `repo` optional if task has `source_repo`.
- `resolved_commit` optional but recommended for mutable refs (`branch`, `tag`, `path`).

### Optional close integration
Extend close API payload with optional annotation:
```json
{
  "ids": ["gr-ab12"],
  "commit": "40hexsha",
  "repo": "github.com/org/repo"
}
```
On successful close, service creates a `closed_by` commit ref.

---

## CLI surface (proposed)

- `grns git add <task-id> --relation <rel> --type <object_type> --value <object_value> [--repo <slug>] [--resolved-commit <sha>] [--note ...]`
- `grns git ls <task-id> [--json]`
- `grns git rm <ref-id>`

Optional convenience:
- `grns close <id> --commit <sha> [--repo <slug>]`

---

## Validation rules

- Task id follows existing task-id validation.
- Repo slug canonicalized to `host/owner/repo` shape.
- `object_type=commit|blob|tree` requires lowercase 40-hex in `object_value`.
- `resolved_commit` if provided must be lowercase 40-hex.
- `object_type=path` must be relative path (no leading `/`, no `..` traversal after clean).
- `relation` lowercased, deduped whitespace, `x-...` allowed for extensions.

---

## Performance expectations

- No changes to existing `tasks` list query path.
- Git refs fetched only via dedicated endpoints (or explicit include in future).
- Indexed lookup by task and by repo/object for cross-reference queries.

---

## Rollout plan

### Phase 1
1. Add migration v6.
2. Add models: `GitRepo`, `TaskGitRef`.
3. Add `GitRefStore` methods in SQLite store.
4. Add `TaskGitRefService` + handlers.
5. Add CLI `grns git add|ls|rm`.
6. Add tests (store/service/integration).

### Phase 2
- Add `close --commit` annotation path.
- Add query/filter support (`list --repo`, `list --commit`) if needed.

---

## Test plan

### Store tests
- repo upsert/idempotency by slug,
- create/list/get/delete refs,
- uniqueness conflict behavior,
- FK cascade on task delete,
- object type and hash constraint checks.

### Service tests
- repo fallback from `task.source_repo`,
- relation and object validation,
- error mapping (`badRequest`, `notFound`, `conflict`),
- path safety checks.

### Integration (BATS)
- create task -> add git ref -> list git refs,
- close with commit annotation (phase 2),
- duplicate ref returns conflict,
- invalid repo/object returns structured error.

---

## Recommendation
Implement `task_git_refs` now (task-scoped, strongly validated) with a small `git_repos` catalog. This is the best balance of correctness, queryability, and low complexity for current Grns scope.