package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"grns/internal/api"
)

func TestAdminUserHandlersLifecycle(t *testing.T) {
	srv := newListTestServer(t)
	h := srv.routes()

	createReq := []byte(`{"username":"Admin","password":"password-123"}`)
	createW := httptest.NewRecorder()
	h.ServeHTTP(createW, httptest.NewRequest(http.MethodPost, "/v1/admin/users", bytes.NewReader(createReq)))
	if createW.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", createW.Code, createW.Body.String())
	}

	var created api.AdminUser
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Username != "admin" {
		t.Fatalf("expected normalized username admin, got %q", created.Username)
	}
	if created.Disabled {
		t.Fatal("expected created user to be enabled")
	}

	dupW := httptest.NewRecorder()
	h.ServeHTTP(dupW, httptest.NewRequest(http.MethodPost, "/v1/admin/users", bytes.NewReader(createReq)))
	if dupW.Code != http.StatusConflict {
		t.Fatalf("expected duplicate create 409, got %d (%s)", dupW.Code, dupW.Body.String())
	}

	listW := httptest.NewRecorder()
	h.ServeHTTP(listW, httptest.NewRequest(http.MethodGet, "/v1/admin/users", nil))
	if listW.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d (%s)", listW.Code, listW.Body.String())
	}
	var users []api.AdminUser
	if err := json.Unmarshal(listW.Body.Bytes(), &users); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(users) != 1 || users[0].Username != "admin" {
		t.Fatalf("unexpected users: %+v", users)
	}

	disableReq := []byte(`{"disabled":true}`)
	disableW := httptest.NewRecorder()
	h.ServeHTTP(disableW, httptest.NewRequest(http.MethodPatch, "/v1/admin/users/admin", bytes.NewReader(disableReq)))
	if disableW.Code != http.StatusOK {
		t.Fatalf("expected disable 200, got %d (%s)", disableW.Code, disableW.Body.String())
	}
	var disabled api.AdminUser
	if err := json.Unmarshal(disableW.Body.Bytes(), &disabled); err != nil {
		t.Fatalf("decode disable response: %v", err)
	}
	if !disabled.Disabled {
		t.Fatal("expected user to be disabled")
	}

	deleteW := httptest.NewRecorder()
	h.ServeHTTP(deleteW, httptest.NewRequest(http.MethodDelete, "/v1/admin/users/admin", nil))
	if deleteW.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d (%s)", deleteW.Code, deleteW.Body.String())
	}
	var deleted api.AdminUserDeleteResponse
	if err := json.Unmarshal(deleteW.Body.Bytes(), &deleted); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if !deleted.Deleted || deleted.Username != "admin" {
		t.Fatalf("unexpected delete response: %+v", deleted)
	}

	deleteMissingW := httptest.NewRecorder()
	h.ServeHTTP(deleteMissingW, httptest.NewRequest(http.MethodDelete, "/v1/admin/users/admin", nil))
	if deleteMissingW.Code != http.StatusNotFound {
		t.Fatalf("expected delete missing 404, got %d (%s)", deleteMissingW.Code, deleteMissingW.Body.String())
	}
	var errResp api.ErrorResponse
	if err := json.Unmarshal(deleteMissingW.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.ErrorCode != ErrCodeUserNotFound {
		t.Fatalf("expected error_code %d, got %d", ErrCodeUserNotFound, errResp.ErrorCode)
	}
}
