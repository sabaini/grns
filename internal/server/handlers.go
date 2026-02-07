package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"grns/internal/api"
)

const (
	exportPageSize        = 500
	defaultJSONMaxBody    = 1 << 20  // 1 MiB
	batchJSONMaxBody      = 8 << 20  // 8 MiB
	importJSONMaxBody     = 64 << 20 // 64 MiB
	importStreamChunkSize = 500
	importStreamMaxLine   = 10 << 20 // 10 MiB
)

func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	s.writeErrorReq(w, nil, status, err)
}

func (s *Server) writeErrorReq(w http.ResponseWriter, r *http.Request, status int, err error) {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}

	code := errorCode(status, err)
	numericCode := errorNumericCode(status, err)
	message := err.Error()

	fields := []any{"status", status, "code", code, "error_code", numericCode, "error", err}
	if r != nil {
		fields = append(fields, "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
	}

	switch {
	case status >= 500:
		s.log().Error("request error", fields...)
		message = "internal error"
	case status >= 400 && shouldWarnClientError(status):
		s.log().Warn("request rejected", fields...)
	case status >= 400:
		s.log().Debug("request rejected", fields...)
	}

	s.writeJSON(w, status, api.ErrorResponse{Error: message, Code: code, ErrorCode: numericCode})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.log().Error("write json response", "status", status, "error", err)
	}
}

type apiError struct {
	status  int
	code    string
	errCode int
	err     error
}

func (e apiError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e apiError) Unwrap() error {
	return e.err
}

func makeAPIError(status int, code string, errCode int, err error) error {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}

	var existing apiError
	if errors.As(err, &existing) {
		if existing.status != 0 {
			return existing
		}
	}

	return apiError{status: status, code: code, errCode: errCode, err: err}
}

func badRequest(err error) error {
	return badRequestCode(err, ErrCodeInvalidArgument)
}

func badRequestCode(err error, code int) error {
	return makeAPIError(http.StatusBadRequest, "invalid_argument", code, err)
}

func notFound(err error) error {
	return notFoundCode(err, ErrCodeTaskNotFound)
}

func notFoundCode(err error, code int) error {
	return makeAPIError(http.StatusNotFound, "not_found", code, err)
}

func conflict(err error) error {
	return conflictCode(err, ErrCodeConflict)
}

func conflictCode(err error, code int) error {
	return makeAPIError(http.StatusConflict, "conflict", code, err)
}

func internalError(err error) error {
	return makeAPIError(http.StatusInternalServerError, "internal", ErrCodeInternal, err)
}

func storeFailure(err error) error {
	return makeAPIError(http.StatusInternalServerError, "internal", ErrCodeStoreFailure, err)
}

func httpStatusFromError(err error) int {
	var apiErr apiError
	if errors.As(err, &apiErr) {
		return apiErr.status
	}
	return http.StatusInternalServerError
}

func errorCode(status int, err error) string {
	var apiErr apiError
	if errors.As(err, &apiErr) && apiErr.code != "" {
		return apiErr.code
	}
	switch status {
	case http.StatusBadRequest:
		return "invalid_argument"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusTooManyRequests:
		return "resource_exhausted"
	case http.StatusInternalServerError:
		return "internal"
	default:
		return ""
	}
}

func errorNumericCode(status int, err error) int {
	var apiErr apiError
	if errors.As(err, &apiErr) && apiErr.errCode > 0 {
		return apiErr.errCode
	}
	return defaultErrorCodeByStatus(status)
}

func shouldWarnClientError(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed: tasks.id")
}

func isInvalidSearchQuery(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unterminated string") ||
		strings.Contains(message, "fts5") && strings.Contains(message, "syntax") ||
		strings.Contains(message, "malformed match")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	maxBytes := defaultJSONMaxBody
	switch r.URL.Path {
	case "/v1/import":
		maxBytes = importJSONMaxBody
	case "/v1/tasks/batch":
		maxBytes = batchJSONMaxBody
	}

	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))
	return json.NewDecoder(r.Body).Decode(dst)
}

func classifyDecodeJSONError(err error) error {
	if err == nil {
		return nil
	}

	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return badRequestCode(fmt.Errorf("request body too large"), ErrCodeRequestTooLarge)
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return badRequestCode(fmt.Errorf("invalid JSON payload"), ErrCodeInvalidJSON)
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return badRequestCode(err, ErrCodeInvalidJSON)
	}

	var unmarshalErr *json.UnmarshalTypeError
	if errors.As(err, &unmarshalErr) {
		return badRequestCode(err, ErrCodeInvalidJSON)
	}

	return badRequestCode(err, ErrCodeInvalidJSON)
}

func (s *Server) decodeJSONReq(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := decodeJSON(w, r, dst); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, classifyDecodeJSONError(err))
		return false
	}
	return true
}

func (s *Server) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	s.writeErrorReq(w, r, httpStatusFromError(err), err)
}

func (s *Server) writeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	s.writeErrorReq(w, r, http.StatusInternalServerError, storeFailure(err))
}

func (s *Server) withLimiter(w http.ResponseWriter, r *http.Request, limiter chan struct{}, name string, fn func()) {
	if !s.acquireLimiter(limiter, w, r, name) {
		return
	}
	defer s.releaseLimiter(limiter)
	fn()
}

func (s *Server) pathIDOrBadRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := requirePathID(r)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return "", false
	}
	return id, true
}

func (s *Server) decodeIDsReq(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if !s.decodeJSONReq(w, r, &req) {
		return nil, false
	}
	if err := requireIDs(req.IDs); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return nil, false
	}
	return req.IDs, true
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func requirePathID(r *http.Request) (string, error) {
	id := strings.TrimSpace(r.PathValue("id"))
	if !validateID(id) {
		return "", badRequestCode(fmt.Errorf("invalid id"), ErrCodeInvalidID)
	}
	return id, nil
}

func requireIDs(ids []string) error {
	if len(ids) == 0 {
		return badRequestCode(fmt.Errorf("ids are required"), ErrCodeMissingRequired)
	}
	for _, id := range ids {
		if !validateID(id) {
			return badRequestCode(fmt.Errorf("invalid id"), ErrCodeInvalidID)
		}
	}
	return nil
}

func validateImportModes(dedupe, orphanHandling string) error {
	switch dedupe {
	case "", "skip", "overwrite", "error":
	default:
		return badRequestCode(fmt.Errorf("invalid dedupe mode: %s", dedupe), ErrCodeInvalidImportMode)
	}
	switch orphanHandling {
	case "", "allow", "skip", "strict":
	default:
		return badRequestCode(fmt.Errorf("invalid orphan_handling: %s", orphanHandling), ErrCodeInvalidImportMode)
	}
	return nil
}

func queryInt(r *http.Request, key string) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, badRequestCode(fmt.Errorf("invalid %s", key), ErrCodeInvalidQuery)
	}
	if parsed < 0 {
		return 0, badRequestCode(fmt.Errorf("%s must be >= 0", key), ErrCodeInvalidQuery)
	}
	return parsed, nil
}

func queryIntDefault(r *http.Request, key string, def int) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return def, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, badRequestCode(fmt.Errorf("invalid %s", key), ErrCodeInvalidQuery)
	}
	if parsed < 0 {
		return 0, badRequestCode(fmt.Errorf("%s must be >= 0", key), ErrCodeInvalidQuery)
	}
	return parsed, nil
}

func queryBool(r *http.Request, key string) (bool, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, badRequestCode(fmt.Errorf("invalid %s", key), ErrCodeInvalidQuery)
	}
	return parsed, nil
}

func valueOrEmpty(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return strings.TrimSpace(*ptr)
}

func parseFlexibleTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	t, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return t, nil
	}
	t, err = time.Parse("2006-01-02", value)
	if err == nil {
		return t, nil
	}
	return time.Time{}, badRequestCode(fmt.Errorf("expected RFC3339 or YYYY-MM-DD format"), ErrCodeInvalidTimeFilter)
}
