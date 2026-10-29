package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"obsidian/internal/metrics"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestRecordClaimDuration(t *testing.T) {
	start := time.Now().Add(-50 * time.Millisecond)
	metrics.RecordClaimDuration(start)
}

func TestMetricVectorsIncrement(t *testing.T) {
	metrics.JobsQueued.WithLabelValues("queue-1", "default").Set(42)
	metrics.JobsCompleted.WithLabelValues("queue-1").Inc()
	metrics.JobsFailed.WithLabelValues("queue-1").Inc()
	metrics.DLQSize.WithLabelValues("queue-1").Set(5)
	metrics.WorkersActive.Set(3)
}

func TestPrometheusMetricsEndpoint(t *testing.T) {
	metrics.JobsQueued.WithLabelValues("test-queue-id", "test-queue").Set(12)
	metrics.RecordClaimDuration(time.Now().Add(-10 * time.Millisecond))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	handler := promhttp.Handler()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	expectedMetrics := []string{
		"obsidian_jobs_queued_total",
		"obsidian_jobs_completed_total",
		"obsidian_jobs_failed_total",
		"obsidian_dlq_size",
		"obsidian_workers_active",
		"obsidian_claim_duration_ms",
	}

	for _, metricName := range expectedMetrics {
		if !strings.Contains(body, metricName) {
			t.Errorf("expected response to contain metric %q", metricName)
		}
	}
}
