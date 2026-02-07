package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"grns/internal/api"
)

func TestListenAddrRemoteGuard(t *testing.T) {
	t.Run("allows loopback", func(t *testing.T) {
		t.Setenv(allowRemoteEnvKey, "")
		addr, err := ListenAddr("http://127.0.0.1:7333")
		if err != nil {
			t.Fatalf("expected loopback to be allowed, got error: %v", err)
		}
		if addr != "127.0.0.1:7333" {
			t.Fatalf("unexpected addr: %s", addr)
		}
	})

	t.Run("blocks non-loopback by default", func(t *testing.T) {
		t.Setenv(allowRemoteEnvKey, "")
		_, err := ListenAddr("http://0.0.0.0:7333")
		if err == nil {
			t.Fatal("expected error for non-loopback listen host")
		}
	})

	t.Run("allows non-loopback when explicitly enabled", func(t *testing.T) {
		t.Setenv(allowRemoteEnvKey, "true")
		addr, err := ListenAddr("http://0.0.0.0:7333")
		if err != nil {
			t.Fatalf("expected allow-remote to permit host, got error: %v", err)
		}
		if addr != "0.0.0.0:7333" {
			t.Fatalf("unexpected addr: %s", addr)
		}
	})
}

func TestWithAuth(t *testing.T) {
	t.Run("denies missing auth", func(t *testing.T) {
		srv := &Server{apiToken: "token"}
		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusNoContent)
		})
		handler := srv.withAuth(next)

		req := httptest.NewRequest(http.MethodGet, "/v1/tasks", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
		var errResp api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if errResp.ErrorCode != ErrCodeUnauthorized {
			t.Fatalf("expected error_code %d, got %d", ErrCodeUnauthorized, errResp.ErrorCode)
		}
		if nextCalled {
			t.Fatal("next handler should not be called")
		}
	})

	t.Run("allows valid auth", func(t *testing.T) {
		srv := &Server{apiToken: "token"}
		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusNoContent)
		})
		handler := srv.withAuth(next)

		req := httptest.NewRequest(http.MethodGet, "/v1/tasks", nil)
		req.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
		if !nextCalled {
			t.Fatal("next handler should be called")
		}
	})

	t.Run("admin routes require admin token when configured", func(t *testing.T) {
		srv := &Server{apiToken: "token", adminToken: "admintoken"}
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		handler := srv.withAuth(next)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/cleanup", nil)
		req.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
		var errResp api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if errResp.ErrorCode != ErrCodeForbidden {
			t.Fatalf("expected error_code %d, got %d", ErrCodeForbidden, errResp.ErrorCode)
		}

		req = httptest.NewRequest(http.MethodPost, "/v1/admin/cleanup", nil)
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("X-Admin-Token", "admintoken")
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
	})

	t.Run("admin token works without api token", func(t *testing.T) {
		srv := &Server{adminToken: "admintoken"}
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		handler := srv.withAuth(next)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/cleanup", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
		var errResp api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if errResp.ErrorCode != ErrCodeForbidden {
			t.Fatalf("expected error_code %d, got %d", ErrCodeForbidden, errResp.ErrorCode)
		}

		req = httptest.NewRequest(http.MethodPost, "/v1/admin/cleanup", nil)
		req.Header.Set("X-Admin-Token", "admintoken")
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
	})
}
