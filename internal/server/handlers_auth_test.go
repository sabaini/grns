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
	srv.requireAuthWithUsers = true
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

func TestAuthRequirementWithProvisionedUsersIsOptIn(t *testing.T) {
	srv := newListTestServer(t)
	seedAdminUser(t, srv, "admin", "password-123")
	h := srv.routes()

	openReq := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	openW := httptest.NewRecorder()
	h.ServeHTTP(openW, openReq)
	if openW.Code != http.StatusOK {
		t.Fatalf("expected open-mode info 200, got %d (%s)", openW.Code, openW.Body.String())
	}

	srv.requireAuthWithUsers = true
	lockedReq := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	lockedW := httptest.NewRecorder()
	h.ServeHTTP(lockedW, lockedReq)
	if lockedW.Code != http.StatusUnauthorized {
		t.Fatalf("expected strict user mode to require auth, got %d (%s)", lockedW.Code, lockedW.Body.String())
	}
}

func TestAPITokenModeAlsoAcceptsSessionCookie(t *testing.T) {
	srv := newListTestServer(t)
	srv.apiToken = "token"
	srv.requireAuthWithUsers = true
	seedAdminUser(t, srv, "admin", "password-123")
	h := srv.routes()

	loginBody := []byte(`{"username":"admin","password":"password-123"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(loginBody))
	loginW := httptest.NewRecorder()
	h.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d (%s)", loginW.Code, loginW.Body.String())
	}

	var sessionCookie *http.Cookie
	for _, c := range loginW.Result().Cookies() {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie")
	}

	infoReq := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	infoReq.AddCookie(sessionCookie)
	infoW := httptest.NewRecorder()
	h.ServeHTTP(infoW, infoReq)
	if infoW.Code != http.StatusOK {
		t.Fatalf("expected session cookie to satisfy auth in api token mode, got %d (%s)", infoW.Code, infoW.Body.String())
	}
}

func TestAuthLoginRateLimitedAfterRepeatedFailures(t *testing.T) {
	srv := newListTestServer(t)
	srv.loginLimiter = newLoginRateLimiter(2, time.Minute, 10*time.Minute)
	seedAdminUser(t, srv, "admin", "password-123")
	h := srv.routes()

	for attempt := 1; attempt <= 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader([]byte(`{"username":"admin","password":"wrong"}`)))
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d (%s)", attempt, w.Code, w.Body.String())
		}
	}

	blockedReq := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader([]byte(`{"username":"admin","password":"wrong"}`)))
	blockedReq.RemoteAddr = "127.0.0.1:12345"
	blockedW := httptest.NewRecorder()
	h.ServeHTTP(blockedW, blockedReq)
	if blockedW.Code != http.StatusTooManyRequests {
		t.Fatalf("expected rate-limited login to return 429, got %d (%s)", blockedW.Code, blockedW.Body.String())
	}

	var errResp api.ErrorResponse
	if err := json.Unmarshal(blockedW.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.ErrorCode != ErrCodeResourceExhausted {
		t.Fatalf("expected error_code %d, got %d", ErrCodeResourceExhausted, errResp.ErrorCode)
	}
}

func TestAuthLoginSuccessResetsRateLimiterState(t *testing.T) {
	srv := newListTestServer(t)
	srv.loginLimiter = newLoginRateLimiter(2, time.Minute, 10*time.Minute)
	seedAdminUser(t, srv, "admin", "password-123")
	h := srv.routes()

	wrongReq := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader([]byte(`{"username":"admin","password":"wrong"}`)))
	wrongReq.RemoteAddr = "127.0.0.1:22334"
	wrongW := httptest.NewRecorder()
	h.ServeHTTP(wrongW, wrongReq)
	if wrongW.Code != http.StatusUnauthorized {
		t.Fatalf("expected first wrong login 401, got %d (%s)", wrongW.Code, wrongW.Body.String())
	}

	successReq := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader([]byte(`{"username":"admin","password":"password-123"}`)))
	successReq.RemoteAddr = "127.0.0.1:22334"
	successW := httptest.NewRecorder()
	h.ServeHTTP(successW, successReq)
	if successW.Code != http.StatusOK {
		t.Fatalf("expected successful login 200, got %d (%s)", successW.Code, successW.Body.String())
	}

	for attempt := 1; attempt <= 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader([]byte(`{"username":"admin","password":"wrong"}`)))
		req.RemoteAddr = "127.0.0.1:22334"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("post-reset attempt %d: expected 401, got %d (%s)", attempt, w.Code, w.Body.String())
		}
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
