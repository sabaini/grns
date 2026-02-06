package store

import (
	"database/sql"
	"net/url"
	"path/filepath"
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
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
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
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
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

	// Create tasks table manually (simulating MVP DB).
	if _, err := db.Exec("CREATE TABLE tasks (id TEXT PRIMARY KEY)"); err != nil {
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
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
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
	if plan.AvailableVersion != 1 {
		t.Fatalf("expected available 1, got %d", plan.AvailableVersion)
	}
	if len(plan.Pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(plan.Pending))
	}
}
