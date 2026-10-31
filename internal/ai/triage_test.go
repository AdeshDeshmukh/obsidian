package ai

import (
	"context"
	"strings"
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		err     string
		pattern string
	}{
		{"context deadline exceeded while fetching resource", "timeout"},
		{"dial tcp 10.0.0.1:80: connection refused", "network_failure"},
		{"failed to unmarshal JSON payload into struct", "payload_error"},
		{"explicit failure simulated", "test_failure"},
		{"worker lease expired before completion", "worker_crash"},
		{"random unexpected error from application", "unknown_error"},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.pattern {
				t.Errorf("ClassifyError(%q) = %q; want %q", tt.err, got, tt.pattern)
			}
		})
	}
}

func TestTriageDLQEmpty(t *testing.T) {
	res := TriageDLQ(context.Background(), nil)
	if res.TotalFailed != 0 {
		t.Errorf("expected TotalFailed 0, got %d", res.TotalFailed)
	}
	if len(res.Groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(res.Groups))
	}
	if res.AISummary != "No failures to triage." {
		t.Errorf("unexpected AISummary: %s", res.AISummary)
	}
}

func TestTriageDLQGrouping(t *testing.T) {
	failures := []FailureItem{
		{JobType: "email.send", FailureReason: "connection refused"},
		{JobType: "email.send", FailureReason: "connection refused: host down"},
		{JobType: "report.generate", FailureReason: "deadline exceeded"},
		{JobType: "email.send", FailureReason: "connection refused: port 25 closed"},
		{JobType: "email.digest", FailureReason: "connection refused: server unreachable"},
	}

	res := TriageDLQ(context.Background(), failures)
	if res.TotalFailed != 5 {
		t.Fatalf("expected TotalFailed 5, got %d", res.TotalFailed)
	}

	patternCounts := make(map[string]int)
	for _, g := range res.Groups {
		patternCounts[g.Pattern] = g.Count
		if g.Pattern == "network_failure" {
			if len(g.Samples) > 3 {
				t.Errorf("samples exceeding limit of 3: %d", len(g.Samples))
			}
			if len(g.JobTypes) != 2 {
				t.Errorf("expected 2 distinct job types, got %v", g.JobTypes)
			}
		}
	}

	if patternCounts["network_failure"] != 4 {
		t.Errorf("expected 4 network_failure, got %d", patternCounts["network_failure"])
	}
	if patternCounts["timeout"] != 1 {
		t.Errorf("expected 1 timeout, got %d", patternCounts["timeout"])
	}
	if !strings.Contains(res.AISummary, "network failures indicate upstream") {
		t.Errorf("expected network failure heuristic recommendation, got %s", res.AISummary)
	}
}
