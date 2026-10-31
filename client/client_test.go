package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mockTransport func(req *http.Request) (*http.Response, error)

func (m mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func newMockClient(fn mockTransport) *Client {
	hc := &http.Client{Transport: fn}
	return New("http://test.local", "test-token", WithHTTPClient(hc))
}

func TestClientEnqueue(t *testing.T) {
	ctx := context.Background()
	c := newMockClient(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", req.Method)
		}
		if req.URL.Path != "/api/queues/queue-123/jobs" {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
		if auth := req.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", auth)
		}

		body := `{"id":"job-abc","status":"queued","run_at":"2026-10-31T12:00:00Z"}`
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})

	resp, err := c.Enqueue(ctx, "queue-123", EnqueueRequest{
		JobType: "email.send",
		Payload: map[string]interface{}{"to": "test@example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "job-abc" || resp.Status != "queued" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestClientGetJob(t *testing.T) {
	ctx := context.Background()
	c := newMockClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/jobs/job-abc" {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}

		body := `{"id":"job-abc","queue_id":"queue-123","job_type":"email.send","status":"running","progress":45}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})

	job, err := c.GetJob(ctx, "job-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.ID != "job-abc" || job.Progress != 45 || job.Status != "running" {
		t.Fatalf("unexpected job data: %+v", job)
	}
}

func TestClientJobActions(t *testing.T) {
	ctx := context.Background()

	// Cancel
	cCancel := newMockClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/jobs/job-1/cancel" {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":"cancelled"}`)),
			Header:     make(http.Header),
		}, nil
	})
	if err := cCancel.CancelJob(ctx, "job-1"); err != nil {
		t.Fatalf("CancelJob failed: %v", err)
	}

	// Retry
	cRetry := newMockClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/jobs/job-1/retry" {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id":"job-1","status":"queued"}`)),
			Header:     make(http.Header),
		}, nil
	})
	if err := cRetry.RetryJob(ctx, "job-1"); err != nil {
		t.Fatalf("RetryJob failed: %v", err)
	}

	// SetJobProgress
	cProgress := newMockClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/jobs/job-1/progress" {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"job_id":"job-1","progress":80,"updated":true}`)),
			Header:     make(http.Header),
		}, nil
	})
	if err := cProgress.SetJobProgress(ctx, "job-1", 80); err != nil {
		t.Fatalf("SetJobProgress failed: %v", err)
	}
}

func TestClientListQueries(t *testing.T) {
	ctx := context.Background()

	// ListQueues
	cQueues := newMockClient(func(req *http.Request) (*http.Response, error) {
		body := `[{"id":"q-1","name":"default","state":"active","concurrency_limit":10,"priority":1,"is_paused":false}]`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	queues, err := cQueues.ListQueues(ctx, "proj-1")
	if err != nil || len(queues) != 1 || queues[0].State != "active" {
		t.Fatalf("ListQueues failed: %v, %+v", err, queues)
	}

	// ListWorkers
	cWorkers := newMockClient(func(req *http.Request) (*http.Response, error) {
		body := `[{"id":"w-1","hostname":"worker-host","status":"active"}]`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	workers, err := cWorkers.ListWorkers(ctx)
	if err != nil || len(workers) != 1 || workers[0].Hostname != "worker-host" {
		t.Fatalf("ListWorkers failed: %v, %+v", err, workers)
	}

	// ListJobs
	cJobs := newMockClient(func(req *http.Request) (*http.Response, error) {
		body := `{"data":[{"id":"j-1","status":"queued"}],"total":1,"page":1,"limit":20}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	jobsResp, err := cJobs.ListJobs(ctx, "q-1", 1, 20)
	if err != nil || jobsResp.Total != 1 || len(jobsResp.Data) != 1 {
		t.Fatalf("ListJobs failed: %v, %+v", err, jobsResp)
	}
}

func TestClientAPIError(t *testing.T) {
	ctx := context.Background()
	c := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{"error":"queue not found"}`)),
			Header:     make(http.Header),
		}, nil
	})

	_, err := c.Enqueue(ctx, "nonexistent", EnqueueRequest{JobType: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "queue not found") {
		t.Fatalf("expected 'queue not found' in error, got %v", err)
	}
}

