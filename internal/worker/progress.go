package worker

import (
	"context"
	"log/slog"

	"obsidian/internal/api/ws"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ProgressReporter func(pct int)

func NewProgressReporter(ctx context.Context, pool *pgxpool.Pool, jobID, queueID string) ProgressReporter {
	return func(pct int) {
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}

		_, err := pool.Exec(ctx, `
			UPDATE job_executions
			SET progress = $1
			WHERE job_id = $2 AND finished_at IS NULL
		`, pct, jobID)
		if err != nil {
			slog.Warn("Failed to update job progress", "job_id", jobID, "progress", pct, "error", err)
			return
		}

		ws.Broadcast("job.progress", map[string]interface{}{
			"id":       jobID,
			"queue_id": queueID,
			"progress": pct,
		})

		slog.Info("Job progress updated", "job_id", jobID, "progress", pct)
	}
}

type progressKey struct{}

func WithProgressReporter(ctx context.Context, reporter ProgressReporter) context.Context {
	return context.WithValue(ctx, progressKey{}, reporter)
}

func GetProgressReporter(ctx context.Context) ProgressReporter {
	if r, ok := ctx.Value(progressKey{}).(ProgressReporter); ok {
		return r
	}
	return func(pct int) {}
}

