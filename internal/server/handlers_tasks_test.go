package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"grns/internal/api"
)

func TestCreateTask_UnknownJSONFieldsAreIgnored(t *testing.T) {
	srv := newListTestServer(t)

	payload := map[string]any{
		"title":             "Forward compatible payload",
		"priority":          2,
		"unknown_new_field": map[string]any{"nested": true},
		"another_future":    "value",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", w.Code, w.Body.String())
	}

	var created api.TaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected created task id")
	}
	if created.Title != "Forward compatible payload" {
		t.Fatalf("unexpected title: %q", created.Title)
	}

	showReq := httptest.NewRequest(http.MethodGet, "/v1/tasks/"+created.ID, nil)
	showW := httptest.NewRecorder()
	srv.routes().ServeHTTP(showW, showReq)
	if showW.Code != http.StatusOK {
		t.Fatalf("expected 200 from show, got %d (%s)", showW.Code, showW.Body.String())
	}

	var shown api.TaskResponse
	if err := json.Unmarshal(showW.Body.Bytes(), &shown); err != nil {
		t.Fatalf("decode show response: %v", err)
	}
	if shown.ID != created.ID {
		t.Fatalf("expected shown id %q, got %q", created.ID, shown.ID)
	}
}
