package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultHTTPTimeout   = 10 * time.Second
	httpTimeoutEnvKey    = "GRNS_HTTP_TIMEOUT"
	apiTokenEnvKey       = "GRNS_API_TOKEN"
	adminTokenEnvKey     = "GRNS_ADMIN_TOKEN"
	idempotentRetryCount = 2 // total attempts = retry count + 1
)

var (
	retryBaseDelay = 50 * time.Millisecond
	retryMaxDelay  = 500 * time.Millisecond
)

// Client is a simple HTTP client for the grns API.
type Client struct {
	baseURL    string
	http       *http.Client
	authToken  string
	adminToken string
}

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		http:       &http.Client{Timeout: httpTimeoutFromEnv()},
		authToken:  strings.TrimSpace(os.Getenv(apiTokenEnvKey)),
		adminToken: strings.TrimSpace(os.Getenv(adminTokenEnvKey)),
	}
}

// Ping checks whether the API server is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/health", nil, nil, nil)
}

// GetInfo returns server/runtime metadata from /v1/info.
func (c *Client) GetInfo(ctx context.Context) (InfoResponse, error) {
	var resp InfoResponse
	err := c.do(ctx, http.MethodGet, "/v1/info", nil, nil, &resp)
	return resp, err
}

// CreateTask creates a task via POST /v1/tasks.
func (c *Client) CreateTask(ctx context.Context, req TaskCreateRequest) (TaskResponse, error) {
	var resp TaskResponse
	err := c.do(ctx, http.MethodPost, "/v1/tasks", nil, req, &resp)
	return resp, err
}

// BatchCreate creates tasks in a single request via POST /v1/tasks/batch.
func (c *Client) BatchCreate(ctx context.Context, req []TaskCreateRequest) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodPost, "/v1/tasks/batch", nil, req, &resp)
	return resp, err
}

// GetTask fetches a task by ID via GET /v1/tasks/{id}.
func (c *Client) GetTask(ctx context.Context, id string) (TaskResponse, error) {
	var resp TaskResponse
	err := c.do(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id), nil, nil, &resp)
	return resp, err
}

// GetTasks fetches multiple tasks in one request via POST /v1/tasks/get.
func (c *Client) GetTasks(ctx context.Context, ids []string) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodPost, "/v1/tasks/get", nil, TaskGetManyRequest{IDs: ids}, &resp)
	return resp, err
}

// UpdateTask updates a task by ID via PATCH /v1/tasks/{id}.
func (c *Client) UpdateTask(ctx context.Context, id string, req TaskUpdateRequest) (TaskResponse, error) {
	var resp TaskResponse
	err := c.do(ctx, http.MethodPatch, "/v1/tasks/"+url.PathEscape(id), nil, req, &resp)
	return resp, err
}

// ListTasks returns tasks matching query filters via GET /v1/tasks.
func (c *Client) ListTasks(ctx context.Context, query url.Values) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodGet, "/v1/tasks", query, nil, &resp)
	return resp, err
}

// Ready returns ready-to-work tasks via GET /v1/tasks/ready.
func (c *Client) Ready(ctx context.Context, query url.Values) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodGet, "/v1/tasks/ready", query, nil, &resp)
	return resp, err
}

// Stale returns stale tasks via GET /v1/tasks/stale.
func (c *Client) Stale(ctx context.Context, query url.Values) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodGet, "/v1/tasks/stale", query, nil, &resp)
	return resp, err
}

// CloseTasks closes one or more tasks via POST /v1/tasks/close.
func (c *Client) CloseTasks(ctx context.Context, req TaskCloseRequest) (map[string]any, error) {
	var resp map[string]any
	err := c.do(ctx, http.MethodPost, "/v1/tasks/close", nil, req, &resp)
	return resp, err
}

// ReopenTasks reopens one or more tasks via POST /v1/tasks/reopen.
func (c *Client) ReopenTasks(ctx context.Context, req TaskReopenRequest) (map[string]any, error) {
	var resp map[string]any
	err := c.do(ctx, http.MethodPost, "/v1/tasks/reopen", nil, req, &resp)
	return resp, err
}

// DependencyTree returns the dependency tree for a task via GET /v1/tasks/{id}/deps/tree.
func (c *Client) DependencyTree(ctx context.Context, id string) (DepTreeResponse, error) {
	var resp DepTreeResponse
	err := c.do(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id)+"/deps/tree", nil, nil, &resp)
	return resp, err
}

// AdminCleanup executes admin cleanup via POST /v1/admin/cleanup.
// If confirm is true, X-Confirm is sent to execute deletion; otherwise it is a dry-run.
func (c *Client) AdminCleanup(ctx context.Context, req CleanupRequest, confirm bool) (CleanupResponse, error) {
	var resp CleanupResponse
	payload, err := json.Marshal(req)
	if err != nil {
		return resp, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/admin/cleanup", bytes.NewReader(payload))
	if err != nil {
		return resp, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if confirm {
		httpReq.Header.Set("X-Confirm", "true")
	}
	c.setAuthHeader(httpReq)
	c.setAdminHeader(httpReq)
	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return resp, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode >= 400 {
		return resp, decodeError(httpResp)
	}
	err = json.NewDecoder(httpResp.Body).Decode(&resp)
	return resp, err
}

// Import sends an import request.
func (c *Client) Import(ctx context.Context, req ImportRequest) (ImportResponse, error) {
	var resp ImportResponse
	err := c.do(ctx, http.MethodPost, "/v1/import", nil, req, &resp)
	return resp, err
}

// ImportStream sends NDJSON import records to the streaming import endpoint.
func (c *Client) ImportStream(ctx context.Context, records io.Reader, dryRun bool, dedupe, orphanHandling string, atomic bool) (ImportResponse, error) {
	var resp ImportResponse
	query := url.Values{}
	if dryRun {
		query.Set("dry_run", "true")
	}
	if dedupe != "" {
		query.Set("dedupe", dedupe)
	}
	if orphanHandling != "" {
		query.Set("orphan_handling", orphanHandling)
	}
	if atomic {
		query.Set("atomic", "true")
	}

	endpoint := c.baseURL + "/v1/import/stream"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, records)
	if err != nil {
		return resp, err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	c.setAuthHeader(req)

	httpResp, err := c.http.Do(req)
	if err != nil {
		return resp, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode >= 400 {
		return resp, decodeError(httpResp)
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// Export streams NDJSON export to a writer.
func (c *Client) Export(ctx context.Context, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/export", nil)
	if err != nil {
		return err
	}
	c.setAuthHeader(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return decodeError(resp)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

// AddDependency creates a dependency edge via POST /v1/deps.
func (c *Client) AddDependency(ctx context.Context, req DepCreateRequest) (map[string]any, error) {
	var resp map[string]any
	err := c.do(ctx, http.MethodPost, "/v1/deps", nil, req, &resp)
	return resp, err
}

// AddLabels adds labels to a task via POST /v1/tasks/{id}/labels.
func (c *Client) AddLabels(ctx context.Context, id string, req LabelsRequest) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodPost, "/v1/tasks/"+url.PathEscape(id)+"/labels", nil, req, &resp)
	return resp, err
}

// RemoveLabels removes labels from a task via DELETE /v1/tasks/{id}/labels.
func (c *Client) RemoveLabels(ctx context.Context, id string, req LabelsRequest) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodDelete, "/v1/tasks/"+url.PathEscape(id)+"/labels", nil, req, &resp)
	return resp, err
}

// ListLabels lists labels for a task via GET /v1/tasks/{id}/labels.
func (c *Client) ListLabels(ctx context.Context, id string) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id)+"/labels", nil, nil, &resp)
	return resp, err
}

// ListAllLabels lists all labels in the project via GET /v1/labels.
func (c *Client) ListAllLabels(ctx context.Context) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodGet, "/v1/labels", nil, nil, &resp)
	return resp, err
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = encoded
	}

	maxAttempts := 1
	if isIdempotentMethod(method) {
		maxAttempts += idempotentRetryCount
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}

		req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
		if err != nil {
			return err
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		c.setAuthHeader(req)

		resp, err := c.http.Do(req)
		if err != nil {
			if attempt+1 < maxAttempts && shouldRetryTransport(err) {
				delay := retryDelay(attempt)
				slog.Debug("api request retrying after transport error", "method", method, "path", path, "attempt", attempt+1, "max_attempts", maxAttempts, "delay_ms", delay.Milliseconds(), "error", err)
				time.Sleep(delay)
				continue
			}
			slog.Debug("api request transport failure", "method", method, "path", path, "attempt", attempt+1, "max_attempts", maxAttempts, "error", err)
			return err
		}

		if resp.StatusCode >= 500 && attempt+1 < maxAttempts && isRetryableStatus(resp.StatusCode) {
			delay := retryDelay(attempt)
			slog.Debug("api request retrying after server error", "method", method, "path", path, "attempt", attempt+1, "max_attempts", maxAttempts, "status", resp.StatusCode, "delay_ms", delay.Milliseconds())
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			time.Sleep(delay)
			continue
		}

		if resp.StatusCode >= 400 {
			slog.Debug("api request failed", "method", method, "path", path, "attempt", attempt+1, "status", resp.StatusCode)
			err := decodeError(resp)
			resp.Body.Close()
			return err
		}

		if attempt > 0 {
			slog.Debug("api request succeeded after retry", "method", method, "path", path, "attempt", attempt+1, "status", resp.StatusCode)
		}

		if out == nil {
			resp.Body.Close()
			return nil
		}

		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		return err
	}

	return fmt.Errorf("request failed after retries")
}

func isIdempotentMethod(method string) bool {
	return method == http.MethodGet
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func shouldRetryTransport(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	return false
}

func retryDelay(attempt int) time.Duration {
	delay := retryBaseDelay << attempt
	if delay > retryMaxDelay {
		delay = retryMaxDelay
	}
	jitterMax := delay / 2
	if jitterMax <= 0 {
		return delay
	}
	jitter := time.Duration(rand.Int63n(int64(jitterMax)))
	return delay + jitter
}

func decodeError(resp *http.Response) error {
	apiErr := &APIError{Status: resp.StatusCode}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
		apiErr.Code = errResp.Code
		apiErr.ErrorCode = errResp.ErrorCode
		apiErr.Message = errResp.Error
		if apiErr.Message != "" {
			return apiErr
		}
	}

	apiErr.Message = fmt.Sprintf("api error: %s", resp.Status)
	return apiErr
}

func (c *Client) setAuthHeader(req *http.Request) {
	if c.authToken == "" || req == nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken)
}

func (c *Client) setAdminHeader(req *http.Request) {
	if c.adminToken == "" || req == nil {
		return
	}
	req.Header.Set("X-Admin-Token", c.adminToken)
}

func httpTimeoutFromEnv() time.Duration {
	value := strings.TrimSpace(os.Getenv(httpTimeoutEnvKey))
	if value == "" {
		return defaultHTTPTimeout
	}

	if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
		return duration
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	return defaultHTTPTimeout
}
