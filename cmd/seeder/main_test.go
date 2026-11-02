package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestDefaultTemplates(t *testing.T) {
	if len(defaultJobTemplates) == 0 {
		t.Fatal("expected non-empty defaultJobTemplates")
	}

	for _, tmpl := range defaultJobTemplates {
		if tmpl.QueueName == "" {
			t.Errorf("job template missing queueName: %+v", tmpl)
		}
		if tmpl.JobType == "" {
			t.Errorf("job template missing jobType: %+v", tmpl)
		}
		if tmpl.Priority <= 0 {
			t.Errorf("job template priority must be > 0: %+v", tmpl)
		}
	}
}

func TestEnqueueSuccess(t *testing.T) {
	var capturedAuth string
	var capturedBody string

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			capturedAuth = req.Header.Get("Authorization")
			b, _ := io.ReadAll(req.Body)
			capturedBody = string(b)
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader(`{"id":"job-123"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	oldClient := httpClient
	httpClient = client
	defer func() { httpClient = oldClient }()

	ctx := context.Background()
	err := enqueue(ctx, "http://localhost:8080", "test-token", "", "q-1", "log", map[string]interface{}{"msg": "hi"}, 5)
	if err != nil {
		t.Fatalf("enqueue returned unexpected error: %v", err)
	}

	if capturedAuth != "Bearer test-token" {
		t.Errorf("expected Bearer test-token, got %s", capturedAuth)
	}
	if !strings.Contains(capturedBody, `"job_type":"log"`) {
		t.Errorf("expected job_type log in body: %s", capturedBody)
	}
}
