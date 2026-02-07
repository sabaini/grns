package store

import (
	"database/sql"
	"fmt"
	"sort"
)

// Migration represents a schema migration step.
type Migration struct {
	Version     int
	Description string
	SQL         string
}

// MigrationStatus reports the current and available migration versions.
type MigrationStatus struct {
	CurrentVersion   int             `json:"current_version"`
	AvailableVersion int             `json:"available_version"`
	Pending          []MigrationInfo `json:"pending"`
}

// MigrationInfo describes a single migration.
type MigrationInfo struct {
	Version     int    `json:"version"`
	Description string `json:"description"`
}

// migrations is the ordered list of all schema migrations.
var migrations = []Migration{
	{
		Version:     1,
		Description: "initial schema: tasks, labels, deps tables and indexes",
		SQL: `
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  type TEXT NOT NULL,
  priority INTEGER NOT NULL,
  description TEXT,
  spec_id TEXT,
  parent_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  closed_at TEXT,
  custom TEXT
);

CREATE TABLE IF NOT EXISTS task_labels (
  task_id TEXT NOT NULL,
  label TEXT NOT NULL,
  UNIQUE(task_id, label),
  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS task_deps (
  child_id TEXT NOT NULL,
  parent_id TEXT NOT NULL,
  type TEXT NOT NULL,
  UNIQUE(child_id, parent_id, type),
  FOREIGN KEY (child_id) REFERENCES tasks(id) ON DELETE CASCADE,
  FOREIGN KEY (parent_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_updated ON tasks(status, updated_at);
CREATE INDEX IF NOT EXISTS idx_tasks_spec_id ON tasks(spec_id);
CREATE INDEX IF NOT EXISTS idx_tasks_parent_id ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_task_labels_label ON task_labels(label);
CREATE INDEX IF NOT EXISTS idx_task_deps_child ON task_deps(child_id);
CREATE INDEX IF NOT EXISTS idx_task_deps_parent ON task_deps(parent_id);
`,
	},
	{
		Version:     2,
		Description: "add assignee, notes, design, acceptance_criteria, source_repo columns",
		SQL: `
ALTER TABLE tasks ADD COLUMN assignee TEXT;
ALTER TABLE tasks ADD COLUMN notes TEXT;
ALTER TABLE tasks ADD COLUMN design TEXT;
ALTER TABLE tasks ADD COLUMN acceptance_criteria TEXT;
ALTER TABLE tasks ADD COLUMN source_repo TEXT;

CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee);
CREATE INDEX IF NOT EXISTS idx_tasks_source_repo ON tasks(source_repo);
`,
	},
	{
		Version:     3,
		Description: "FTS5 full-text search on tasks",
		SQL: `
CREATE VIRTUAL TABLE IF NOT EXISTS tasks_fts USING fts5(
	task_id UNINDEXED,
	title,
	description,
	notes
);

INSERT INTO tasks_fts(task_id, title, description, notes)
	SELECT id, title, COALESCE(description, ''), COALESCE(notes, '')
	FROM tasks;

CREATE TRIGGER IF NOT EXISTS tasks_fts_insert AFTER INSERT ON tasks BEGIN
	INSERT INTO tasks_fts(task_id, title, description, notes)
		VALUES (new.id, new.title, COALESCE(new.description, ''), COALESCE(new.notes, ''));
END;

CREATE TRIGGER IF NOT EXISTS tasks_fts_update AFTER UPDATE ON tasks BEGIN
	DELETE FROM tasks_fts WHERE task_id = old.id;
	INSERT INTO tasks_fts(task_id, title, description, notes)
		VALUES (new.id, new.title, COALESCE(new.description, ''), COALESCE(new.notes, ''));
END;

CREATE TRIGGER IF NOT EXISTS tasks_fts_delete AFTER DELETE ON tasks BEGIN
	DELETE FROM tasks_fts WHERE task_id = old.id;
END;
`,
	},
	{
		Version:     4,
		Description: "list query index tuning from measured query plans",
		SQL: `
CREATE INDEX IF NOT EXISTS idx_tasks_updated_at_desc ON tasks(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_type_updated_desc ON tasks(type, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee_updated_desc ON tasks(assignee, updated_at DESC);
`,
	},
}

const migrationsTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
`

// ensureMigrationsTable creates the schema_migrations table if it doesn't exist.
func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(migrationsTableSQL)
	return err
}

// currentVersion returns the highest applied migration version, or 0 if none.
func currentVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// detectPreMigrationDB checks if the tasks table exists but no migrations have been recorded.
// This indicates a database created before the migration framework was added.
func detectPreMigrationDB(db *sql.DB) (bool, error) {
	var tasksExist int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&tasksExist)
	if err != nil {
		return false, err
	}
	if tasksExist == 0 {
		return false, nil
	}

	// Check if schema_migrations table exists.
	var migrationsExist int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&migrationsExist)
	if err != nil {
		return false, err
	}
	if migrationsExist == 0 {
		return true, nil
	}

	// Table exists but may be empty (e.g. created but no versions recorded).
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// runMigrations applies all pending migrations in order.
func runMigrations(db *sql.DB) error {
	// Detect pre-migration databases BEFORE creating the migrations table.
	preMigration, err := detectPreMigrationDB(db)
	if err != nil {
		return fmt.Errorf("detect pre-migration db: %w", err)
	}

	if err := ensureMigrationsTable(db); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	if preMigration {
		// Mark migration 1 as applied since the schema already exists.
		if _, err := db.Exec("INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (?, datetime('now'))", 1); err != nil {
			return fmt.Errorf("stamp pre-migration db: %w", err)
		}
	}

	current, err := currentVersion(db)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	sorted := make([]Migration, len(migrations))
	copy(sorted, migrations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Version < sorted[j].Version })

	for _, m := range sorted {
		if m.Version <= current {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(m.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Description, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, datetime('now'))", m.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}
	}

	return nil
}

// MigrationPlan returns the current migration status without applying anything.
func MigrationPlan(db *sql.DB) (*MigrationStatus, error) {
	// Detect pre-migration databases BEFORE creating the migrations table.
	preMigration, err := detectPreMigrationDB(db)
	if err != nil {
		return nil, err
	}

	if err := ensureMigrationsTable(db); err != nil {
		return nil, err
	}

	current, err := currentVersion(db)
	if err != nil {
		return nil, err
	}

	// If pre-migration DB, treat as version 1 for planning purposes.
	effective := current
	if preMigration && effective == 0 {
		effective = 1
	}

	sorted := make([]Migration, len(migrations))
	copy(sorted, migrations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Version < sorted[j].Version })

	available := 0
	if len(sorted) > 0 {
		available = sorted[len(sorted)-1].Version
	}

	var pending []MigrationInfo
	for _, m := range sorted {
		if m.Version > effective {
			pending = append(pending, MigrationInfo{Version: m.Version, Description: m.Description})
		}
	}

	return &MigrationStatus{
		CurrentVersion:   effective,
		AvailableVersion: available,
		Pending:          pending,
	}, nil
}
