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

	// Project-scoped tasks collection.
	mux.HandleFunc("POST /v1/projects/{project}/tasks", s.handleCreateTask)
	mux.HandleFunc("GET /v1/projects/{project}/tasks", s.handleListTasks)

	// Project-scoped task batch operations.
	mux.HandleFunc("POST /v1/projects/{project}/tasks/get", s.handleGetTasks)
	mux.HandleFunc("POST /v1/projects/{project}/tasks/batch", s.handleBatchCreate)
	mux.HandleFunc("POST /v1/projects/{project}/tasks/close", s.handleClose)
	mux.HandleFunc("POST /v1/projects/{project}/tasks/reopen", s.handleReopen)

	// Project-scoped task queries.
	mux.HandleFunc("GET /v1/projects/{project}/tasks/ready", s.handleReady)
	mux.HandleFunc("GET /v1/projects/{project}/tasks/stale", s.handleStale)

	// Project-scoped single task.
	mux.HandleFunc("GET /v1/projects/{project}/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("PATCH /v1/projects/{project}/tasks/{id}", s.handleUpdateTask)

	// Project-scoped task labels.
	mux.HandleFunc("GET /v1/projects/{project}/tasks/{id}/labels", s.handleListTaskLabels)
	mux.HandleFunc("POST /v1/projects/{project}/tasks/{id}/labels", s.handleAddTaskLabels)
	mux.HandleFunc("DELETE /v1/projects/{project}/tasks/{id}/labels", s.handleRemoveTaskLabels)

	// Project-scoped attachments.
	mux.HandleFunc("POST /v1/projects/{project}/tasks/{id}/attachments", s.handleCreateTaskAttachment)
	mux.HandleFunc("POST /v1/projects/{project}/tasks/{id}/attachments/link", s.handleCreateTaskAttachmentLink)
	mux.HandleFunc("GET /v1/projects/{project}/tasks/{id}/attachments", s.handleListTaskAttachments)
	mux.HandleFunc("GET /v1/projects/{project}/attachments/{attachment_id}", s.handleGetAttachment)
	mux.HandleFunc("GET /v1/projects/{project}/attachments/{attachment_id}/content", s.handleGetAttachmentContent)
	mux.HandleFunc("DELETE /v1/projects/{project}/attachments/{attachment_id}", s.handleDeleteAttachment)

	// Project-scoped task git references.
	mux.HandleFunc("POST /v1/projects/{project}/tasks/{id}/git-refs", s.handleCreateTaskGitRef)
	mux.HandleFunc("GET /v1/projects/{project}/tasks/{id}/git-refs", s.handleListTaskGitRefs)
	mux.HandleFunc("GET /v1/projects/{project}/git-refs/{ref_id}", s.handleGetTaskGitRef)
	mux.HandleFunc("DELETE /v1/projects/{project}/git-refs/{ref_id}", s.handleDeleteTaskGitRef)

	// Project-scoped dependency tree.
	mux.HandleFunc("GET /v1/projects/{project}/tasks/{id}/deps/tree", s.handleDepTree)

	// Project-scoped import/export.
	mux.HandleFunc("GET /v1/projects/{project}/export", s.handleExport)
	mux.HandleFunc("POST /v1/projects/{project}/import", s.handleImport)
	mux.HandleFunc("POST /v1/projects/{project}/import/stream", s.handleImportStream)

	// Admin.
	mux.HandleFunc("POST /v1/admin/cleanup", s.handleAdminCleanup)
	mux.HandleFunc("POST /v1/admin/gc-blobs", s.handleAdminGCBlobs)

	// Project-scoped dependencies and labels.
	mux.HandleFunc("POST /v1/projects/{project}/deps", s.handleDeps)
	mux.HandleFunc("GET /v1/projects/{project}/labels", s.handleLabels)

	// Embedded Web UI.
	mux.HandleFunc("GET /{$}", s.handleUIIndex)
	mux.Handle("GET /ui/", s.uiAssetHandler())

	return s.withRequestLogging(s.withAuth(s.withProjectContext(mux)))
}

func (s *Server) withProjectContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/projects/") {
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if len(parts) < 4 {
			next.ServeHTTP(w, r)
			return
		}
		project := strings.TrimSpace(parts[2])
		ctx := contextWithProject(r.Context(), project)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
