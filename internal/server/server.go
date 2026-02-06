package server

import (
	"fmt"
	"net/http"
	"net/url"

	"grns/internal/store"
)

// Server wraps HTTP handlers for the grns API.
type Server struct {
	addr          string
	store         store.TaskStore
	projectPrefix string
	service       *TaskService
}

// New creates a new server instance.
func New(addr string, store store.TaskStore, projectPrefix string) *Server {
	return &Server{
		addr:          addr,
		store:         store,
		projectPrefix: projectPrefix,
		service:       NewTaskService(store, projectPrefix),
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	server := &http.Server{
		Addr:    s.addr,
		Handler: s.routes(),
	}

	return server.ListenAndServe()
}

// ListenAddr converts a base API URL into a listen address.
func ListenAddr(apiURL string) (string, error) {
	if apiURL == "" {
		return "", fmt.Errorf("api url is required")
	}
	if u, err := url.Parse(apiURL); err == nil && u.Host != "" {
		return u.Host, nil
	}
	return apiURL, nil
}
