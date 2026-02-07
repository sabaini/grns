package store

import (
	"path/filepath"
	"testing"
	"time"
)

// testStore creates a temporary store for testing.
func testStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func intPtr(v int) *int { return &v }

func timePtr(t time.Time) *time.Time { return &t }
