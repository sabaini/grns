package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout = 10 * time.Second
	httpTimeoutEnvKey  = "GRNS_HTTP_TIMEOUT"
	apiTokenEnvKey     = "GRNS_API_TOKEN"
	adminTokenEnvKey   = "GRNS_ADMIN_TOKEN"
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

func (c *Client) GetInfo(ctx context.Context) (InfoResponse, error) {
	var resp InfoResponse
	err := c.do(ctx, http.MethodGet, "/v1/info", nil, nil, &resp)
	return resp, err
}

func (c *Client) CreateTask(ctx context.Context, req TaskCreateRequest) (TaskResponse, error) {
	var resp TaskResponse
	err := c.do(ctx, http.MethodPost, "/v1/tasks", nil, req, &resp)
	return resp, err
}

func (c *Client) BatchCreate(ctx context.Context, req []TaskCreateRequest) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodPost, "/v1/tasks/batch", nil, req, &resp)
	return resp, err
}

func (c *Client) GetTask(ctx context.Context, id string) (TaskResponse, error) {
	var resp TaskResponse
	err := c.do(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id), nil, nil, &resp)
	return resp, err
}

func (c *Client) UpdateTask(ctx context.Context, id string, req TaskUpdateRequest) (TaskResponse, error) {
	var resp TaskResponse
	err := c.do(ctx, http.MethodPatch, "/v1/tasks/"+url.PathEscape(id), nil, req, &resp)
	return resp, err
}

func (c *Client) ListTasks(ctx context.Context, query url.Values) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodGet, "/v1/tasks", query, nil, &resp)
	return resp, err
}

func (c *Client) Ready(ctx context.Context, query url.Values) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodGet, "/v1/tasks/ready", query, nil, &resp)
	return resp, err
}

func (c *Client) Stale(ctx context.Context, query url.Values) ([]TaskResponse, error) {
	var resp []TaskResponse
	err := c.do(ctx, http.MethodGet, "/v1/tasks/stale", query, nil, &resp)
	return resp, err
}

func (c *Client) CloseTasks(ctx context.Context, req TaskCloseRequest) (map[string]any, error) {
	var resp map[string]any
	err := c.do(ctx, http.MethodPost, "/v1/tasks/close", nil, req, &resp)
	return resp, err
}

func (c *Client) ReopenTasks(ctx context.Context, req TaskReopenRequest) (map[string]any, error) {
	var resp map[string]any
	err := c.do(ctx, http.MethodPost, "/v1/tasks/reopen", nil, req, &resp)
	return resp, err
}

func (c *Client) DependencyTree(ctx context.Context, id string) (DepTreeResponse, error) {
	var resp DepTreeResponse
	err := c.do(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id)+"/deps/tree", nil, nil, &resp)
	return resp, err
}

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
func (c *Client) ImportStream(ctx context.Context, records io.Reader, dryRun bool, dedupe, orphanHandling string) (ImportResponse, error) {
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

func (c *Client) AddDependency(ctx context.Context, req DepCreateRequest) (map[string]any, error) {
	var resp map[string]any
	err := c.do(ctx, http.MethodPost, "/v1/deps", nil, req, &resp)
	return resp, err
}

func (c *Client) AddLabels(ctx context.Context, id string, req LabelsRequest) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodPost, "/v1/tasks/"+url.PathEscape(id)+"/labels", nil, req, &resp)
	return resp, err
}

func (c *Client) RemoveLabels(ctx context.Context, id string, req LabelsRequest) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodDelete, "/v1/tasks/"+url.PathEscape(id)+"/labels", nil, req, &resp)
	return resp, err
}

func (c *Client) ListLabels(ctx context.Context, id string) ([]string, error) {
	var resp []string
	err := c.do(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id)+"/labels", nil, nil, &resp)
	return resp, err
}

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

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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

	if out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func decodeError(resp *http.Response) error {
	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
		if errResp.Code != "" {
			return fmt.Errorf("%s: %s", errResp.Code, errResp.Error)
		}
		return fmt.Errorf("%s", errResp.Error)
	}
	return fmt.Errorf("api error: %s", resp.Status)
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
