package server

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"grns/internal/store"
)

const (
	apiTokenEnvKey         = "GRNS_API_TOKEN"
	adminTokenEnvKey       = "GRNS_ADMIN_TOKEN"
	allowRemoteEnvKey      = "GRNS_ALLOW_REMOTE"
	readHeaderTimeout      = 5 * time.Second
	readTimeout            = 30 * time.Second
	writeTimeout           = 60 * time.Second
	idleTimeout            = 60 * time.Second
	importConcurrencyLimit = 1
	exportConcurrencyLimit = 2
	searchConcurrencyLimit = 4
)

// Server wraps HTTP handlers for the grns API.
type Server struct {
	addr          string
	store         store.TaskStore
	projectPrefix string
	service       *TaskService
	logger        *slog.Logger
	apiToken      string
	adminToken    string
	importLimiter chan struct{}
	exportLimiter chan struct{}
	searchLimiter chan struct{}
}

// New creates a new server instance.
func New(addr string, store store.TaskStore, projectPrefix string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		addr:          addr,
		store:         store,
		projectPrefix: projectPrefix,
		service:       NewTaskService(store, projectPrefix),
		logger:        logger,
		apiToken:      strings.TrimSpace(os.Getenv(apiTokenEnvKey)),
		adminToken:    strings.TrimSpace(os.Getenv(adminTokenEnvKey)),
		importLimiter: make(chan struct{}, importConcurrencyLimit),
		exportLimiter: make(chan struct{}, exportConcurrencyLimit),
		searchLimiter: make(chan struct{}, searchConcurrencyLimit),
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	s.logger.Info("starting server", "addr", s.addr)
	server := &http.Server{
		Addr:              s.addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	return server.ListenAndServe()
}

// ListenAddr converts a base API URL into a listen address.
func ListenAddr(apiURL string) (string, error) {
	if apiURL == "" {
		return "", fmt.Errorf("api url is required")
	}
	if u, err := url.Parse(apiURL); err == nil && u.Host != "" {
		host := u.Hostname()
		if !isAllowedListenHost(host) {
			return "", fmt.Errorf("remote listen host %q requires %s=true", host, allowRemoteEnvKey)
		}
		return u.Host, nil
	}

	host, _, err := net.SplitHostPort(apiURL)
	if err == nil && !isAllowedListenHost(host) {
		return "", fmt.Errorf("remote listen host %q requires %s=true", host, allowRemoteEnvKey)
	}

	return apiURL, nil
}

func isAllowedListenHost(host string) bool {
	if host == "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(allowRemoteEnvKey)), "true") {
		return true
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) acquireLimiter(limiter chan struct{}, w http.ResponseWriter, name string) bool {
	if limiter == nil {
		return true
	}
	select {
	case limiter <- struct{}{}:
		return true
	default:
		s.writeError(w, http.StatusTooManyRequests, apiError{
			status: http.StatusTooManyRequests,
			code:   "resource_exhausted",
			err:    fmt.Errorf("too many concurrent %s requests", name),
		})
		return false
	}
}

func (s *Server) releaseLimiter(limiter chan struct{}) {
	if limiter == nil {
		return
	}
	select {
	case <-limiter:
	default:
	}
}
