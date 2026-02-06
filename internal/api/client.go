package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a simple HTTP client for the grns API.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Ping checks whether the API server is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/health", nil, nil, nil)
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
