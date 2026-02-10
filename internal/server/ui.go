package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed uiassets/dist/*
var uiFS embed.FS

func (s *Server) uiAssetHandler() http.Handler {
	dist, err := fs.Sub(uiFS, "uiassets/dist")
	if err != nil {
		return http.NotFoundHandler()
	}

	fileServer := http.StripPrefix("/ui/", http.FileServerFS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/ui/") {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}

		asset := strings.TrimPrefix(r.URL.Path, "/ui/")
		if isFingerprintAsset(asset) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}

		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) handleUIIndex(w http.ResponseWriter, r *http.Request) {
	dist, err := fs.Sub(uiFS, "uiassets/dist")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(index)
}

func isFingerprintAsset(assetPath string) bool {
	base := path.Base(strings.TrimSpace(assetPath))
	parts := strings.Split(base, ".")
	if len(parts) < 3 {
		return false
	}

	hash := parts[len(parts)-2]
	if len(hash) < 8 {
		return false
	}
	for _, ch := range hash {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return false
		}
	}
	return true
}
