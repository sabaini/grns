package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"grns/internal/config"
)

func TestAdminUserListUsesAPIClient(t *testing.T) {
	var called atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/admin/users" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		called.Store(true)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.APIURL = ts.URL
	cfg.DBPath = "/definitely/not/used.db"

	jsonOutput := false
	cmd := newAdminUserListCmd(&cfg, &jsonOutput)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute admin user list: %v", err)
	}
	if !called.Load() {
		t.Fatal("expected admin user list to call API endpoint")
	}
}
