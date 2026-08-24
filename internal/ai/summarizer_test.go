package ai_test

import (
	"context"
	"strings"
	"testing"

	"obsidian/internal/ai"
)

func TestSummarizeFailure_Heuristics(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		jobType      string
		errorMessage string
		logs         []string
		expectedSub  string
	}{
		{
			name:         "Explicit failure simulation",
			jobType:      "fail",
			errorMessage: "explicit failure simulated",
			logs:         []string{"Simulating error for test"},
			expectedSub:  "Test exception trigger executed",
		},
		{
			name:         "Timeout deadline exceeded",
			jobType:      "heavy_compute",
			errorMessage: "context deadline exceeded",
			logs:         []string{"Processing chunk 4/10", "Timeout reached"},
			expectedSub:  "exceeded maximum lease or HTTP timeout",
		},
		{
			name:         "Network connection refused",
			jobType:      "webhook_dispatch",
			errorMessage: "dial tcp 127.0.0.1:9000: connection refused",
			logs:         []string{"POST https://api.partner.com"},
			expectedSub:  "Upstream dependency network failure",
		},
		{
			name:         "JSON unmarshal error",
			jobType:      "process_order",
			errorMessage: "invalid payload: json: cannot unmarshal string into Go struct",
			logs:         []string{"Parsing payload"},
			expectedSub:  "Malformed payload schema mismatch",
		},
		{
			name:         "Generic application error",
			jobType:      "custom_worker",
			errorMessage: "out of disk space on /tmp",
			logs:         []string{"Writing temporary artifact"},
			expectedSub:  "Handler execution failed for job 'custom_worker'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := ai.SummarizeFailure(ctx, tt.jobType, tt.errorMessage, tt.logs)
			if !strings.Contains(summary, tt.expectedSub) {
				t.Errorf("expected summary to contain %q, got: %q", tt.expectedSub, summary)
			}
		})
	}
}
