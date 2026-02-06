package store

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"grns/internal/models"
)

type ListFilter struct {
	Statuses    []string
	Types       []string
	Priority    *int
	PriorityMin *int
	PriorityMax *int
	ParentID    string
	Labels      []string
	LabelsAny   []string
	SpecRegex   string
	Limit       int
	Offset      int
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
			id, title, status, type, priority, description, spec_id, parent_id, created_at, updated_at, closed_at, custom
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		task.ID,
		task.Title,
		task.Status,
		task.Type,
		task.Priority,
		nullIfEmpty(task.Description),
		nullIfEmpty(task.SpecID),
		nullIfEmpty(task.ParentID),
		formatTime(task.CreatedAt),
		formatTime(task.UpdatedAt),
		nullTime(task.ClosedAt),
		nil,
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
		SELECT id, title, status, type, priority, description, spec_id, parent_id, created_at, updated_at, closed_at
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
	if update.ClosedAt != nil {
		set = append(set, "closed_at = ?")
		args = append(args, nullTime(update.ClosedAt))
	}

	set = append(set, "updated_at = ?")
	args = append(args, formatTime(update.UpdatedAt))

	if len(set) == 0 {
		return nil
	}

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

	if filter.SpecRegex != "" {
		filtered, err := filterBySpecRegex(tasks, filter.SpecRegex, filter.Limit, filter.Offset)
		if err != nil {
			return nil, err
		}
		return filtered, nil
	}

	return tasks, nil
}

// ListReadyTasks returns tasks with no open blockers.
func (s *Store) ListReadyTasks(ctx context.Context, limit int) ([]models.Task, error) {
	args := []any{}
	query := `
		SELECT id, title, status, type, priority, description, spec_id, parent_id, created_at, updated_at, closed_at
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
	args := []any{formatTime(cutoff)}
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
		SELECT id, title, status, type, priority, description, spec_id, parent_id, created_at, updated_at, closed_at
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
	_, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO task_labels (task_id, label) VALUES " + labelValues(len(labels)), labelArgs(id, labels)...)
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

// CloseTasks closes tasks and sets closed_at.
func (s *Store) CloseTasks(ctx context.Context, ids []string, closedAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}

	args := []any{formatTime(closedAt), formatTime(closedAt)}
	for _, id := range ids {
		args = append(args, id)
	}
	query := fmt.Sprintf("UPDATE tasks SET status = 'closed', closed_at = ?, updated_at = ? WHERE id IN (%s)", placeholders(len(ids)))
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// ReopenTasks reopens tasks and clears closed_at.
func (s *Store) ReopenTasks(ctx context.Context, ids []string, reopenedAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}

	args := []any{formatTime(reopenedAt)}
	for _, id := range ids {
		args = append(args, id)
	}
	query := fmt.Sprintf("UPDATE tasks SET status = 'open', closed_at = NULL, updated_at = ? WHERE id IN (%s)", placeholders(len(ids)))
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// TaskUpdate describes fields to update.
type TaskUpdate struct {
	Title       *string
	Status      *string
	Type        *string
	Priority    *int
	Description *string
	SpecID      *string
	ParentID    *string
	ClosedAt    *time.Time
	UpdatedAt   time.Time
}

func buildListQuery(filter ListFilter) (string, []any) {
	query := "SELECT id, title, status, type, priority, description, spec_id, parent_id, created_at, updated_at, closed_at FROM tasks"
	where := []string{}
	args := []any{}

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

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	query += " ORDER BY updated_at DESC"

	if filter.SpecRegex == "" && filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.SpecRegex == "" && filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	return query, args
}

func filterBySpecRegex(tasks []models.Task, pattern string, limit, offset int) ([]models.Task, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	var filtered []models.Task
	for _, task := range tasks {
		if task.SpecID == "" {
			continue
		}
		if re.MatchString(task.SpecID) {
			filtered = append(filtered, task)
		}
	}

	if offset > 0 {
		if offset >= len(filtered) {
			return []models.Task{}, nil
		}
		filtered = filtered[offset:]
	}
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}

	return filtered, nil
}

func scanTask(scanner interface{
	Scan(dest ...any) error
}) (*models.Task, error) {
	var task models.Task
	var description, specID, parentID sql.NullString
	var createdAt, updatedAt string
	var closedAt sql.NullString

	if err := scanner.Scan(
		&task.ID,
		&task.Title,
		&task.Status,
		&task.Type,
		&task.Priority,
		&description,
		&specID,
		&parentID,
		&createdAt,
		&updatedAt,
		&closedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	task.Description = description.String
	task.SpecID = specID.String
	task.ParentID = parentID.String

	parsedCreated, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	parsedUpdated, err := parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	task.CreatedAt = parsedCreated
	task.UpdatedAt = parsedUpdated
	if closedAt.Valid {
		parsedClosed, err := parseTime(closedAt.String)
		if err != nil {
			return nil, err
		}
		task.ClosedAt = &parsedClosed
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

func nullTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return formatTime(*value)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
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
