package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"grns/internal/models"
)

const taskColumns = "id, title, status, type, priority, description, spec_id, parent_id, assignee, notes, design, acceptance_criteria, source_repo, created_at, updated_at, closed_at, custom"
const qualifiedTaskColumns = "tasks.id, tasks.title, tasks.status, tasks.type, tasks.priority, tasks.description, tasks.spec_id, tasks.parent_id, tasks.assignee, tasks.notes, tasks.design, tasks.acceptance_criteria, tasks.source_repo, tasks.created_at, tasks.updated_at, tasks.closed_at, tasks.custom"

type ListFilter struct {
	Statuses         []string
	Types            []string
	Priority         *int
	PriorityMin      *int
	PriorityMax      *int
	ParentID         string
	Labels           []string
	LabelsAny        []string
	SpecRegex        string
	Assignee         string
	NoAssignee       bool
	IDs              []string
	TitleContains    string
	DescContains     string
	NotesContains    string
	CreatedAfter     *time.Time
	CreatedBefore    *time.Time
	UpdatedAfter     *time.Time
	UpdatedBefore    *time.Time
	ClosedAfter      *time.Time
	ClosedBefore     *time.Time
	EmptyDescription bool
	NoLabels         bool
	SearchQuery      string
	Limit            int
	Offset           int
}

// CreateTask inserts a task with optional labels and dependencies.
func (s *Store) CreateTask(ctx context.Context, task *models.Task, labels []string, deps []models.Dependency) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO tasks (
			id, title, status, type, priority, description, spec_id, parent_id,
			assignee, notes, design, acceptance_criteria, source_repo,
			created_at, updated_at, closed_at, custom
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		task.ID,
		task.Title,
		task.Status,
		task.Type,
		task.Priority,
		nullIfEmpty(task.Description),
		nullIfEmpty(task.SpecID),
		nullIfEmpty(task.ParentID),
		nullIfEmpty(task.Assignee),
		nullIfEmpty(task.Notes),
		nullIfEmpty(task.Design),
		nullIfEmpty(task.AcceptanceCriteria),
		nullIfEmpty(task.SourceRepo),
		dbFormatTime(task.CreatedAt),
		dbFormatTime(task.UpdatedAt),
		nullTime(task.ClosedAt),
		customToJSON(task.Custom),
	)
	if err != nil {
		return err
	}

	if err = insertLabels(ctx, tx, task.ID, labels); err != nil {
		return err
	}
	if err = insertDeps(ctx, tx, task.ID, deps); err != nil {
		return err
	}

	return tx.Commit()
}

// GetTask returns a task by id.
func (s *Store) GetTask(ctx context.Context, id string) (*models.Task, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+taskColumns+`
		FROM tasks WHERE id = ?
	`, id)
	return scanTask(row)
}

// UpdateTask updates mutable fields on a task.
func (s *Store) UpdateTask(ctx context.Context, id string, update TaskUpdate) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}

	set := []string{}
	args := []any{}

	if update.Title != nil {
		set = append(set, "title = ?")
		args = append(args, *update.Title)
	}
	if update.Status != nil {
		set = append(set, "status = ?")
		args = append(args, *update.Status)
	}
	if update.Type != nil {
		set = append(set, "type = ?")
		args = append(args, *update.Type)
	}
	if update.Priority != nil {
		set = append(set, "priority = ?")
		args = append(args, *update.Priority)
	}
	if update.Description != nil {
		set = append(set, "description = ?")
		args = append(args, nullIfEmpty(*update.Description))
	}
	if update.SpecID != nil {
		set = append(set, "spec_id = ?")
		args = append(args, nullIfEmpty(*update.SpecID))
	}
	if update.ParentID != nil {
		set = append(set, "parent_id = ?")
		args = append(args, nullIfEmpty(*update.ParentID))
	}
	if update.Assignee != nil {
		set = append(set, "assignee = ?")
		args = append(args, nullIfEmpty(*update.Assignee))
	}
	if update.Notes != nil {
		set = append(set, "notes = ?")
		args = append(args, nullIfEmpty(*update.Notes))
	}
	if update.Design != nil {
		set = append(set, "design = ?")
		args = append(args, nullIfEmpty(*update.Design))
	}
	if update.AcceptanceCriteria != nil {
		set = append(set, "acceptance_criteria = ?")
		args = append(args, nullIfEmpty(*update.AcceptanceCriteria))
	}
	if update.SourceRepo != nil {
		set = append(set, "source_repo = ?")
		args = append(args, nullIfEmpty(*update.SourceRepo))
	}
	if update.ClosedAt != nil {
		set = append(set, "closed_at = ?")
		args = append(args, nullTime(update.ClosedAt))
	}
	if update.Custom != nil {
		set = append(set, "custom = ?")
		args = append(args, customToJSON(*update.Custom))
	}

	set = append(set, "updated_at = ?")
	args = append(args, dbFormatTime(update.UpdatedAt))

	args = append(args, id)
	query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = ?", strings.Join(set, ", "))
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// ListTasks returns tasks matching the provided filter.
func (s *Store) ListTasks(ctx context.Context, filter ListFilter) ([]models.Task, error) {
	query, args := buildListQuery(filter)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if filter.SpecRegex != "" {
		return filterRowsBySpecRegex(rows, filter.SpecRegex, filter.Limit, filter.Offset)
	}

	var tasks []models.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

// ListReadyTasks returns tasks with no open blockers.
func (s *Store) ListReadyTasks(ctx context.Context, limit int) ([]models.Task, error) {
	args := []any{}
	query := `
		SELECT ` + taskColumns + `
		FROM tasks t
		WHERE t.status IN ('open', 'in_progress', 'blocked', 'deferred', 'pinned')
		AND NOT EXISTS (
			SELECT 1 FROM task_deps d
			JOIN tasks p ON p.id = d.parent_id
			WHERE d.child_id = t.id
			AND d.type = 'blocks'
			AND p.status IN ('open', 'in_progress', 'blocked', 'deferred', 'pinned')
		)
		ORDER BY updated_at DESC
	`
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *task)
	}
	return tasks, rows.Err()
}

// ListStaleTasks returns tasks not updated since cutoff.
func (s *Store) ListStaleTasks(ctx context.Context, cutoff time.Time, statuses []string, limit int) ([]models.Task, error) {
	args := []any{dbFormatTime(cutoff)}
	where := []string{"updated_at < ?"}

	if len(statuses) > 0 {
		where = append(where, fmt.Sprintf("status IN (%s)", placeholders(len(statuses))))
		for _, status := range statuses {
			args = append(args, status)
		}
	} else {
		where = append(where, "status NOT IN ('closed', 'tombstone')")
	}

	query := fmt.Sprintf(`
		SELECT `+taskColumns+`
		FROM tasks
		WHERE %s
		ORDER BY updated_at ASC
	`, strings.Join(where, " AND "))

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *task)
	}
	return tasks, rows.Err()
}

// AddLabels adds labels to a task.
func (s *Store) AddLabels(ctx context.Context, id string, labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO task_labels (task_id, label) VALUES "+labelValues(len(labels)), labelArgs(id, labels)...)
	return err
}

// RemoveLabels removes labels from a task.
func (s *Store) RemoveLabels(ctx context.Context, id string, labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	args := []any{id}
	for _, label := range labels {
		args = append(args, label)
	}
	query := fmt.Sprintf("DELETE FROM task_labels WHERE task_id = ? AND label IN (%s)", placeholders(len(labels)))
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// ListLabels returns labels for a task.
func (s *Store) ListLabels(ctx context.Context, id string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT label FROM task_labels WHERE task_id = ? ORDER BY label ASC", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := []string{}
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}

// ListAllLabels returns all labels in the database.
func (s *Store) ListAllLabels(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT label FROM task_labels ORDER BY label ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := []string{}
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}

// AddDependency adds a dependency edge between tasks.
func (s *Store) AddDependency(ctx context.Context, childID, parentID, depType string) error {
	_, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO task_deps (child_id, parent_id, type) VALUES (?, ?, ?)", childID, parentID, depType)
	return err
}

// ListDependencies returns dependencies where the task is the child.
func (s *Store) ListDependencies(ctx context.Context, id string) ([]models.Dependency, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT parent_id, type FROM task_deps WHERE child_id = ? ORDER BY parent_id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []models.Dependency
	for rows.Next() {
		var dep models.Dependency
		if err := rows.Scan(&dep.ParentID, &dep.Type); err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}
	return deps, rows.Err()
}

// ListLabelsForTasks returns labels mapped by task id.
func (s *Store) ListLabelsForTasks(ctx context.Context, ids []string) (map[string][]string, error) {
	labels := make(map[string][]string)
	if len(ids) == 0 {
		return labels, nil
	}

	query := fmt.Sprintf("SELECT task_id, label FROM task_labels WHERE task_id IN (%s)", placeholders(len(ids)))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var taskID, label string
		if err := rows.Scan(&taskID, &label); err != nil {
			return nil, err
		}
		labels[taskID] = append(labels[taskID], label)
	}

	for _, list := range labels {
		sort.Strings(list)
	}

	return labels, rows.Err()
}

// ListDependenciesForTasks returns dependencies keyed by child task id.
func (s *Store) ListDependenciesForTasks(ctx context.Context, ids []string) (map[string][]models.Dependency, error) {
	deps := make(map[string][]models.Dependency)
	if len(ids) == 0 {
		return deps, nil
	}

	query := fmt.Sprintf("SELECT child_id, parent_id, type FROM task_deps WHERE child_id IN (%s)", placeholders(len(ids)))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var childID, parentID, depType string
		if err := rows.Scan(&childID, &parentID, &depType); err != nil {
			return nil, err
		}
		deps[childID] = append(deps[childID], models.Dependency{ParentID: parentID, Type: depType})
	}
	return deps, rows.Err()
}

// ReplaceLabels replaces all labels for a task.
func (s *Store) ReplaceLabels(ctx context.Context, id string, labels []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, "DELETE FROM task_labels WHERE task_id = ?", id); err != nil {
		return err
	}
	if err = insertLabels(ctx, tx, id, labels); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveDependencies removes all dependencies where the task is a child.
func (s *Store) RemoveDependencies(ctx context.Context, childID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM task_deps WHERE child_id = ?", childID)
	return err
}

// CloseTasks closes tasks and sets closed_at.
func (s *Store) CloseTasks(ctx context.Context, ids []string, closedAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}

	args := []any{dbFormatTime(closedAt), dbFormatTime(closedAt)}
	for _, id := range ids {
		args = append(args, id)
	}
	query := fmt.Sprintf("UPDATE tasks SET status = 'closed', closed_at = ?, updated_at = ? WHERE id IN (%s)", placeholders(len(ids)))
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// ReopenTasks reopens tasks and clears closed_at.
func (s *Store) ReopenTasks(ctx context.Context, ids []string, reopenedAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}

	args := []any{dbFormatTime(reopenedAt)}
	for _, id := range ids {
		args = append(args, id)
	}
	query := fmt.Sprintf("UPDATE tasks SET status = 'open', closed_at = NULL, updated_at = ? WHERE id IN (%s)", placeholders(len(ids)))
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// TaskUpdate describes fields to update.
type TaskUpdate struct {
	Title              *string
	Status             *string
	Type               *string
	Priority           *int
	Description        *string
	SpecID             *string
	ParentID           *string
	Assignee           *string
	Notes              *string
	Design             *string
	AcceptanceCriteria *string
	SourceRepo         *string
	ClosedAt           *time.Time
	Custom             *map[string]any
	UpdatedAt          time.Time
}

func buildListQuery(filter ListFilter) (string, []any) {
	args := []any{}
	query := "SELECT " + taskColumns + " FROM tasks"
	if filter.SearchQuery != "" {
		query = "SELECT " + qualifiedTaskColumns + " FROM tasks JOIN tasks_fts ON tasks.id = tasks_fts.task_id AND tasks_fts MATCH ?"
		args = append(args, filter.SearchQuery)
	}
	where := []string{}

	if len(filter.Statuses) > 0 {
		where = append(where, fmt.Sprintf("status IN (%s)", placeholders(len(filter.Statuses))))
		for _, status := range filter.Statuses {
			args = append(args, status)
		}
	}
	if len(filter.Types) > 0 {
		where = append(where, fmt.Sprintf("type IN (%s)", placeholders(len(filter.Types))))
		for _, t := range filter.Types {
			args = append(args, t)
		}
	}
	if filter.Priority != nil {
		where = append(where, "priority = ?")
		args = append(args, *filter.Priority)
	}
	if filter.PriorityMin != nil {
		where = append(where, "priority >= ?")
		args = append(args, *filter.PriorityMin)
	}
	if filter.PriorityMax != nil {
		where = append(where, "priority <= ?")
		args = append(args, *filter.PriorityMax)
	}
	if filter.ParentID != "" {
		where = append(where, "parent_id = ?")
		args = append(args, filter.ParentID)
	}
	if len(filter.Labels) > 0 {
		where = append(where, fmt.Sprintf("id IN (SELECT task_id FROM task_labels WHERE label IN (%s) GROUP BY task_id HAVING COUNT(DISTINCT label) = %d)", placeholders(len(filter.Labels)), len(filter.Labels)))
		for _, label := range filter.Labels {
			args = append(args, label)
		}
	}
	if len(filter.LabelsAny) > 0 {
		where = append(where, fmt.Sprintf("id IN (SELECT task_id FROM task_labels WHERE label IN (%s))", placeholders(len(filter.LabelsAny))))
		for _, label := range filter.LabelsAny {
			args = append(args, label)
		}
	}
	if filter.Assignee != "" {
		where = append(where, "assignee = ?")
		args = append(args, filter.Assignee)
	}
	if filter.NoAssignee {
		where = append(where, "(assignee IS NULL OR assignee = '')")
	}
	if len(filter.IDs) > 0 {
		where = append(where, fmt.Sprintf("id IN (%s)", placeholders(len(filter.IDs))))
		for _, id := range filter.IDs {
			args = append(args, id)
		}
	}
	if filter.TitleContains != "" {
		where = append(where, "tasks.title LIKE '%' || ? || '%'")
		args = append(args, filter.TitleContains)
	}
	if filter.DescContains != "" {
		where = append(where, "tasks.description LIKE '%' || ? || '%'")
		args = append(args, filter.DescContains)
	}
	if filter.NotesContains != "" {
		where = append(where, "tasks.notes LIKE '%' || ? || '%'")
		args = append(args, filter.NotesContains)
	}
	if filter.CreatedAfter != nil {
		where = append(where, "created_at > ?")
		args = append(args, dbFormatTime(*filter.CreatedAfter))
	}
	if filter.CreatedBefore != nil {
		where = append(where, "created_at < ?")
		args = append(args, dbFormatTime(*filter.CreatedBefore))
	}
	if filter.UpdatedAfter != nil {
		where = append(where, "updated_at > ?")
		args = append(args, dbFormatTime(*filter.UpdatedAfter))
	}
	if filter.UpdatedBefore != nil {
		where = append(where, "updated_at < ?")
		args = append(args, dbFormatTime(*filter.UpdatedBefore))
	}
	if filter.ClosedAfter != nil {
		where = append(where, "closed_at > ?")
		args = append(args, dbFormatTime(*filter.ClosedAfter))
	}
	if filter.ClosedBefore != nil {
		where = append(where, "closed_at < ?")
		args = append(args, dbFormatTime(*filter.ClosedBefore))
	}
	if filter.EmptyDescription {
		where = append(where, "(tasks.description IS NULL OR tasks.description = '')")
	}
	if filter.NoLabels {
		where = append(where, "id NOT IN (SELECT task_id FROM task_labels)")
	}

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	if filter.SearchQuery != "" {
		query += " ORDER BY tasks_fts.rank"
	} else {
		query += " ORDER BY updated_at DESC"
	}

	if filter.SpecRegex == "" {
		hasLimit := false
		if filter.Limit > 0 {
			query += " LIMIT ?"
			args = append(args, filter.Limit)
			hasLimit = true
		}
		if filter.Offset > 0 {
			if !hasLimit {
				query += " LIMIT -1"
			}
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}

	return query, args
}

func filterRowsBySpecRegex(rows *sql.Rows, pattern string, limit, offset int) ([]models.Task, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	filtered := []models.Task{}
	skipped := 0
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		if task.SpecID == "" || !re.MatchString(task.SpecID) {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		filtered = append(filtered, *task)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return filtered, nil
}

func scanTask(scanner interface {
	Scan(dest ...any) error
}) (*models.Task, error) {
	var task models.Task
	var description, specID, parentID sql.NullString
	var assignee, notes, design, acceptanceCriteria, sourceRepo sql.NullString
	var createdAt, updatedAt string
	var closedAt, customJSON sql.NullString

	if err := scanner.Scan(
		&task.ID,
		&task.Title,
		&task.Status,
		&task.Type,
		&task.Priority,
		&description,
		&specID,
		&parentID,
		&assignee,
		&notes,
		&design,
		&acceptanceCriteria,
		&sourceRepo,
		&createdAt,
		&updatedAt,
		&closedAt,
		&customJSON,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	task.Description = description.String
	task.SpecID = specID.String
	task.ParentID = parentID.String
	task.Assignee = assignee.String
	task.Notes = notes.String
	task.Design = design.String
	task.AcceptanceCriteria = acceptanceCriteria.String
	task.SourceRepo = sourceRepo.String

	parsedCreated, err := dbParseTime(createdAt)
	if err != nil {
		return nil, err
	}
	parsedUpdated, err := dbParseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	task.CreatedAt = parsedCreated
	task.UpdatedAt = parsedUpdated
	if closedAt.Valid {
		parsedClosed, err := dbParseTime(closedAt.String)
		if err != nil {
			return nil, err
		}
		task.ClosedAt = &parsedClosed
	}
	if customJSON.Valid && customJSON.String != "" {
		if err := json.Unmarshal([]byte(customJSON.String), &task.Custom); err != nil {
			return nil, fmt.Errorf("parse custom JSON: %w", err)
		}
	}

	return &task, nil
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func customToJSON(m map[string]any) any {
	if len(m) == 0 {
		return nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return string(data)
}

func nullTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return dbFormatTime(*value)
}

func dbFormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func dbParseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func labelValues(count int) string {
	values := make([]string, count)
	for i := 0; i < count; i++ {
		values[i] = "(?, ?)"
	}
	return strings.Join(values, ",")
}

func labelArgs(id string, labels []string) []any {
	args := make([]any, 0, len(labels)*2)
	for _, label := range labels {
		args = append(args, id, label)
	}
	return args
}

func insertLabels(ctx context.Context, tx *sql.Tx, id string, labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO task_labels (task_id, label) VALUES "+labelValues(len(labels)), labelArgs(id, labels)...)
	return err
}

// DependencyTree returns the full dependency graph for a task.
func (s *Store) DependencyTree(ctx context.Context, id string) ([]models.DepTreeNode, error) {
	query := `
		WITH RECURSIVE
		upstream(id, depth, dep_type, path) AS (
			SELECT parent_id, 1, type, ',' || ? || ',' || parent_id || ','
			FROM task_deps WHERE child_id = ?
			UNION ALL
			SELECT d.parent_id, u.depth + 1, d.type, u.path || d.parent_id || ','
			FROM task_deps d
			JOIN upstream u ON d.child_id = u.id
			WHERE u.depth < 50
			AND INSTR(u.path, ',' || d.parent_id || ',') = 0
		),
		downstream(id, depth, dep_type, path) AS (
			SELECT child_id, 1, type, ',' || ? || ',' || child_id || ','
			FROM task_deps WHERE parent_id = ?
			UNION ALL
			SELECT d.child_id, dn.depth + 1, d.type, dn.path || d.child_id || ','
			FROM task_deps d
			JOIN downstream dn ON d.parent_id = dn.id
			WHERE dn.depth < 50
			AND INSTR(dn.path, ',' || d.child_id || ',') = 0
		)
		SELECT t.id, t.title, t.status, t.type, u.depth, 'upstream' AS direction, u.dep_type
		FROM upstream u
		JOIN tasks t ON t.id = u.id
		UNION ALL
		SELECT t.id, t.title, t.status, t.type, d.depth, 'downstream' AS direction, d.dep_type
		FROM downstream d
		JOIN tasks t ON t.id = d.id
		ORDER BY 6, 5, 1
	`

	rows, err := s.db.QueryContext(ctx, query, id, id, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []models.DepTreeNode
	for rows.Next() {
		var node models.DepTreeNode
		if err := rows.Scan(&node.ID, &node.Title, &node.Status, &node.Type, &node.Depth, &node.Direction, &node.DepType); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// StoreInfo returns metadata about the database.
func (s *Store) StoreInfo(ctx context.Context) (*StoreInfo, error) {
	info := &StoreInfo{
		TaskCounts: make(map[string]int),
	}

	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return nil, err
	}
	info.SchemaVersion = version

	rows, err := s.db.QueryContext(ctx, "SELECT status, COUNT(*) FROM tasks GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	total := 0
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		info.TaskCounts[status] = count
		total += count
	}
	info.TotalTasks = total

	return info, rows.Err()
}

// CleanupClosedTasks removes (or reports) closed tasks older than cutoff.
func (s *Store) CleanupClosedTasks(ctx context.Context, cutoff time.Time, dryRun bool) (*CleanupResult, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM tasks WHERE status = 'closed' AND updated_at < ?", dbFormatTime(cutoff))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &CleanupResult{
		TaskIDs: ids,
		Count:   len(ids),
		DryRun:  dryRun,
	}

	if dryRun || len(ids) == 0 {
		return result, nil
	}

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	query := fmt.Sprintf("DELETE FROM tasks WHERE id IN (%s)", placeholders(len(ids)))
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return nil, err
	}

	return result, nil
}

func insertDeps(ctx context.Context, tx *sql.Tx, childID string, deps []models.Dependency) error {
	if len(deps) == 0 {
		return nil
	}
	query := "INSERT OR IGNORE INTO task_deps (child_id, parent_id, type) VALUES "
	values := make([]string, len(deps))
	args := make([]any, 0, len(deps)*3)
	for i, dep := range deps {
		values[i] = "(?, ?, ?)"
		args = append(args, childID, dep.ParentID, dep.Type)
	}
	query += strings.Join(values, ",")
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}
