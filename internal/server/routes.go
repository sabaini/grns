package server

import (
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Health check and info.
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/info", s.handleInfo)

	// Tasks collection.
	mux.HandleFunc("POST /v1/tasks", s.handleCreateTask)
	mux.HandleFunc("GET /v1/tasks", s.handleListTasks)

	// Task batch operations.
	mux.HandleFunc("POST /v1/tasks/get", s.handleGetTasks)
	mux.HandleFunc("POST /v1/tasks/batch", s.handleBatchCreate)
	mux.HandleFunc("POST /v1/tasks/close", s.handleClose)
	mux.HandleFunc("POST /v1/tasks/reopen", s.handleReopen)

	// Task queries.
	mux.HandleFunc("GET /v1/tasks/ready", s.handleReady)
	mux.HandleFunc("GET /v1/tasks/stale", s.handleStale)

	// Single task.
	mux.HandleFunc("GET /v1/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("PATCH /v1/tasks/{id}", s.handleUpdateTask)

	// Task labels.
	mux.HandleFunc("GET /v1/tasks/{id}/labels", s.handleListTaskLabels)
	mux.HandleFunc("POST /v1/tasks/{id}/labels", s.handleAddTaskLabels)
	mux.HandleFunc("DELETE /v1/tasks/{id}/labels", s.handleRemoveTaskLabels)

	// Dependency tree.
	mux.HandleFunc("GET /v1/tasks/{id}/deps/tree", s.handleDepTree)

	// Import/Export.
	mux.HandleFunc("GET /v1/export", s.handleExport)
	mux.HandleFunc("POST /v1/import", s.handleImport)
	mux.HandleFunc("POST /v1/import/stream", s.handleImportStream)

	// Admin.
	mux.HandleFunc("POST /v1/admin/cleanup", s.handleAdminCleanup)

	// Dependencies and labels.
	mux.HandleFunc("POST /v1/deps", s.handleDeps)
	mux.HandleFunc("GET /v1/labels", s.handleLabels)

	return s.withRequestLogging(s.withAuth(mux))
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	if s.apiToken == "" && s.adminToken == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		if s.apiToken != "" {
			auth := strings.TrimSpace(r.Header.Get("Authorization"))
			expected := "Bearer " + s.apiToken
			if auth != expected {
				s.writeErrorReq(w, r, http.StatusUnauthorized, apiError{
					status:  http.StatusUnauthorized,
					code:    "unauthorized",
					errCode: ErrCodeUnauthorized,
					err:     fmt.Errorf("unauthorized"),
				})
				return
			}
		}

		if s.adminToken != "" && strings.HasPrefix(r.URL.Path, "/v1/admin/") {
			adminToken := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
			if adminToken != s.adminToken {
				s.writeErrorReq(w, r, http.StatusForbidden, apiError{
					status:  http.StatusForbidden,
					code:    "forbidden",
					errCode: ErrCodeForbidden,
					err:     fmt.Errorf("forbidden"),
				})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
