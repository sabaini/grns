package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite"
)

const (
	busyTimeoutMS   = 5000
	maxOpenConns    = 1
	maxIdleConns    = 1
	connMaxLifetime = 5 * time.Minute
)

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
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

	// Tune connection pool for local usage.
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	return nil
}

func sqliteDSN(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("db path is required")
	}
	u := url.URL{Scheme: "file", Path: path}
	return u.String(), nil
}
