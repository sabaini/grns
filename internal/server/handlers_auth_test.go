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
	internalauth "grns/internal/auth"
	"grns/internal/store"
)

func TestAuthMeOpenMode(t *testing.T) {
	srv := newListTestServer(t)
	h := srv.routes()

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	var resp api.AuthMeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode auth me response: %v", err)
	}
	if resp.AuthRequired {
		t.Fatal("expected auth_required=false in open mode")
	}
	if resp.Authenticated {
		t.Fatal("expected authenticated=false in open mode")
	}
}

func TestBrowserSessionLoginFlow(t *testing.T) {
	srv := newListTestServer(t)
	seedAdminUser(t, srv, "admin", "password-123")
	h := srv.routes()

	loginBody := []byte(`{"username":"admin","password":"password-123"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(loginBody))
	loginW := httptest.NewRecorder()
	h.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d (%s)", loginW.Code, loginW.Body.String())
	}

	cookies := loginW.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie on login response")
	}
	if sessionCookie.HttpOnly != true {
		t.Fatal("expected HttpOnly session cookie")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	meReq.AddCookie(sessionCookie)
	meW := httptest.NewRecorder()
	h.ServeHTTP(meW, meReq)
	if meW.Code != http.StatusOK {
		t.Fatalf("expected auth me 200, got %d (%s)", meW.Code, meW.Body.String())
	}

	var meResp api.AuthMeResponse
	if err := json.Unmarshal(meW.Body.Bytes(), &meResp); err != nil {
		t.Fatalf("decode auth me response: %v", err)
	}
	if !meResp.AuthRequired || !meResp.Authenticated {
		t.Fatalf("expected authenticated auth-required response, got %+v", meResp)
	}
	if meResp.Username != "admin" {
		t.Fatalf("expected username admin, got %q", meResp.Username)
	}
	if meResp.AuthType != authTypeSession {
		t.Fatalf("expected auth_type %q, got %q", authTypeSession, meResp.AuthType)
	}

	logoutNoOriginReq := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	logoutNoOriginReq.AddCookie(sessionCookie)
	logoutNoOriginW := httptest.NewRecorder()
	h.ServeHTTP(logoutNoOriginW, logoutNoOriginReq)
	if logoutNoOriginW.Code != http.StatusForbidden {
		t.Fatalf("expected logout without origin to be forbidden, got %d", logoutNoOriginW.Code)
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	logoutReq.Header.Set("Origin", "http://example.com")
	logoutReq.AddCookie(sessionCookie)
	logoutW := httptest.NewRecorder()
	h.ServeHTTP(logoutW, logoutReq)
	if logoutW.Code != http.StatusNoContent {
		t.Fatalf("expected logout 204, got %d (%s)", logoutW.Code, logoutW.Body.String())
	}

	infoReq := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	infoReq.AddCookie(sessionCookie)
	infoW := httptest.NewRecorder()
	h.ServeHTTP(infoW, infoReq)
	if infoW.Code != http.StatusUnauthorized {
		t.Fatalf("expected info to be unauthorized after logout, got %d", infoW.Code)
	}
}

func TestAuthLoginInvalidCredentials(t *testing.T) {
	srv := newListTestServer(t)
	seedAdminUser(t, srv, "admin", "password-123")
	h := srv.routes()

	loginBody := []byte(`{"username":"admin","password":"wrong"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(loginBody))
	loginW := httptest.NewRecorder()
	h.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusUnauthorized {
		t.Fatalf("expected login 401, got %d (%s)", loginW.Code, loginW.Body.String())
	}
}

func seedAdminUser(t *testing.T, srv *Server, username, password string) {
	t.Helper()
	authStore, ok := any(srv.store).(store.AuthStore)
	if !ok {
		t.Fatal("server store does not implement auth store")
	}
	hash, err := internalauth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := authStore.CreateAdminUser(context.Background(), username, hash, time.Now().UTC()); err != nil {
		t.Fatalf("create admin user: %v", err)
	}
}
