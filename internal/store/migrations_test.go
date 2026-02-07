package store

import (
	"database/sql"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func testRawDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunMigrationsFreshDB(t *testing.T) {
	db := testRawDB(t)

	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	version, err := currentVersion(db)
	if err != nil {
		t.Fatalf("current version: %v", err)
	}
	if version != 4 {
		t.Fatalf("expected version 4, got %d", version)
	}

	// Verify tasks table exists.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&count); err != nil {
		t.Fatalf("check tasks: %v", err)
	}
	if count != 1 {
		t.Fatal("tasks table not created")
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	db := testRawDB(t)

	if err := runMigrations(db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := runMigrations(db); err != nil {
		t.Fatalf("second run: %v", err)
	}

	version, err := currentVersion(db)
	if err != nil {
		t.Fatalf("current version: %v", err)
	}
	if version != 4 {
		t.Fatalf("expected version 4, got %d", version)
	}
}

func TestDetectPreMigrationDB(t *testing.T) {
	db := testRawDB(t)

	// Empty DB — not pre-migration.
	pre, err := detectPreMigrationDB(db)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if pre {
		t.Fatal("empty DB should not be pre-migration")
	}

	// Create tasks table manually (simulating MVP DB with full v1 schema).
	if _, err := db.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY, title TEXT NOT NULL, status TEXT NOT NULL,
		type TEXT NOT NULL, priority INTEGER NOT NULL, description TEXT,
		spec_id TEXT, parent_id TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
		closed_at TEXT, custom TEXT
	)`); err != nil {
		t.Fatalf("create tasks: %v", err)
	}

	pre, err = detectPreMigrationDB(db)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !pre {
		t.Fatal("DB with tasks but no schema_migrations should be pre-migration")
	}

	// After running migrations, should stamp version 1 and detect as migrated.
	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pre, err = detectPreMigrationDB(db)
	if err != nil {
		t.Fatalf("detect after migration: %v", err)
	}
	if pre {
		t.Fatal("after migration should not be pre-migration")
	}

	version, err := currentVersion(db)
	if err != nil {
		t.Fatalf("current version: %v", err)
	}
	if version != 4 {
		t.Fatalf("expected version 4, got %d", version)
	}
}

func TestMigrationPlan(t *testing.T) {
	db := testRawDB(t)

	plan, err := MigrationPlan(db)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.CurrentVersion != 0 {
		t.Fatalf("expected current 0, got %d", plan.CurrentVersion)
	}
	if plan.AvailableVersion != 4 {
		t.Fatalf("expected available 4, got %d", plan.AvailableVersion)
	}
	if len(plan.Pending) != 4 {
		t.Fatalf("expected 4 pending, got %d", len(plan.Pending))
	}
}

func TestMigration002UpgradePath(t *testing.T) {
	db := testRawDB(t)

	// Apply only migration 1 by running and then verifying.
	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	version, err := currentVersion(db)
	if err != nil {
		t.Fatalf("current version: %v", err)
	}
	if version != 4 {
		t.Fatalf("expected version 4, got %d", version)
	}

	// Verify new columns exist by inserting a row that uses them.
	_, err = db.Exec(`INSERT INTO tasks (id, title, status, type, priority, assignee, notes, design, acceptance_criteria, source_repo, created_at, updated_at)
		VALUES ('test-1', 'Test', 'open', 'task', 2, 'alice', 'some notes', 'some design', 'criteria', 'github.com/test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert with new columns: %v", err)
	}

	var assignee, notes string
	err = db.QueryRow("SELECT assignee, notes FROM tasks WHERE id = 'test-1'").Scan(&assignee, &notes)
	if err != nil {
		t.Fatalf("query new columns: %v", err)
	}
	if assignee != "alice" {
		t.Fatalf("expected assignee 'alice', got %q", assignee)
	}
	if notes != "some notes" {
		t.Fatalf("expected notes 'some notes', got %q", notes)
	}
}

func TestMigration003FTS5(t *testing.T) {
	db := testRawDB(t)

	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	// Verify FTS table exists.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='tasks_fts'").Scan(&count); err != nil {
		t.Fatalf("check fts: %v", err)
	}
	if count != 1 {
		t.Fatal("tasks_fts table not created")
	}

	// Insert a task and verify trigger syncs to FTS.
	if _, err := db.Exec(`INSERT INTO tasks (id, title, status, type, priority, description, notes, created_at, updated_at)
		VALUES ('fts-1', 'Authentication bug', 'open', 'bug', 0, 'Login fails with OAuth', 'Needs investigation', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Search by title.
	var taskID string
	err := db.QueryRow("SELECT task_id FROM tasks_fts WHERE tasks_fts MATCH 'authentication'").Scan(&taskID)
	if err != nil {
		t.Fatalf("fts search title: %v", err)
	}
	if taskID != "fts-1" {
		t.Fatalf("expected fts-1, got %q", taskID)
	}

	// Search by description.
	err = db.QueryRow("SELECT task_id FROM tasks_fts WHERE tasks_fts MATCH 'OAuth'").Scan(&taskID)
	if err != nil {
		t.Fatalf("fts search description: %v", err)
	}
	if taskID != "fts-1" {
		t.Fatalf("expected fts-1, got %q", taskID)
	}

	// Update and verify FTS syncs.
	if _, err := db.Exec("UPDATE tasks SET title = 'Authorization bug' WHERE id = 'fts-1'"); err != nil {
		t.Fatalf("update: %v", err)
	}

	err = db.QueryRow("SELECT task_id FROM tasks_fts WHERE tasks_fts MATCH 'authorization'").Scan(&taskID)
	if err != nil {
		t.Fatalf("fts search after update: %v", err)
	}

	// Old title should not match.
	err = db.QueryRow("SELECT task_id FROM tasks_fts WHERE tasks_fts MATCH 'authentication'").Scan(&taskID)
	if err == nil {
		t.Fatal("old title should not match after update")
	}

	// Delete and verify FTS syncs.
	if _, err := db.Exec("DELETE FROM tasks WHERE id = 'fts-1'"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	err = db.QueryRow("SELECT task_id FROM tasks_fts WHERE tasks_fts MATCH 'authorization'").Scan(&taskID)
	if err == nil {
		t.Fatal("deleted task should not match in FTS")
	}
}

func TestMigration004ListQueryIndexes(t *testing.T) {
	db := testRawDB(t)
	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	t.Run("default list uses updated_at index", func(t *testing.T) {
		query := `EXPLAIN QUERY PLAN SELECT id, title, status, type, priority, description, spec_id, parent_id, assignee, notes, design, acceptance_criteria, source_repo, created_at, updated_at, closed_at, custom FROM tasks ORDER BY updated_at DESC LIMIT 20`
		var planText string
		rows, err := db.Query(query)
		if err != nil {
			t.Fatalf("explain default list: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
				t.Fatalf("scan explain row: %v", err)
			}
			planText += detail + "\n"
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("explain rows: %v", err)
		}
		if !containsPlan(planText, "idx_tasks_updated_at_desc") {
			t.Fatalf("expected default list to use idx_tasks_updated_at_desc, plan:\n%s", planText)
		}
	})

	t.Run("type-filter list uses type+updated index", func(t *testing.T) {
		query := `EXPLAIN QUERY PLAN SELECT id, title, status, type, priority, description, spec_id, parent_id, assignee, notes, design, acceptance_criteria, source_repo, created_at, updated_at, closed_at, custom FROM tasks WHERE type IN ('task') ORDER BY updated_at DESC LIMIT 20`
		var planText string
		rows, err := db.Query(query)
		if err != nil {
			t.Fatalf("explain type list: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
				t.Fatalf("scan explain row: %v", err)
			}
			planText += detail + "\n"
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("explain rows: %v", err)
		}
		if !containsPlan(planText, "idx_tasks_type_updated_desc") {
			t.Fatalf("expected type-filter list to use idx_tasks_type_updated_desc, plan:\n%s", planText)
		}
	})
}

func containsPlan(plan, needle string) bool {
	return strings.Contains(plan, needle)
}
