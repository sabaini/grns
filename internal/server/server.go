package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"grns/internal/blobstore"
	"grns/internal/store"
)

const (
	apiTokenEnvKey              = "GRNS_API_TOKEN"
	adminTokenEnvKey            = "GRNS_ADMIN_TOKEN"
	requireAuthWithUsersEnvKey  = "GRNS_REQUIRE_AUTH_WITH_USERS"
	defaultBlobRootDir          = ".grns/blobs"
	readHeaderTimeout           = 5 * time.Second
	readTimeout                 = 30 * time.Second
	writeTimeout                = 60 * time.Second
	idleTimeout                 = 60 * time.Second
	importConcurrencyLimit      = 1
	exportConcurrencyLimit      = 2
	searchConcurrencyLimit      = 4
	defaultLoginMaxFailures     = 8
	defaultLoginFailureWindow   = 5 * time.Minute
	defaultLoginBlockedDuration = 10 * time.Minute

	defaultAttachmentUploadMaxBody   int64 = 100 << 20 // 100 MiB
	defaultAttachmentMultipartMemory int64 = 8 << 20   // 8 MiB
)

// Server wraps HTTP handlers for the grns API.
type Server struct {
	addr                      string
	store                     store.TaskStore
	projectPrefix             string
	service                   *TaskService
	attachmentService         *AttachmentService
	gitRefService             *TaskGitRefService
	authService               *AuthService
	blobStore                 blobstore.BlobStore
	logger                    *slog.Logger
	apiToken                  string
	adminToken                string
	requireAuthWithUsers      bool
	importLimiter             chan struct{}
	exportLimiter             chan struct{}
	searchLimiter             chan struct{}
	loginLimiter              *loginRateLimiter
	attachmentUploadMaxBody   int64
	attachmentMultipartMemory int64
	dbPath                    string
}

// AttachmentOptions configures attachment runtime behavior on the server.
type AttachmentOptions struct {
	MaxUploadBytes          int64
	MultipartMaxMemory      int64
	AllowedMediaTypes       []string
	RejectMediaTypeMismatch bool
	GCBatchSize             int
}

// New creates a new server instance.
func New(addr string, taskStore store.TaskStore, projectPrefix string, logger *slog.Logger, blobStores ...blobstore.BlobStore) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	var bs blobstore.BlobStore
	blobStoreSource := "injected"
	blobStoreRoot := ""
	if len(blobStores) > 0 {
		bs = blobStores[0]
	} else {
		fallbackRoot := filepath.Join(os.TempDir(), defaultBlobRootDir)
		blobStoreSource = "fallback"
		blobStoreRoot = fallbackRoot
		cas, err := blobstore.NewLocalCAS(fallbackRoot)
		if err == nil {
			bs = cas
		} else {
			logger.Warn("failed to initialize fallback blob store", "root", fallbackRoot, "error", err)
		}
	}

	var attachmentService *AttachmentService
	if attachmentStore, ok := any(taskStore).(store.AttachmentStore); ok {
		attachmentService = NewAttachmentService(taskStore, attachmentStore, bs, projectPrefix)
	}

	var gitRefService *TaskGitRefService
	if gitRefStore, ok := any(taskStore).(store.GitRefStore); ok {
		gitRefService = NewTaskGitRefService(taskStore, gitRefStore, projectPrefix)
	}

	srv := &Server{
		addr:                      addr,
		store:                     taskStore,
		projectPrefix:             projectPrefix,
		service:                   NewTaskService(taskStore, projectPrefix),
		attachmentService:         attachmentService,
		gitRefService:             gitRefService,
		blobStore:                 bs,
		logger:                    logger,
		apiToken:                  strings.TrimSpace(os.Getenv(apiTokenEnvKey)),
		adminToken:                strings.TrimSpace(os.Getenv(adminTokenEnvKey)),
		requireAuthWithUsers:      boolFromEnv(requireAuthWithUsersEnvKey),
		importLimiter:             make(chan struct{}, importConcurrencyLimit),
		exportLimiter:             make(chan struct{}, exportConcurrencyLimit),
		searchLimiter:             make(chan struct{}, searchConcurrencyLimit),
		loginLimiter:              newLoginRateLimiter(defaultLoginMaxFailures, defaultLoginFailureWindow, defaultLoginBlockedDuration),
		attachmentUploadMaxBody:   defaultAttachmentUploadMaxBody,
		attachmentMultipartMemory: defaultAttachmentMultipartMemory,
	}
	if authStore, ok := any(taskStore).(store.AuthStore); ok {
		srv.authService = NewAuthService(authStore)
	}

	fields := []any{
		"addr", addr,
		"project_prefix", projectPrefix,
		"attachment_service_enabled", attachmentService != nil,
		"git_ref_service_enabled", gitRefService != nil,
		"auth_service_enabled", srv.authService != nil,
		"api_token_configured", srv.apiToken != "",
		"admin_token_configured", srv.adminToken != "",
		"require_auth_with_users", srv.requireAuthWithUsers,
		"blob_store_source", blobStoreSource,
		"blob_store_enabled", bs != nil,
	}
	if blobStoreRoot != "" {
		fields = append(fields, "blob_store_root", blobStoreRoot)
	}
	logger.Debug("server initialized", fields...)

	return srv
}

// ConfigureAttachmentOptions applies attachment settings from config.
func (s *Server) ConfigureAttachmentOptions(opts AttachmentOptions) {
	if s == nil {
		return
	}
	if opts.MaxUploadBytes > 0 {
		s.attachmentUploadMaxBody = opts.MaxUploadBytes
	}
	if opts.MultipartMaxMemory > 0 {
		s.attachmentMultipartMemory = opts.MultipartMaxMemory
	}
	if s.attachmentService != nil {
		s.attachmentService.ConfigurePolicy(opts.AllowedMediaTypes, opts.RejectMediaTypeMismatch, opts.GCBatchSize)
	}
	if s.logger != nil {
		s.log().Debug("attachment options configured",
			"max_upload_bytes", s.attachmentUploadMaxBody,
			"multipart_max_memory", s.attachmentMultipartMemory,
			"allowed_media_type_count", len(opts.AllowedMediaTypes),
			"reject_media_type_mismatch", opts.RejectMediaTypeMismatch,
			"gc_batch_size", opts.GCBatchSize,
		)
	}
}

// SetDBPath records the active database path for runtime metadata endpoints.
func (s *Server) SetDBPath(path string) {
	if s == nil {
		return
	}
	s.dbPath = strings.TrimSpace(path)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	s.log().Info("starting server",
		"addr", s.addr,
		"project_prefix", s.projectPrefix,
		"api_token_configured", s.apiToken != "",
		"admin_token_configured", s.adminToken != "",
		"require_auth_with_users", s.requireAuthWithUsers,
	)
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
		return u.Host, nil
	}
	return apiURL, nil
}

func boolFromEnv(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}
	return parsed
}

func (s *Server) acquireLimiter(limiter chan struct{}, w http.ResponseWriter, r *http.Request, name string) bool {
	if limiter == nil {
		return true
	}
	select {
	case limiter <- struct{}{}:
		return true
	default:
		s.log().Debug("request concurrency limited", "limiter", name, "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
		err := apiError{
			status:  http.StatusTooManyRequests,
			code:    "resource_exhausted",
			errCode: ErrCodeResourceExhausted,
			err:     fmt.Errorf("too many concurrent %s requests", name),
		}
		s.writeErrorReq(w, r, http.StatusTooManyRequests, err)
		return false
	}
}

func (s *Server) log() *slog.Logger {
	if s != nil && s.logger != nil {
		return s.logger
	}
	return slog.Default()
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
