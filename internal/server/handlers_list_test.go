package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grns/internal/api"
	"grns/internal/blobstore"
	"grns/internal/models"
	"grns/internal/store"
)

func TestHandleListTasksOffsetWithoutLimit(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "gr-a001", "first", 1)
	seedListTask(t, srv, "gr-a002", "second", 2)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/gr/tasks?offset=1", nil)
	w := httptest.NewRecorder()

	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	var got []api.TaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 task with offset-only query, got %d", len(got))
	}
}

func TestHandleListTasksInvalidQueryParams(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "gr-b001", "seed", 2)

	tests := []struct {
		name        string
		query       string
		wantMessage string
		wantCode    int
	}{
		{name: "invalid limit", query: "limit=abc", wantMessage: "invalid limit", wantCode: ErrCodeInvalidQuery},
		{name: "negative offset", query: "offset=-1", wantMessage: "offset must be >= 0", wantCode: ErrCodeInvalidQuery},
		{name: "priority out of range", query: "priority=5", wantMessage: "priority must be between 0 and 4", wantCode: ErrCodeInvalidPriority},
		{name: "priority min parse", query: "priority_min=bad", wantMessage: "invalid priority_min", wantCode: ErrCodeInvalidPriority},
		{name: "priority range inverted", query: "priority_min=4&priority_max=1", wantMessage: "priority_min cannot be greater than priority_max", wantCode: ErrCodeInvalidPriority},
		{name: "invalid created_after", query: "created_after=nope", wantMessage: "invalid created_after", wantCode: ErrCodeInvalidTimeFilter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/projects/gr/tasks?"+tt.query, nil)
			w := httptest.NewRecorder()

			srv.routes().ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d (%s)", w.Code, w.Body.String())
			}

			var errResp api.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if !strings.Contains(errResp.Error, tt.wantMessage) {
				t.Fatalf("expected error containing %q, got %q", tt.wantMessage, errResp.Error)
			}
			if errResp.ErrorCode != tt.wantCode {
				t.Fatalf("expected error_code %d, got %d", tt.wantCode, errResp.ErrorCode)
			}
		})
	}
}

func TestHandleListTasksMalformedSearch(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "gr-c001", "searchable", 1)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/gr/tasks?search=%22", nil)
	w := httptest.NewRecorder()

	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", w.Code, w.Body.String())
	}

	var errResp api.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error != "invalid search query" {
		t.Fatalf("expected invalid search query error, got %q", errResp.Error)
	}
	if errResp.ErrorCode != ErrCodeInvalidSearchQuery {
		t.Fatalf("expected error_code %d, got %d", ErrCodeInvalidSearchQuery, errResp.ErrorCode)
	}
}

func newListTestServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv(apiTokenEnvKey, "")
	t.Setenv(adminTokenEnvKey, "")

	dbPath := filepath.Join(t.TempDir(), "grns-test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	bs, err := blobstore.NewLocalCAS(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatalf("open blob store: %v", err)
	}

	return New("127.0.0.1:0", st, "gr", nil, bs)
}

func seedListTask(t *testing.T, srv *Server, id, title string, priority int) {
	t.Helper()
	now := time.Now().UTC()
	task := &models.Task{
		ID:        id,
		Title:     title,
		Status:    "open",
		Type:      "task",
		Priority:  priority,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := srv.store.CreateTask(context.Background(), task, nil, nil); err != nil {
		t.Fatalf("seed task %s: %v", id, err)
	}
}
