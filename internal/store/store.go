package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"grns/internal/models"

	_ "modernc.org/sqlite"
)

const (
	busyTimeoutMS         = 5000
	maxOpenConns          = 1
	maxIdleConns          = 1
	connMaxLifetime       = 5 * time.Minute
	maxOpenConnsEnvKey    = "GRNS_DB_MAX_OPEN_CONNS"
	maxIdleConnsEnvKey    = "GRNS_DB_MAX_IDLE_CONNS"
	connMaxLifetimeEnvKey = "GRNS_DB_CONN_MAX_LIFETIME"
)

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

type txImportMutator struct {
	tx *sql.Tx
}

func (m *txImportMutator) TaskExists(id string) (bool, error) {
	var exists int
	err := m.tx.QueryRow("SELECT 1 FROM tasks WHERE id = ? LIMIT 1", id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (m *txImportMutator) CreateTask(ctx context.Context, task *models.Task, labels []string, deps []models.Dependency) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}
	if err := insertTaskRow(ctx, m.tx, task); err != nil {
		return err
	}
	if err := insertLabels(ctx, m.tx, task.ID, labels); err != nil {
		return err
	}
	if err := insertDeps(ctx, m.tx, task.ID, deps); err != nil {
		return err
	}
	return nil
}

func (m *txImportMutator) UpdateTask(ctx context.Context, id string, update TaskUpdate) error {
	return updateTaskExec(ctx, m.tx, id, update)
}

func (m *txImportMutator) AddDependency(ctx context.Context, childID, parentID, depType string) error {
	return addDependencyExec(ctx, m.tx, childID, parentID, depType)
}

func (m *txImportMutator) ReplaceLabels(ctx context.Context, id string, labels []string) error {
	return replaceLabelsTx(ctx, m.tx, id, labels)
}

func (m *txImportMutator) RemoveDependencies(ctx context.Context, childID string) error {
	return removeDependenciesExec(ctx, m.tx, childID)
}

// TaskExists checks whether a task exists by id.
func (s *Store) TaskExists(id string) (bool, error) {
	var exists int
	err := s.db.QueryRow("SELECT 1 FROM tasks WHERE id = ? LIMIT 1", id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Open opens the SQLite database and bootstraps the schema.
func Open(path string) (*Store, error) {
	dsn, err := sqliteDSN(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if err := configureDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// RunInTx executes fn in a single database transaction for atomic import operations.
func (s *Store) RunInTx(ctx context.Context, fn func(ImportMutator) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	mutator := &txImportMutator{tx: tx}
	if err := fn(mutator); err != nil {
		return err
	}
	return tx.Commit()
}

func configureDB(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA foreign_keys = ON;",
		fmt.Sprintf("PRAGMA busy_timeout = %d;", busyTimeoutMS),
	}
	for _, stmt := range pragmas {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	// Tune connection pool for local usage (configurable via env for benchmarks/tuning).
	db.SetMaxOpenConns(intFromEnv(maxOpenConnsEnvKey, maxOpenConns))
	db.SetMaxIdleConns(intFromEnv(maxIdleConnsEnvKey, maxIdleConns))
	db.SetConnMaxLifetime(durationFromEnv(connMaxLifetimeEnvKey, connMaxLifetime))

	return nil
}

func intFromEnv(key string, def int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return def
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return def
	}
	return parsed
}

func durationFromEnv(key string, def time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return def
	}
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return def
}

func sqliteDSN(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("db path is required")
	}
	u := url.URL{Scheme: "file", Path: path}
	return u.String(), nil
}
