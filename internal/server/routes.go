package server

import (
	"net/http"
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

	// Admin.
	mux.HandleFunc("POST /v1/admin/cleanup", s.handleAdminCleanup)

	// Dependencies and labels.
	mux.HandleFunc("POST /v1/deps", s.handleDeps)
	mux.HandleFunc("GET /v1/labels", s.handleLabels)

	return mux
}
