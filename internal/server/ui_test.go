package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestUIRoutes(t *testing.T) {
	srv := newListTestServer(t)
	h := srv.routes()

	t.Run("root serves index", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
		}
		if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Fatalf("expected html content type, got %q", got)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("expected no-cache for index, got %q", got)
		}
		if !strings.Contains(strings.ToLower(w.Body.String()), "<!doctype html>") {
			t.Fatalf("expected html document, got %q", w.Body.String())
		}
	})

	t.Run("static assets referenced by index are served from /ui", func(t *testing.T) {
		indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
		indexW := httptest.NewRecorder()
		h.ServeHTTP(indexW, indexReq)
		if indexW.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (%s)", indexW.Code, indexW.Body.String())
		}

		assets := extractUIAssetPaths(indexW.Body.String())
		if len(assets) == 0 {
			t.Fatal("expected at least one /ui asset reference in index.html")
		}

		for _, asset := range assets {
			req := httptest.NewRequest(http.MethodGet, asset, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s, got %d (%s)", asset, w.Code, w.Body.String())
			}
			if body := strings.TrimSpace(w.Body.String()); body == "" {
				t.Fatalf("expected non-empty asset body for %s", asset)
			}
			cache := w.Header().Get("Cache-Control")
			if isFingerprintAsset(strings.TrimPrefix(asset, "/ui/")) {
				if cache != "public, max-age=31536000, immutable" {
					t.Fatalf("expected immutable cache for %s, got %q", asset, cache)
				}
			} else if cache != "no-cache" {
				t.Fatalf("expected no-cache for non-fingerprinted asset %s, got %q", asset, cache)
			}
		}
	})

	t.Run("unknown path returns not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("api routes still handled as api", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
		}
		if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Fatalf("expected json content type, got %q", got)
		}
	})
}

func TestUIRoutesRemainAccessibleWhenAPITokenEnabled(t *testing.T) {
	srv := newListTestServer(t)
	srv.apiToken = "secret"
	h := srv.routes()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected UI root to be public, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected /v1/info to require auth, got %d", w.Code)
	}
}

func TestIsFingerprintAsset(t *testing.T) {
	tests := []struct {
		name  string
		asset string
		want  bool
	}{
		{name: "plain", asset: "app.js", want: false},
		{name: "short hash", asset: "app.abc123.js", want: false},
		{name: "alnum hash", asset: "app.ABC123de.js", want: true},
		{name: "nested path", asset: "assets/app.DDBVD2RV.css", want: true},
		{name: "symbol in hash", asset: "app.zzzz-zzz.js", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFingerprintAsset(tt.asset); got != tt.want {
				t.Fatalf("isFingerprintAsset(%q)=%v want %v", tt.asset, got, tt.want)
			}
		})
	}
}

func extractUIAssetPaths(index string) []string {
	re := regexp.MustCompile(`(?:src|href)=["'](/ui/[^"']+)["']`)
	matches := re.FindAllStringSubmatch(index, -1)
	if len(matches) == 0 {
		return nil
	}
	assets := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		asset := strings.TrimSpace(m[1])
		if asset == "" || !strings.HasPrefix(asset, "/ui/") {
			continue
		}
		if !slices.Contains(assets, asset) {
			assets = append(assets, asset)
		}
	}
	return assets
}
