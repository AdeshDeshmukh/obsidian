package queue

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RetryPolicy struct {
	Strategy    string // fixed | linear | exponential
	BaseDelayMs int
	MaxDelayMs  int
	MaxAttempts int
}

// ComputeBackoff calculates delay based on policy strategy and attempt count
func ComputeBackoff(p RetryPolicy, attempt int) time.Duration {
	var delay time.Duration
	base := time.Duration(p.BaseDelayMs) * time.Millisecond
	maxDelay := time.Duration(p.MaxDelayMs) * time.Millisecond

	switch p.Strategy {
	case "fixed":
		delay = base
	case "linear":
		delay = base * time.Duration(attempt+1)
	case "exponential":
		delay = time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	default:
		delay = base
	}

	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

// MoveToDeadLetter moves job to dead letter queue table and marks job status as 'dead_letter'
func MoveToDeadLetter(ctx context.Context, pool *pgxpool.Pool, job ClaimedJob, errMsg string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Update job status
	_, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status = 'dead_letter', updated_at = now()
		WHERE id = $1
	`, job.ID)
	if err != nil {
		return err
	}

	// Insert execution log update
	_, err = tx.Exec(ctx, `
		UPDATE job_executions
		SET finished_at = now(), outcome = 'failure', error_message = $1,
		    duration_ms = EXTRACT(EPOCH FROM (now() - started_at)) * 1000
		WHERE job_id = $2 AND finished_at IS NULL
	`, errMsg, job.ID)
	if err != nil {
		return err
	}

	// Insert into DLQ
	_, err = tx.Exec(ctx, `
		INSERT INTO dead_letter_queue (original_job_id, queue_id, payload, failure_reason, attempts_made, moved_at)
		VALUES ($1, $2, $3, $4, $5, now())
	`, job.ID, job.QueueID, job.Payload, errMsg, job.Attempt+1)
	if err != nil {
		return err
	}

	// Add log entry
	_, err = tx.Exec(ctx, `
		INSERT INTO job_logs (job_id, level, message)
		VALUES ($1, 'error', $2)
	`, job.ID, "Job moved to Dead Letter Queue: "+errMsg)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// RequeueWithDelay resets job state to 'queued' or 'scheduled' with a calculated run delay
func RequeueWithDelay(ctx context.Context, pool *pgxpool.Pool, jobID string, delay time.Duration, errMsg string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	runAt := time.Now().Add(delay)

	// Update execution outcome
	_, err = tx.Exec(ctx, `
		UPDATE job_executions
		SET finished_at = now(), outcome = 'failure', error_message = $1,
		    duration_ms = EXTRACT(EPOCH FROM (now() - started_at)) * 1000
		WHERE job_id = $2 AND finished_at IS NULL
	`, errMsg, jobID)
	if err != nil {
		return err
	}

	// Requeue the job
	_, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status = 'queued',
		    run_at = $1,
		    claimed_by = NULL,
		    lease_expires_at = NULL,
		    attempt = attempt + 1,
		    updated_at = now()
		WHERE id = $2
	`, runAt, jobID)
	if err != nil {
		return err
	}

	// Add log entry
	_, err = tx.Exec(ctx, `
		INSERT INTO job_logs (job_id, level, message)
		VALUES ($1, 'warn', $2)
	`, jobID, "Execution failed. Requeuing job: "+errMsg)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
