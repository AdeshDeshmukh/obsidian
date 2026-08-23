package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"obsidian/internal/queue"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
)

// RunScheduler starts the cron polling loop
func RunScheduler(ctx context.Context, pool *pgxpool.Pool, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	slog.Info("Cron scheduler service started")

	// Standard cron parser (Minute Hour DayOfMonth Month DayOfWeek)
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Cron scheduler service stopping")
			return
		case <-ticker.C:
			if err := dispatchDueJobs(ctx, pool, parser); err != nil {
				slog.Error("Error dispatching due cron jobs", "error", err)
			}

			// Reclaim expired worker leases and return jobs to 'queued' state
			reclaimed, err := queue.ReclaimExpired(ctx, pool)
			if err != nil {
				slog.Error("Error reclaiming expired worker leases", "error", err)
			} else if reclaimed > 0 {
				slog.Info("Reclaimed expired worker leases", "count", reclaimed)
			}
		}
	}
}

type dueJob struct {
	id              string
	queueID         string
	cronExpr        string
	jobType         string
	payloadTemplate []byte
	nextRunAt       time.Time
}

func dispatchDueJobs(ctx context.Context, pool *pgxpool.Pool, parser cron.Parser) error {
	// Find active scheduled jobs that are due
	rows, err := pool.Query(ctx, `
		SELECT id, queue_id, cron_expr, job_type, payload_template, next_run_at
		FROM scheduled_jobs
		WHERE is_active = true AND next_run_at <= now()
	`)
	if err != nil {
		return fmt.Errorf("error querying scheduled jobs: %w", err)
	}
	defer rows.Close()

	var dues []dueJob
	for rows.Next() {
		var d dueJob
		if err := rows.Scan(&d.id, &d.queueID, &d.cronExpr, &d.jobType, &d.payloadTemplate, &d.nextRunAt); err != nil {
			return fmt.Errorf("error scanning scheduled jobs: %w", err)
		}
		dues = append(dues, d)
	}

	for _, due := range dues {
		// Acquire advisory lock per queue to serialize insertion across replicas
		err := WithQueueLock(ctx, pool, due.queueID, func() error {
			tx, err := pool.Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx)

			// Row lock the cron definition to check state double-dispatch prevention
			var currentNextRunAt time.Time
			err = tx.QueryRow(ctx, `
				SELECT next_run_at FROM scheduled_jobs WHERE id = $1 FOR UPDATE
			`, due.id).Scan(&currentNextRunAt)
			if err != nil {
				return err
			}

			// If it's already updated to a future time, skip it
			if currentNextRunAt.After(time.Now()) {
				return nil
			}

			// Parse cron expr and compute next time
			sched, err := parser.Parse(due.cronExpr)
			if err != nil {
				return fmt.Errorf("invalid cron expression %q: %w", due.cronExpr, err)
			}
			nextTime := sched.Next(time.Now())

			// Insert job instance
			_, err = tx.Exec(ctx, `
				INSERT INTO jobs (queue_id, job_type, payload, status, run_at, created_at, updated_at)
				VALUES ($1, $2, $3, 'queued', $4, now(), now())
			`, due.queueID, due.jobType, due.payloadTemplate, due.nextRunAt)
			if err != nil {
				return fmt.Errorf("failed to insert cron job instance: %w", err)
			}

			// Update schedule rule next_run_at
			_, err = tx.Exec(ctx, `
				UPDATE scheduled_jobs
				SET next_run_at = $1
				WHERE id = $2
			`, nextTime, due.id)
			if err != nil {
				return fmt.Errorf("failed to update scheduled job run time: %w", err)
			}

			slog.Info("Dispatched recurring job", "job_id", due.id, "job_type", due.jobType, "queue_id", due.queueID, "next_run", nextTime)
			return tx.Commit(ctx)
		})
		if err != nil {
			slog.Error("Lock acquisition or dispatch failed for scheduled job", "job_id", due.id, "error", err)
		}
	}

	return nil
}
