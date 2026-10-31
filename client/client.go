package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

func New(baseURL, token string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewBuffer(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(respBody, &errResp)
		if errResp.Error != "" {
			return fmt.Errorf("API error %d: %s", resp.StatusCode, errResp.Error)
		}
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	if out != nil && len(respBody) > 0 {
		return json.Unmarshal(respBody, out)
	}
	return nil
}

func (c *Client) Enqueue(ctx context.Context, queueID string, req EnqueueRequest) (*EnqueueResponse, error) {
	var resp EnqueueResponse
	if err := c.do(ctx, http.MethodPost, "/api/queues/"+queueID+"/jobs", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetJob(ctx context.Context, jobID string) (*Job, error) {
	var job Job
	if err := c.do(ctx, http.MethodGet, "/api/jobs/"+jobID, nil, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) CancelJob(ctx context.Context, jobID string) error {
	return c.do(ctx, http.MethodPost, "/api/jobs/"+jobID+"/cancel", nil, nil)
}

func (c *Client) RetryJob(ctx context.Context, jobID string) error {
	return c.do(ctx, http.MethodPost, "/api/jobs/"+jobID+"/retry", nil, nil)
}

func (c *Client) SetJobProgress(ctx context.Context, jobID string, pct int) error {
	payload := map[string]int{"progress": pct}
	return c.do(ctx, http.MethodPost, "/api/jobs/"+jobID+"/progress", payload, nil)
}

func (c *Client) ListQueues(ctx context.Context, projectID string) ([]Queue, error) {
	var queues []Queue
	if err := c.do(ctx, http.MethodGet, "/api/projects/"+projectID+"/queues", nil, &queues); err != nil {
		return nil, err
	}
	return queues, nil
}

func (c *Client) ListJobs(ctx context.Context, queueID string, page, limit int) (*ListJobsResponse, error) {
	path := fmt.Sprintf("/api/queues/%s/jobs?page=%d&limit=%d", queueID, page, limit)
	var resp ListJobsResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListWorkers(ctx context.Context) ([]Worker, error) {
	var workers []Worker
	if err := c.do(ctx, http.MethodGet, "/api/workers", nil, &workers); err != nil {
		return nil, err
	}
	return workers, nil
}

