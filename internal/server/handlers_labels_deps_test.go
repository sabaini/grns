package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"grns/internal/api"
)

func TestHandleListTaskLabels_ProjectScoped(t *testing.T) {
	srv := newListTestServer(t)

	seedListTask(t, srv, "xy-l001", "xy task", 2)
	if err := srv.store.AddLabels(context.Background(), "xy-l001", []string{"secret"}); err != nil {
		t.Fatalf("seed labels: %v", err)
	}

	t.Run("same project can read labels", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/projects/xy/tasks/xy-l001/labels", nil)
		w := httptest.NewRecorder()
		srv.routes().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
		}

		var labels []string
		if err := json.Unmarshal(w.Body.Bytes(), &labels); err != nil {
			t.Fatalf("decode labels: %v", err)
		}
		if len(labels) != 1 || labels[0] != "secret" {
			t.Fatalf("unexpected labels: %#v", labels)
		}
	})

	t.Run("cross-project read is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/projects/gr/tasks/xy-l001/labels", nil)
		w := httptest.NewRecorder()
		srv.routes().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d (%s)", w.Code, w.Body.String())
		}

		var errResp api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if errResp.ErrorCode != ErrCodeTaskNotFound {
			t.Fatalf("expected error_code %d, got %d", ErrCodeTaskNotFound, errResp.ErrorCode)
		}
	})
}

func TestHandleDepTree_RequiresTaskInScopedProject(t *testing.T) {
	srv := newListTestServer(t)
	seedListTask(t, srv, "xy-d001", "xy task", 2)

	tests := []struct {
		name string
		url  string
	}{
		{name: "missing task", url: "/v1/projects/gr/tasks/gr-d999/deps/tree"},
		{name: "cross-project task", url: "/v1/projects/gr/tasks/xy-d001/deps/tree"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()
			srv.routes().ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d (%s)", w.Code, w.Body.String())
			}

			var errResp api.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if errResp.ErrorCode != ErrCodeTaskNotFound {
				t.Fatalf("expected error_code %d, got %d", ErrCodeTaskNotFound, errResp.ErrorCode)
			}
		})
	}
}
