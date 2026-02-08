package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"grns/internal/models"
)

const gitRepoColumns = "id, slug, default_branch, created_at, updated_at"
const taskGitRefColumns = "r.id, r.task_id, r.repo_id, g.slug, r.relation, r.object_type, r.object_value, r.resolved_commit, r.note, r.meta_json, r.created_at, r.updated_at"

// UpsertGitRepo inserts a canonical repo row if missing and returns it by slug.
func (s *Store) UpsertGitRepo(ctx context.Context, repo *models.GitRepo) (*models.GitRepo, error) {
	if repo == nil {
		return nil, fmt.Errorf("repo is required")
	}

	repo.Slug = strings.ToLower(strings.TrimSpace(repo.Slug))
	if repo.Slug == "" {
		return nil, fmt.Errorf("repo slug is required")
	}

	if strings.TrimSpace(repo.ID) == "" {
		id, err := GenerateGitRepoID(func(id string) (bool, error) {
			return s.gitRepoIDExists(ctx, id)
		})
		if err != nil {
			return nil, err
		}
		repo.ID = id
	}

	now := time.Now().UTC()
	if repo.CreatedAt.IsZero() {
		repo.CreatedAt = now
	}
	if repo.UpdatedAt.IsZero() {
		repo.UpdatedAt = now
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO git_repos (id, slug, default_branch, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(slug) DO UPDATE SET
			default_branch = COALESCE(excluded.default_branch, git_repos.default_branch),
			updated_at = excluded.updated_at
	`,
		repo.ID,
		repo.Slug,
		nullIfEmpty(strings.TrimSpace(repo.DefaultBranch)),
		dbFormatTime(repo.CreatedAt),
		dbFormatTime(repo.UpdatedAt),
	)
	if err != nil {
		return nil, err
	}

	return s.GetGitRepoBySlug(ctx, repo.Slug)
}

// GetGitRepoBySlug returns one repo by canonical slug.
func (s *Store) GetGitRepoBySlug(ctx context.Context, slug string) (*models.GitRepo, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+gitRepoColumns+` FROM git_repos WHERE slug = ?`, strings.ToLower(strings.TrimSpace(slug)))
	return scanGitRepo(row)
}

// CreateTaskGitRef inserts one task git reference row.
func (s *Store) CreateTaskGitRef(ctx context.Context, ref *models.TaskGitRef) error {
	if ref == nil {
		return fmt.Errorf("task git ref is required")
	}

	now := time.Now().UTC()
	if ref.CreatedAt.IsZero() {
		ref.CreatedAt = now
	}
	if ref.UpdatedAt.IsZero() {
		ref.UpdatedAt = ref.CreatedAt
	}

	metaJSON, err := taskGitRefMetaToJSON(ref.Meta)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO task_git_refs (
			id, task_id, repo_id, relation, object_type, object_value,
			resolved_commit, note, meta_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		ref.ID,
		ref.TaskID,
		ref.RepoID,
		ref.Relation,
		ref.ObjectType,
		ref.ObjectValue,
		nullIfEmpty(strings.TrimSpace(ref.ResolvedCommit)),
		nullIfEmpty(strings.TrimSpace(ref.Note)),
		metaJSON,
		dbFormatTime(ref.CreatedAt),
		dbFormatTime(ref.UpdatedAt),
	)
	return err
}

// GetTaskGitRef returns one task git reference row by id.
func (s *Store) GetTaskGitRef(ctx context.Context, project, id string) (*models.TaskGitRef, error) {
	project = normalizeProject(project)
	if project == "" {
		row := s.db.QueryRowContext(ctx, `
			SELECT `+taskGitRefColumns+`
			FROM task_git_refs r
			JOIN git_repos g ON g.id = r.repo_id
			WHERE r.id = ?
		`, id)
		return scanTaskGitRef(row)
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT `+taskGitRefColumns+`
		FROM task_git_refs r
		JOIN git_repos g ON g.id = r.repo_id
		JOIN tasks t ON t.id = r.task_id
		WHERE r.id = ? AND t.project_id = ?
	`, id, project)
	return scanTaskGitRef(row)
}

// ListTaskGitRefs lists git refs for one task ordered by created_at descending.
func (s *Store) ListTaskGitRefs(ctx context.Context, project, taskID string) ([]models.TaskGitRef, error) {
	project = normalizeProject(project)

	var (
		rows *sql.Rows
		err  error
	)
	if project == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+taskGitRefColumns+`
			FROM task_git_refs r
			JOIN git_repos g ON g.id = r.repo_id
			WHERE r.task_id = ?
			ORDER BY r.created_at DESC
		`, taskID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+taskGitRefColumns+`
			FROM task_git_refs r
			JOIN git_repos g ON g.id = r.repo_id
			JOIN tasks t ON t.id = r.task_id
			WHERE r.task_id = ? AND t.project_id = ?
			ORDER BY r.created_at DESC
		`, taskID, project)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	refs := []models.TaskGitRef{}
	for rows.Next() {
		ref, err := scanTaskGitRef(rows)
		if err != nil {
			return nil, err
		}
		if ref != nil {
			refs = append(refs, *ref)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

// DeleteTaskGitRef deletes one task git reference row.
func (s *Store) DeleteTaskGitRef(ctx context.Context, project, id string) error {
	project = normalizeProject(project)
	if project == "" {
		_, err := s.db.ExecContext(ctx, "DELETE FROM task_git_refs WHERE id = ?", id)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM task_git_refs
		WHERE id = ?
		AND task_id IN (SELECT id FROM tasks WHERE project_id = ?)
	`, id, project)
	return err
}

// CloseTasksWithGitRefs closes tasks and adds one git ref annotation per task in a single transaction.
// Duplicate annotations (same task/repo/relation/object/resolved_commit) are ignored.
func (s *Store) CloseTasksWithGitRefs(ctx context.Context, project string, ids []string, closedAt time.Time, refs []CloseTaskGitRefInput) (created int, err error) {
	project = normalizeProject(project)
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return 0, nil
	}

	refsByTask := make(map[string]CloseTaskGitRefInput, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.TaskID) == "" {
			return 0, fmt.Errorf("close git ref task_id is required")
		}
		refsByTask[ref.TaskID] = ref
	}
	for _, id := range ids {
		if _, ok := refsByTask[id]; !ok {
			return 0, fmt.Errorf("close git ref missing for task %s", id)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if project == "" {
		project = projectFromTaskID(ids[0])
	}
	for _, id := range ids {
		if projectFromTaskID(id) != project {
			return 0, ErrProjectMismatch
		}
	}

	existsCount, err := countExistingTasksInProject(ctx, tx, project, ids)
	if err != nil {
		return 0, err
	}
	if existsCount != len(ids) {
		return 0, ErrTaskNotFound
	}

	args := []any{string(models.StatusClosed), dbFormatTime(closedAt), dbFormatTime(closedAt), project}
	for _, id := range ids {
		args = append(args, id)
	}
	query := fmt.Sprintf("UPDATE tasks SET status = ?, closed_at = ?, updated_at = ? WHERE project_id = ? AND id IN (%s)", placeholders(len(ids)))
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return 0, err
	}

	for _, id := range ids {
		input := refsByTask[id]

		repo, err := upsertGitRepoTx(ctx, tx, &models.GitRepo{Slug: input.RepoSlug})
		if err != nil {
			return 0, err
		}
		if repo == nil {
			return 0, fmt.Errorf("git repo not found after upsert")
		}

		refID, err := GenerateTaskGitRefID(func(candidate string) (bool, error) {
			return taskGitRefIDExistsTx(ctx, tx, candidate)
		})
		if err != nil {
			return 0, err
		}

		metaJSON, err := taskGitRefMetaToJSON(input.Meta)
		if err != nil {
			return 0, err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO task_git_refs (
				id, task_id, repo_id, relation, object_type, object_value,
				resolved_commit, note, meta_json, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			refID,
			id,
			repo.ID,
			input.Relation,
			input.ObjectType,
			input.ObjectValue,
			nullIfEmpty(strings.TrimSpace(input.ResolvedCommit)),
			nullIfEmpty(strings.TrimSpace(input.Note)),
			metaJSON,
			dbFormatTime(closedAt),
			dbFormatTime(closedAt),
		)
		if err != nil {
			if isStoreUniqueConstraint(err) {
				continue
			}
			return 0, err
		}
		created++
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return created, nil
}

func (s *Store) gitRepoIDExists(ctx context.Context, id string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM git_repos WHERE id = ? LIMIT 1", id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func upsertGitRepoTx(ctx context.Context, tx *sql.Tx, repo *models.GitRepo) (*models.GitRepo, error) {
	if repo == nil {
		return nil, fmt.Errorf("repo is required")
	}

	repo.Slug = strings.ToLower(strings.TrimSpace(repo.Slug))
	if repo.Slug == "" {
		return nil, fmt.Errorf("repo slug is required")
	}

	if strings.TrimSpace(repo.ID) == "" {
		id, err := GenerateGitRepoID(func(id string) (bool, error) {
			var exists int
			err := tx.QueryRowContext(ctx, "SELECT 1 FROM git_repos WHERE id = ? LIMIT 1", id).Scan(&exists)
			if err == sql.ErrNoRows {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return true, nil
		})
		if err != nil {
			return nil, err
		}
		repo.ID = id
	}

	now := time.Now().UTC()
	if repo.CreatedAt.IsZero() {
		repo.CreatedAt = now
	}
	if repo.UpdatedAt.IsZero() {
		repo.UpdatedAt = now
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO git_repos (id, slug, default_branch, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(slug) DO UPDATE SET
			default_branch = COALESCE(excluded.default_branch, git_repos.default_branch),
			updated_at = excluded.updated_at
	`,
		repo.ID,
		repo.Slug,
		nullIfEmpty(strings.TrimSpace(repo.DefaultBranch)),
		dbFormatTime(repo.CreatedAt),
		dbFormatTime(repo.UpdatedAt),
	)
	if err != nil {
		return nil, err
	}

	row := tx.QueryRowContext(ctx, `SELECT `+gitRepoColumns+` FROM git_repos WHERE slug = ?`, repo.Slug)
	return scanGitRepo(row)
}

func taskGitRefIDExistsTx(ctx context.Context, tx *sql.Tx, id string) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM task_git_refs WHERE id = ? LIMIT 1", id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func isStoreUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

func scanGitRepo(scanner interface {
	Scan(dest ...any) error
}) (*models.GitRepo, error) {
	repo := models.GitRepo{}
	var defaultBranch sql.NullString
	var createdAt, updatedAt string

	err := scanner.Scan(&repo.ID, &repo.Slug, &defaultBranch, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	repo.DefaultBranch = defaultBranch.String
	parsedCreated, err := dbParseTime(createdAt)
	if err != nil {
		return nil, err
	}
	parsedUpdated, err := dbParseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	repo.CreatedAt = parsedCreated
	repo.UpdatedAt = parsedUpdated

	return &repo, nil
}

func scanTaskGitRef(scanner interface {
	Scan(dest ...any) error
}) (*models.TaskGitRef, error) {
	ref := models.TaskGitRef{}
	var resolvedCommit, note, metaJSON sql.NullString
	var createdAt, updatedAt string

	err := scanner.Scan(
		&ref.ID,
		&ref.TaskID,
		&ref.RepoID,
		&ref.Repo,
		&ref.Relation,
		&ref.ObjectType,
		&ref.ObjectValue,
		&resolvedCommit,
		&note,
		&metaJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	ref.Project = projectFromTaskID(ref.TaskID)
	ref.ResolvedCommit = resolvedCommit.String
	ref.Note = note.String

	parsedCreated, err := dbParseTime(createdAt)
	if err != nil {
		return nil, err
	}
	parsedUpdated, err := dbParseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	ref.CreatedAt = parsedCreated
	ref.UpdatedAt = parsedUpdated

	if metaJSON.Valid && metaJSON.String != "" {
		if err := json.Unmarshal([]byte(metaJSON.String), &ref.Meta); err != nil {
			return nil, fmt.Errorf("parse task git ref meta_json: %w", err)
		}
	}

	return &ref, nil
}

func taskGitRefMetaToJSON(meta map[string]any) (any, error) {
	if len(meta) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal task git ref meta_json: %w", err)
	}
	return string(data), nil
}
