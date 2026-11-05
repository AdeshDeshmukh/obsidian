package metrics

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"
)

type QueueHealth struct {
	QueueID       string  `json:"queue_id"`
	QueueName     string  `json:"queue_name"`
	Score         int     `json:"score"`
	Grade         string  `json:"grade"`
	DLQRate       float64 `json:"dlq_rate"`
	FailRate      float64 `json:"fail_rate"`
	QueueDepth    int     `json:"queue_depth"`
	ActiveWorkers int     `json:"active_workers"`
}

func ComputeScore(dlqRate, failRate float64, queueDepth int) (int, string) {
	dlqPenalty := math.Min(dlqRate*500, 50)
	failPenalty := math.Min(failRate*150, 30)
	depthPenalty := math.Min(float64(queueDepth)/50, 20)

	score := 100.0 - dlqPenalty - failPenalty - depthPenalty
	if score < 0 {
		score = 0
	}

	roundedScore := int(math.Round(score))

	var grade string
	switch {
	case roundedScore >= 90:
		grade = "A"
	case roundedScore >= 75:
		grade = "B"
	case roundedScore >= 60:
		grade = "C"
	case roundedScore >= 40:
		grade = "D"
	default:
		grade = "F"
	}

	return roundedScore, grade
}

func GetQueueHealth(ctx context.Context, pool *pgxpool.Pool, queueID string) (*QueueHealth, error) {
	h := &QueueHealth{QueueID: queueID}

	query := `
		SELECT
			q.name,
			COALESCE((SELECT COUNT(*) FROM jobs WHERE queue_id = q.id AND status = 'queued'), 0) AS queue_depth,
			COALESCE((
				SELECT CASE WHEN COUNT(*) > 0 THEN
					COUNT(*) FILTER (WHERE status = 'dead_letter')::float / COUNT(*)
				ELSE 0 END
				FROM jobs
				WHERE queue_id = q.id AND updated_at > now() - interval '1 hour'
			), 0) AS dlq_rate,
			COALESCE((
				SELECT CASE WHEN COUNT(je.id) > 0 THEN
					COUNT(je.id) FILTER (WHERE je.outcome = 'failure')::float / COUNT(je.id)
				ELSE 0 END
				FROM jobs j
				JOIN job_executions je ON je.job_id = j.id
				WHERE j.queue_id = q.id AND je.started_at > now() - interval '1 hour'
			), 0) AS fail_rate,
			COALESCE((
				SELECT COUNT(DISTINCT claimed_by)
				FROM jobs
				WHERE queue_id = q.id AND status = 'running' AND claimed_by IS NOT NULL
			), 0) AS active_workers
		FROM queues q
		WHERE q.id = $1 AND q.deleted_at IS NULL
	`

	err := pool.QueryRow(ctx, query, queueID).Scan(
		&h.QueueName,
		&h.QueueDepth,
		&h.DLQRate,
		&h.FailRate,
		&h.ActiveWorkers,
	)
	if err != nil {
		return nil, err
	}

	h.Score, h.Grade = ComputeScore(h.DLQRate, h.FailRate, h.QueueDepth)
	return h, nil
}
