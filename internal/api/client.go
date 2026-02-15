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
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"grns/internal/models"
)

const (
	defaultHTTPTimeout   = 10 * time.Second
	httpTimeoutEnvKey    = "GRNS_HTTP_TIMEOUT"
	apiTokenEnvKey       = "GRNS_API_TOKEN"
	adminTokenEnvKey     = "GRNS_ADMIN_TOKEN"
	idempotentRetryCount = 2 // total attempts = retry count + 1
	defaultProject       = "gr"
)

var (
	retryBaseDelay = 50 * time.Millisecond
	retryMaxDelay  = 500 * time.Millisecond
)

// Client is a simple HTTP client for the grns API.
type Client struct {
	baseURL    string
	project    string
	http       *http.Client
	authToken  string
	adminToken string
}

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		project:    defaultProject,
		http:       &http.Client{Timeout: httpTimeoutFromEnv()},
		authToken:  strings.TrimSpace(os.Getenv(apiTokenEnvKey)),
		adminToken: strings.TrimSpace(os.Getenv(adminTokenEnvKey)),
	}
}

// SetProject sets the default project used for project-scoped endpoints.
func (c *Client) SetProject(project string) {
	if c == nil {
		return
	}
	c.project = normalizeProject(project)
}

func (c *Client) scopedPath(path string) string {
	if c == nil {
		return path
	}
	project := normalizeProject(c.project)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "/v1/projects/" + url.PathEscape(project) + path
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
	err := c.do(ctx, http.MethodPost, c.scopedPath("/tasks"), nil, req, &resp)
	return resp, err
}

// BatchCreate creates tasks in a single request via POST /v1/tasks/batch.
func (c *Client) BatchCreate(ctx context.Context, req []TaskCreateRequest) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodPost, c.scopedPath("/tasks/batch"), nil, req, &resp)
	return resp, err
}

// GetTask fetches a task by ID via GET /v1/tasks/{id}.
func (c *Client) GetTask(ctx context.Context, id string) (TaskResponse, error) {
	var resp TaskResponse
	err := c.do(ctx, http.MethodGet, c.scopedPath("/tasks/"+url.PathEscape(id)), nil, nil, &resp)
	return resp, err
}

// GetTasks fetches multiple tasks in one request via POST /v1/tasks/get.
func (c *Client) GetTasks(ctx context.Context, ids []string) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodPost, c.scopedPath("/tasks/get"), nil, TaskGetManyRequest{IDs: ids}, &resp)
	return resp, err
}

// UpdateTask updates a task by ID via PATCH /v1/tasks/{id}.
func (c *Client) UpdateTask(ctx context.Context, id string, req TaskUpdateRequest) (TaskResponse, error) {
	var resp TaskResponse
	err := c.do(ctx, http.MethodPatch, c.scopedPath("/tasks/"+url.PathEscape(id)), nil, req, &resp)
	return resp, err
}

// ListTasks returns tasks matching query filters via GET /v1/tasks.
func (c *Client) ListTasks(ctx context.Context, query url.Values) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodGet, c.scopedPath("/tasks"), query, nil, &resp)
	return resp, err
}

// Ready returns ready-to-work tasks via GET /v1/tasks/ready.
func (c *Client) Ready(ctx context.Context, query url.Values) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodGet, c.scopedPath("/tasks/ready"), query, nil, &resp)
	return resp, err
}

// Stale returns stale tasks via GET /v1/tasks/stale.
func (c *Client) Stale(ctx context.Context, query url.Values) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodGet, c.scopedPath("/tasks/stale"), query, nil, &resp)
	return resp, err
}

// CloseTasks closes one or more tasks via POST /v1/tasks/close.
func (c *Client) CloseTasks(ctx context.Context, req TaskCloseRequest) (map[string]any, error) {
	var resp map[string]any
	err := c.do(ctx, http.MethodPost, c.scopedPath("/tasks/close"), nil, req, &resp)
	return resp, err
}

// ReopenTasks reopens one or more tasks via POST /v1/tasks/reopen.
func (c *Client) ReopenTasks(ctx context.Context, req TaskReopenRequest) (map[string]any, error) {
	var resp map[string]any
	err := c.do(ctx, http.MethodPost, c.scopedPath("/tasks/reopen"), nil, req, &resp)
	return resp, err
}

// DependencyTree returns the dependency tree for a task via GET /v1/tasks/{id}/deps/tree.
func (c *Client) DependencyTree(ctx context.Context, id string) (DepTreeResponse, error) {
	var resp DepTreeResponse
	err := c.do(ctx, http.MethodGet, c.scopedPath("/tasks/"+url.PathEscape(id))+"/deps/tree", nil, nil, &resp)
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

// AdminUserAdd provisions one local admin user.
func (c *Client) AdminUserAdd(ctx context.Context, req AdminUserCreateRequest) (AdminUser, error) {
	var resp AdminUser
	payload, err := json.Marshal(req)
	if err != nil {
		return resp, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/admin/users", bytes.NewReader(payload))
	if err != nil {
		return resp, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// AdminUserList lists provisioned local admin users.
func (c *Client) AdminUserList(ctx context.Context) ([]AdminUser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/admin/users", nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeader(httpReq)
	c.setAdminHeader(httpReq)

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode >= 400 {
		return nil, decodeError(httpResp)
	}

	var users []AdminUser
	if err := json.NewDecoder(httpResp.Body).Decode(&users); err != nil {
		return nil, err
	}
	return users, nil
}

// AdminUserSetDisabled enables or disables one local admin user.
func (c *Client) AdminUserSetDisabled(ctx context.Context, username string, disabled bool) (AdminUser, error) {
	var resp AdminUser
	payload, err := json.Marshal(AdminUserSetDisabledRequest{Disabled: disabled})
	if err != nil {
		return resp, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/v1/admin/users/"+url.PathEscape(username), bytes.NewReader(payload))
	if err != nil {
		return resp, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// AdminUserDelete deletes one local admin user.
func (c *Client) AdminUserDelete(ctx context.Context, username string) (AdminUserDeleteResponse, error) {
	var resp AdminUserDeleteResponse
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/v1/admin/users/"+url.PathEscape(username), nil)
	if err != nil {
		return resp, err
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
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// Import sends an import request.
func (c *Client) Import(ctx context.Context, req ImportRequest) (ImportResponse, error) {
	var resp ImportResponse
	err := c.do(ctx, http.MethodPost, c.scopedPath("/import"), nil, req, &resp)
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

	endpoint := c.baseURL + c.scopedPath("/import/stream")
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+c.scopedPath("/export"), nil)
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
	err := c.do(ctx, http.MethodPost, c.scopedPath("/deps"), nil, req, &resp)
	return resp, err
}

// AddLabels adds labels to a task via POST /v1/tasks/{id}/labels.
func (c *Client) AddLabels(ctx context.Context, id string, req LabelsRequest) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodPost, c.scopedPath("/tasks/"+url.PathEscape(id)+"/labels"), nil, req, &resp)
	return resp, err
}

// RemoveLabels removes labels from a task via DELETE /v1/tasks/{id}/labels.
func (c *Client) RemoveLabels(ctx context.Context, id string, req LabelsRequest) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodDelete, c.scopedPath("/tasks/"+url.PathEscape(id)+"/labels"), nil, req, &resp)
	return resp, err
}

// ListLabels lists labels for a task via GET /v1/tasks/{id}/labels.
func (c *Client) ListLabels(ctx context.Context, id string) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodGet, c.scopedPath("/tasks/"+url.PathEscape(id)+"/labels"), nil, nil, &resp)
	return resp, err
}

// ListAllLabels lists all labels in the project via GET /v1/labels.
func (c *Client) ListAllLabels(ctx context.Context) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodGet, c.scopedPath("/labels"), nil, nil, &resp)
	return resp, err
}

// CreateTaskGitRef creates one git reference for a task via POST /v1/tasks/{id}/git-refs.
func (c *Client) CreateTaskGitRef(ctx context.Context, taskID string, req TaskGitRefCreateRequest) (models.TaskGitRef, error) {
	var resp models.TaskGitRef
	err := c.do(ctx, http.MethodPost, c.scopedPath("/tasks/"+url.PathEscape(taskID)+"/git-refs"), nil, req, &resp)
	return resp, err
}

// ListTaskGitRefs lists git references for one task via GET /v1/tasks/{id}/git-refs.
func (c *Client) ListTaskGitRefs(ctx context.Context, taskID string) ([]models.TaskGitRef, error) {
	var resp []models.TaskGitRef
	err := c.do(ctx, http.MethodGet, c.scopedPath("/tasks/"+url.PathEscape(taskID)+"/git-refs"), nil, nil, &resp)
	return resp, err
}

// GetTaskGitRef fetches one git reference by id via GET /v1/git-refs/{ref_id}.
func (c *Client) GetTaskGitRef(ctx context.Context, refID string) (models.TaskGitRef, error) {
	var resp models.TaskGitRef
	err := c.do(ctx, http.MethodGet, c.scopedPath("/git-refs/"+url.PathEscape(refID)), nil, nil, &resp)
	return resp, err
}

// DeleteTaskGitRef deletes one git reference by id via DELETE /v1/git-refs/{ref_id}.
func (c *Client) DeleteTaskGitRef(ctx context.Context, refID string) (map[string]any, error) {
	var resp map[string]any
	err := c.do(ctx, http.MethodDelete, c.scopedPath("/git-refs/"+url.PathEscape(refID)), nil, nil, &resp)
	return resp, err
}

// CreateTaskAttachment uploads managed attachment content via POST /v1/tasks/{id}/attachments.
func (c *Client) CreateTaskAttachment(ctx context.Context, taskID string, req AttachmentUploadRequest, content io.Reader) (models.Attachment, error) {
	var resp models.Attachment
	if content == nil {
		return resp, fmt.Errorf("content is required")
	}

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	if err := writer.WriteField("kind", req.Kind); err != nil {
		return resp, err
	}
	if req.Title != "" {
		if err := writer.WriteField("title", req.Title); err != nil {
			return resp, err
		}
	}
	if req.MediaType != "" {
		if err := writer.WriteField("media_type", req.MediaType); err != nil {
			return resp, err
		}
	}
	if req.ExpiresAt != nil {
		if err := writer.WriteField("expires_at", req.ExpiresAt.UTC().Format(time.RFC3339)); err != nil {
			return resp, err
		}
	}
	for _, label := range req.Labels {
		if strings.TrimSpace(label) == "" {
			continue
		}
		if err := writer.WriteField("label", label); err != nil {
			return resp, err
		}
	}

	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "attachment.bin"
	}
	part, err := writer.CreateFormFile("content", filename)
	if err != nil {
		return resp, err
	}
	if _, err := io.Copy(part, content); err != nil {
		return resp, err
	}
	if err := writer.Close(); err != nil {
		return resp, err
	}

	endpoint := c.baseURL + c.scopedPath("/tasks/"+url.PathEscape(taskID)+"/attachments")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload.Bytes()))
	if err != nil {
		return resp, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	c.setAuthHeader(httpReq)

	httpResp, err := c.http.Do(httpReq)
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

// CreateTaskAttachmentLink creates a link/repo attachment via POST /v1/tasks/{id}/attachments/link.
func (c *Client) CreateTaskAttachmentLink(ctx context.Context, taskID string, req AttachmentCreateLinkRequest) (models.Attachment, error) {
	var resp models.Attachment
	err := c.do(ctx, http.MethodPost, c.scopedPath("/tasks/"+url.PathEscape(taskID)+"/attachments/link"), nil, req, &resp)
	return resp, err
}

// ListTaskAttachments lists task attachments via GET /v1/tasks/{id}/attachments.
func (c *Client) ListTaskAttachments(ctx context.Context, taskID string) ([]models.Attachment, error) {
	var resp []models.Attachment
	err := c.do(ctx, http.MethodGet, c.scopedPath("/tasks/"+url.PathEscape(taskID)+"/attachments"), nil, nil, &resp)
	return resp, err
}

// GetAttachment fetches an attachment by id via GET /v1/attachments/{attachment_id}.
func (c *Client) GetAttachment(ctx context.Context, attachmentID string) (models.Attachment, error) {
	var resp models.Attachment
	err := c.do(ctx, http.MethodGet, c.scopedPath("/attachments/"+url.PathEscape(attachmentID)), nil, nil, &resp)
	return resp, err
}

// GetAttachmentContent streams managed content via GET /v1/attachments/{attachment_id}/content.
func (c *Client) GetAttachmentContent(ctx context.Context, attachmentID string, w io.Writer) error {
	if w == nil {
		return fmt.Errorf("writer is required")
	}
	endpoint := c.baseURL + c.scopedPath("/attachments/"+url.PathEscape(attachmentID)) + "/content"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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

// DeleteAttachment deletes an attachment via DELETE /v1/attachments/{attachment_id}.
func (c *Client) DeleteAttachment(ctx context.Context, attachmentID string) (map[string]any, error) {
	var resp map[string]any
	err := c.do(ctx, http.MethodDelete, c.scopedPath("/attachments/"+url.PathEscape(attachmentID)), nil, nil, &resp)
	return resp, err
}

// AdminGCBlobs executes blob garbage collection via POST /v1/admin/gc-blobs.
// If confirm is true, X-Confirm is sent to execute deletion; otherwise it is a dry-run.
func (c *Client) AdminGCBlobs(ctx context.Context, req BlobGCRequest, confirm bool) (BlobGCResponse, error) {
	var resp BlobGCResponse
	payload, err := json.Marshal(req)
	if err != nil {
		return resp, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/admin/gc-blobs", bytes.NewReader(payload))
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
	return errors.As(err, &netErr)
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

func normalizeProject(project string) string {
	project = strings.TrimSpace(strings.ToLower(project))
	if len(project) != 2 {
		return defaultProject
	}
	for _, r := range project {
		if r < 'a' || r > 'z' {
			return defaultProject
		}
	}
	return project
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
