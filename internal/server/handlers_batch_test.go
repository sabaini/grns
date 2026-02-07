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

func TestHandleBatchCreateAllOrNothing(t *testing.T) {
	srv := newListTestServer(t)

	payload := []api.TaskCreateRequest{
		{ID: "gr-zz01", Title: "first"},
		{ID: "gr-zz01", Title: "duplicate"},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d (%s)", w.Code, w.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/tasks", nil)
	listW := httptest.NewRecorder()
	srv.routes().ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list status: %d (%s)", listW.Code, listW.Body.String())
	}

	var tasks []api.TaskResponse
	if err := json.Unmarshal(listW.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no tasks after failed batch, got %d", len(tasks))
	}
}

func TestHandleBatchCreateSuccess(t *testing.T) {
	srv := newListTestServer(t)

	payload := []api.TaskCreateRequest{{Title: "first"}, {Title: "second"}}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	var tasks []api.TaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].ID == "" || tasks[1].ID == "" {
		t.Fatalf("expected generated ids, got %+v", tasks)
	}
}

func TestHandleGetTasks(t *testing.T) {
	srv := newListTestServer(t)
	now := time.Now().UTC()
	mustCreate := func(id, title string) {
		t.Helper()
		err := srv.store.CreateTask(context.Background(), &models.Task{ID: id, Title: title, Status: "open", Type: "task", Priority: 2, CreatedAt: now, UpdatedAt: now}, nil, nil)
		if err != nil {
			t.Fatalf("create task %s: %v", id, err)
		}
	}
	mustCreate("gr-gm11", "first")
	mustCreate("gr-gm22", "second")

	payload := api.TaskGetManyRequest{IDs: []string{"gr-gm22", "gr-gm11"}}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/get", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	var tasks []api.TaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].ID != "gr-gm22" || tasks[1].ID != "gr-gm11" {
		t.Fatalf("expected request order preserved, got %+v", tasks)
	}
}
