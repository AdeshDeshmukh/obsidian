package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestCommandHierarchy(t *testing.T) {
	root := newRootCmd()
	if root.Use != "obsidianctl" {
		t.Errorf("expected root command obsidianctl, got %s", root.Use)
	}

	commands := root.Commands()
	found := make(map[string]bool)
	for _, c := range commands {
		found[c.Name()] = true
	}

	for _, expected := range []string{"queue", "jobs", "workers"} {
		if !found[expected] {
			t.Errorf("expected subcommand %q not found in root", expected)
		}
	}
}

func TestListQueues(t *testing.T) {
	oldClient := httpClient
	httpClient = &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/projects/proj-1/queues" {
				t.Errorf("unexpected path: %s", req.URL.Path)
			}
			jsonBody := `[{"id":"q-1","name":"billing","state":"active","concurrency_limit":10,"priority":5}]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(jsonBody)),
				Header:     make(http.Header),
			}, nil
		}),
	}
	defer func() { httpClient = oldClient }()

	var buf bytes.Buffer
	err := listQueues(&buf, "proj-1")
	if err != nil {
		t.Fatalf("listQueues returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "billing") || !strings.Contains(out, "active") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestUpdateQueueState(t *testing.T) {
	oldClient := httpClient
	var capturedPath string
	var capturedBody string
	httpClient = &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			capturedPath = req.URL.Path
			b, _ := io.ReadAll(req.Body)
			capturedBody = string(b)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":"ok"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}
	defer func() { httpClient = oldClient }()

	var buf bytes.Buffer
	err := updateQueueState(&buf, "q-1", "paused")
	if err != nil {
		t.Fatalf("updateQueueState returned error: %v", err)
	}

	if capturedPath != "/api/queues/q-1" {
		t.Errorf("expected path /api/queues/q-1, got %s", capturedPath)
	}
	if !strings.Contains(capturedBody, `"paused"`) {
		t.Errorf("expected body to contain paused, got %s", capturedBody)
	}
}
