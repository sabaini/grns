package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Health check and info.
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/info", s.handleInfo)

	// Authentication.
	mux.HandleFunc("POST /v1/auth/login", s.handleAuthLogin)
	mux.HandleFunc("POST /v1/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("GET /v1/auth/me", s.handleAuthMe)

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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		if r.URL.Path == "/v1/auth/login" {
			next.ServeHTTP(w, r)
			return
		}

		requireAuth, err := s.apiAuthRequired(r)
		if err != nil {
			s.writeStoreError(w, r, err)
			return
		}

		authenticated := false
		principal := authPrincipal{}
		if requireAuth {
			authenticated, principal, err = s.authorizeAPIRequest(r)
			if err != nil {
				s.writeStoreError(w, r, err)
				return
			}
			if !authenticated {
				s.writeErrorReq(w, r, http.StatusUnauthorized, apiError{
					status:  http.StatusUnauthorized,
					code:    "unauthorized",
					errCode: ErrCodeUnauthorized,
					err:     fmt.Errorf("unauthorized"),
				})
				return
			}
			if principal.AuthType == authTypeSession && isUnsafeMethod(r.Method) && !sameOrigin(r) {
				s.writeErrorReq(w, r, http.StatusForbidden, apiError{
					status:  http.StatusForbidden,
					code:    "forbidden",
					errCode: ErrCodeForbidden,
					err:     fmt.Errorf("forbidden"),
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

		ctx := contextWithAuthRequired(r.Context(), requireAuth)
		if authenticated {
			ctx = contextWithAuthPrincipal(ctx, principal)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) apiAuthRequired(r *http.Request) (bool, error) {
	if s == nil {
		return false, nil
	}
	if s.authService == nil {
		return s.apiToken != "", nil
	}

	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	return s.authService.AuthRequired(ctx, s.apiToken != "", s.requireAuthWithUsers, time.Now().UTC())
}

func (s *Server) authorizeAPIRequest(r *http.Request) (bool, authPrincipal, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if s.apiToken != "" && authHeader == "Bearer "+s.apiToken {
		return true, authPrincipal{AuthType: authTypeBearer}, nil
	}

	sessionToken := sessionTokenFromRequest(r)
	if sessionToken == "" || s.authService == nil {
		return false, authPrincipal{}, nil
	}
	user, err := s.authService.AuthenticateSessionToken(r.Context(), sessionToken, time.Now().UTC())
	if err != nil {
		return false, authPrincipal{}, err
	}
	if user == nil {
		return false, authPrincipal{}, nil
	}
	return true, authPrincipal{AuthType: authTypeSession, User: user}, nil
}

func sessionTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if originURL.Host == "" {
		return false
	}

	scheme := requestScheme(r)
	if !strings.EqualFold(originURL.Scheme, scheme) {
		return false
	}

	return strings.EqualFold(originURL.Host, strings.TrimSpace(r.Host))
}

func requestScheme(r *http.Request) string {
	if r != nil {
		if r.TLS != nil {
			return "https"
		}
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
			return "https"
		}
	}
	return "http"
}
