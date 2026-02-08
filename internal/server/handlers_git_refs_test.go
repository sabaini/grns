package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"grns/internal/api"
	"grns/internal/models"
)

func TestTaskGitRefCRUDHandlers(t *testing.T) {
	srv := newListTestServer(t)
	now := time.Now().UTC()
	task := &models.Task{
		ID:         "gr-g001",
		Title:      "git refs task",
		Status:     "open",
		Type:       "task",
		Priority:   2,
		SourceRepo: "github.com/acme/repo",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := srv.store.CreateTask(context.Background(), task, nil, nil); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	payload := api.TaskGitRefCreateRequest{
		Relation:    "design_doc",
		ObjectType:  "path",
		ObjectValue: "docs/design.md",
		Note:        "primary design",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/gr-g001/git-refs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	var created models.TaskGitRef
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created ref: %v", err)
	}
	if created.ID == "" || created.Repo != "github.com/acme/repo" {
		t.Fatalf("unexpected created ref: %#v", created)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/tasks/gr-g001/git-refs", nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var list []models.TaskGitRef
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode refs list: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("unexpected refs list: %#v", list)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/git-refs/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/git-refs/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/git-refs/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (%s)", w.Code, w.Body.String())
	}
	var errResp api.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.ErrorCode != ErrCodeGitRefNotFound {
		t.Fatalf("expected error_code %d, got %d", ErrCodeGitRefNotFound, errResp.ErrorCode)
	}
}

func TestCloseWithCommitCreatesClosedByGitRefs(t *testing.T) {
	srv := newListTestServer(t)
	now := time.Now().UTC()
	task := &models.Task{
		ID:         "gr-g002",
		Title:      "close annotate",
		Status:     "open",
		Type:       "task",
		Priority:   2,
		SourceRepo: "github.com/acme/repo",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := srv.store.CreateTask(context.Background(), task, nil, nil); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	commit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := api.TaskCloseRequest{IDs: []string{"gr-g002"}, Commit: commit}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/close", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	var closeResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &closeResp); err != nil {
		t.Fatalf("decode close response: %v", err)
	}
	if got, ok := closeResp["annotated"].(float64); !ok || int(got) != 1 {
		t.Fatalf("expected annotated=1, got %#v", closeResp["annotated"])
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/tasks/gr-g002/git-refs", nil)
	w = httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var refs []models.TaskGitRef
	if err := json.Unmarshal(w.Body.Bytes(), &refs); err != nil {
		t.Fatalf("decode refs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].Relation != "closed_by" || refs[0].ObjectType != string(models.GitObjectTypeCommit) || refs[0].ObjectValue != commit {
		t.Fatalf("unexpected ref: %#v", refs[0])
	}
}

func TestCloseWithRepoRequiresCommit(t *testing.T) {
	srv := newListTestServer(t)
	payload := api.TaskCloseRequest{IDs: []string{"gr-a001"}, Repo: "github.com/acme/repo"}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/close", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestCloseWithCommitWithoutRepoContextDoesNotCloseTask(t *testing.T) {
	srv := newListTestServer(t)
	now := time.Now().UTC()
	task := &models.Task{
		ID:        "gr-g003",
		Title:     "close without repo context",
		Status:    "open",
		Type:      "task",
		Priority:  2,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := srv.store.CreateTask(context.Background(), task, nil, nil); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	payload := api.TaskCloseRequest{
		IDs:    []string{"gr-g003"},
		Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/close", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", w.Code, w.Body.String())
	}

	var errResp api.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.ErrorCode != ErrCodeMissingRequired {
		t.Fatalf("expected error_code %d, got %d", ErrCodeMissingRequired, errResp.ErrorCode)
	}

	showReq := httptest.NewRequest(http.MethodGet, "/v1/tasks/gr-g003", nil)
	showW := httptest.NewRecorder()
	srv.routes().ServeHTTP(showW, showReq)
	if showW.Code != http.StatusOK {
		t.Fatalf("expected 200 from show, got %d (%s)", showW.Code, showW.Body.String())
	}
	var shown api.TaskResponse
	if err := json.Unmarshal(showW.Body.Bytes(), &shown); err != nil {
		t.Fatalf("decode show response: %v", err)
	}
	if shown.Status != "open" {
		t.Fatalf("expected task to remain open, got %q", shown.Status)
	}
}
