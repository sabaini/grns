package server

import (
	"net/http"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/tasks/ready", s.handleReady)
	mux.HandleFunc("/v1/tasks/stale", s.handleStale)
	mux.HandleFunc("/v1/tasks/batch", s.handleBatchCreate)
	mux.HandleFunc("/v1/tasks", s.handleTasks)
	mux.HandleFunc("/v1/tasks/", s.handleTaskByID)
	mux.HandleFunc("/v1/tasks/close", s.handleClose)
	mux.HandleFunc("/v1/tasks/reopen", s.handleReopen)
	mux.HandleFunc("/v1/deps", s.handleDeps)
	mux.HandleFunc("/v1/labels", s.handleLabels)

	return mux
}
