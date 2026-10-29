package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	JobsQueued = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "obsidian_jobs_queued_total",
		Help: "Number of jobs currently in queued state per queue.",
	}, []string{"queue_id", "queue_name"})

	JobsCompleted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "obsidian_jobs_completed_total",
		Help: "Total number of jobs completed successfully per queue.",
	}, []string{"queue_id"})

	JobsFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "obsidian_jobs_failed_total",
		Help: "Total number of jobs that failed (moved to DLQ) per queue.",
	}, []string{"queue_id"})

	DLQSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "obsidian_dlq_size",
		Help: "Number of entries currently in the dead letter queue per queue.",
	}, []string{"queue_id"})

	WorkersActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "obsidian_workers_active",
		Help: "Number of active worker nodes with heartbeats within last 30s.",
	})

	ClaimDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "obsidian_claim_duration_ms",
		Help:    "Duration of ClaimBatch execution in milliseconds.",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
	})
)

func RecordClaimDuration(start time.Time) {
	ClaimDuration.Observe(float64(time.Since(start).Milliseconds()))
}

func ScrapeDBMetrics(ctx context.Context, pool *pgxpool.Pool) {
	rows, err := pool.Query(ctx, `
		SELECT
			q.id,
			q.name,
			COUNT(j.id) FILTER (WHERE j.status = 'queued') AS queued_count,
			COUNT(dlq.id) AS dlq_count
		FROM queues q
		LEFT JOIN jobs j ON j.queue_id = q.id
		LEFT JOIN dead_letter_queue dlq ON dlq.queue_id = q.id
		WHERE q.deleted_at IS NULL
		GROUP BY q.id, q.name
	`)
	if err != nil {
		slog.Error("failed to scrape database metrics", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var queueID, queueName string
		var queuedCount, dlqCount float64
		if err := rows.Scan(&queueID, &queueName, &queuedCount, &dlqCount); err != nil {
			continue
		}
		JobsQueued.WithLabelValues(queueID, queueName).Set(queuedCount)
		DLQSize.WithLabelValues(queueID).Set(dlqCount)
	}

	var activeWorkers float64
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM workers WHERE status = 'active' AND last_seen_at > now() - interval '30 seconds'
	`).Scan(&activeWorkers)
	WorkersActive.Set(activeWorkers)
}

func StartMetricsScraper(ctx context.Context, pool *pgxpool.Pool) {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ScrapeDBMetrics(ctx, pool)
			}
		}
	}()
}
