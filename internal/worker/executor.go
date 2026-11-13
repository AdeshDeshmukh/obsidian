package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"obsidian/internal/api/ws"
	"obsidian/internal/metrics"
	"obsidian/internal/queue"
	"obsidian/internal/telemetry"
	"obsidian/internal/webhook"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
)

type HandlerFunc func(ctx context.Context, payload []byte) error
type ResultHandlerFunc func(ctx context.Context, payload []byte) (interface{}, error)

var (
	handlersMu sync.RWMutex
	handlers   = make(map[string]ResultHandlerFunc)

	runningJobsMu sync.Mutex
	runningJobs   = make(map[string]context.CancelFunc)
)

func RegisterRunningJob(jobID string, cancel context.CancelFunc) {
	runningJobsMu.Lock()
	defer runningJobsMu.Unlock()
	runningJobs[jobID] = cancel
}

func UnregisterRunningJob(jobID string) {
	runningJobsMu.Lock()
	defer runningJobsMu.Unlock()
	delete(runningJobs, jobID)
}

func CancelRunningJob(jobID string) bool {
	runningJobsMu.Lock()
	defer runningJobsMu.Unlock()
	if cancel, ok := runningJobs[jobID]; ok {
		cancel()
		return true
	}
	return false
}

func RegisterHandler(jobType string, fn HandlerFunc) {
	RegisterResultHandler(jobType, func(ctx context.Context, payload []byte) (interface{}, error) {
		return nil, fn(ctx, payload)
	})
}

func RegisterResultHandler(jobType string, fn ResultHandlerFunc) {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	handlers[jobType] = fn
}

func init() {
	// Register default test handlers
	RegisterHandler("noop", func(ctx context.Context, payload []byte) error {
		return nil
	})

	RegisterHandler("log", func(ctx context.Context, payload []byte) error {
		var p struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("invalid payload: %w", err)
		}
		slog.Info("Job log handler executed", "message", p.Message)
		return nil
	})

	RegisterHandler("sleep_60s", func(ctx context.Context, payload []byte) error {
		select {
		case <-time.After(60 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	RegisterHandler("sleep", func(ctx context.Context, payload []byte) error {
		var p struct {
			Seconds int `json:"seconds"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("invalid payload: %w", err)
		}
		duration := time.Duration(p.Seconds) * time.Second
		select {
		case <-time.After(duration):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	RegisterHandler("fail", func(ctx context.Context, payload []byte) error {
		return fmt.Errorf("explicit failure simulated")
	})
}

// Execute runs the job handler, handles heartbeats, updates metrics and handles retries or DLQ.
func Execute(ctx context.Context, pool *pgxpool.Pool, job queue.ClaimedJob, workerID string) {
	ctx, span := telemetry.Tracer.Start(ctx, "ExecuteJob")
	defer span.End()
	span.SetAttributes(
		attribute.String("job_id", job.ID),
		attribute.String("job_type", job.JobType),
		attribute.Int("attempt", job.Attempt),
	)

	slog.Info("Running job",
		"job_id", job.ID,
		"job_type", job.JobType,
		"queue_id", job.QueueID,
		"attempt", job.Attempt,
		"worker_id", workerID,
	)

	// Start lease renewal heartbeat
	stopHeartbeat := StartHeartbeat(ctx, pool, job.ID, 10*time.Second, 30*time.Second)
	defer stopHeartbeat()

	start := time.Now()

	jobCtx, jobCancel := context.WithCancel(ctx)
	RegisterRunningJob(job.ID, jobCancel)
	defer UnregisterRunningJob(job.ID)
	defer jobCancel()

	reporter := NewProgressReporter(jobCtx, pool, job.ID, job.QueueID)
	jobCtx = WithProgressReporter(jobCtx, reporter)

	// Dispatch handler
	res, err := runHandler(jobCtx, job)
	duration := time.Since(start)

	if err == nil {
		slog.Info("Job completed successfully",
			"job_id", job.ID,
			"queue_id", job.QueueID,
			"attempt", job.Attempt,
			"worker_id", workerID,
			"duration_ms", duration.Milliseconds(),
		)
		if err := markCompleted(ctx, pool, job, workerID, duration, res); err != nil {
			slog.Error("Failed marking job completed",
				"job_id", job.ID,
				"queue_id", job.QueueID,
				"worker_id", workerID,
				"error", err,
			)
		}
		ws.Broadcast("job.updated", map[string]interface{}{"id": job.ID, "queue_id": job.QueueID, "status": "completed", "result": res})
		go webhook.Dispatch(context.Background(), pool, job.ID, job.QueueID, "completed", "")
		return
	}

	if errors.Is(err, context.Canceled) || jobCtx.Err() == context.Canceled {
		slog.Warn("Job execution was cancelled",
			"job_id", job.ID,
			"queue_id", job.QueueID,
			"worker_id", workerID,
			"duration_ms", duration.Milliseconds(),
		)
		if err := markCancelled(ctx, pool, job, workerID, duration); err != nil {
			slog.Error("Failed marking job cancelled",
				"job_id", job.ID,
				"error", err,
			)
		}
		ws.Broadcast("job.updated", map[string]interface{}{"id": job.ID, "queue_id": job.QueueID, "status": "cancelled"})
		go webhook.Dispatch(context.Background(), pool, job.ID, job.QueueID, "cancelled", "Job execution cancelled by operator")
		return
	}

	slog.Error("Job execution failed",
		"job_id", job.ID,
		"queue_id", job.QueueID,
		"attempt", job.Attempt,
		"worker_id", workerID,
		"duration_ms", duration.Milliseconds(),
		"error", err,
	)
	policy := loadRetryPolicy(ctx, pool, job.ID)
	
	// Check if attempts exhausted
	if job.Attempt+1 >= policy.MaxAttempts {
		slog.Warn("Job attempts exhausted, moving to DLQ",
			"job_id", job.ID,
			"queue_id", job.QueueID,
			"attempts", job.Attempt+1,
			"max_attempts", policy.MaxAttempts,
			"worker_id", workerID,
		)
		if err := queue.MoveToDeadLetter(ctx, pool, job, err.Error()); err != nil {
			slog.Error("Failed moving job to DLQ",
				"job_id", job.ID,
				"queue_id", job.QueueID,
				"worker_id", workerID,
				"error", err,
			)
		}
		metrics.JobsFailed.WithLabelValues(job.QueueID).Inc()
		ws.Broadcast("job.updated", map[string]interface{}{"id": job.ID, "queue_id": job.QueueID, "status": "dead_letter"})
		go webhook.Dispatch(context.Background(), pool, job.ID, job.QueueID, "failed", err.Error())
		return
	}

	// Requeue with backoff delay
	delay := queue.ComputeBackoff(policy, job.Attempt)
	slog.Info("Requeuing job with backoff delay",
		"job_id", job.ID,
		"queue_id", job.QueueID,
		"attempt", job.Attempt,
		"delay_ms", delay.Milliseconds(),
		"worker_id", workerID,
	)
	if err := queue.RequeueWithDelay(ctx, pool, job.ID, delay, err.Error()); err != nil {
		slog.Error("Failed requeuing job",
			"job_id", job.ID,
			"queue_id", job.QueueID,
			"worker_id", workerID,
			"error", err,
		)
	}
	ws.Broadcast("job.updated", map[string]interface{}{"id": job.ID, "queue_id": job.QueueID, "status": "queued"})
}

func runHandler(ctx context.Context, job queue.ClaimedJob) (interface{}, error) {
	handlersMu.RLock()
	handler, ok := handlers[job.JobType]
	handlersMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no handler registered for job type: %s", job.JobType)
	}

	return handler(ctx, job.Payload)
}

func markCompleted(ctx context.Context, pool *pgxpool.Pool, job queue.ClaimedJob, workerID string, duration time.Duration, result interface{}) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var resultJSON []byte
	if result != nil {
		resultJSON, _ = json.Marshal(result)
	}

	_, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status = 'completed', result = $2, updated_at = now()
		WHERE id = $1
	`, job.ID, resultJSON)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE job_executions
		SET finished_at = now(), outcome = 'success',
		    progress = 100,
		    result = $1,
		    duration_ms = $2
		WHERE job_id = $3 AND finished_at IS NULL
	`, resultJSON, duration.Milliseconds(), job.ID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO job_logs (job_id, level, message)
		VALUES ($1, 'info', 'Job completed successfully')
	`, job.ID)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	metrics.JobsCompleted.WithLabelValues(job.QueueID).Inc()
	return nil
}

func markCancelled(ctx context.Context, pool *pgxpool.Pool, job queue.ClaimedJob, workerID string, duration time.Duration) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, _ = tx.Exec(ctx, `
		UPDATE jobs
		SET status = 'cancelled', updated_at = now()
		WHERE id = $1
	`, job.ID)

	_, _ = tx.Exec(ctx, `
		UPDATE job_executions
		SET finished_at = now(), outcome = 'failure',
		    error_message = 'Job execution cancelled by operator',
		    duration_ms = $1
		WHERE job_id = $2 AND finished_at IS NULL
	`, duration.Milliseconds(), job.ID)

	_, _ = tx.Exec(ctx, `
		INSERT INTO job_logs (job_id, level, message)
		VALUES ($1, 'warn', 'Job execution cancelled by operator')
	`, job.ID)

	return tx.Commit(ctx)
}

func loadRetryPolicy(ctx context.Context, pool *pgxpool.Pool, jobID string) queue.RetryPolicy {
	var p queue.RetryPolicy
	// Default fallback
	p.Strategy = "fixed"
	p.BaseDelayMs = 1000
	p.MaxDelayMs = 30000
	p.MaxAttempts = 3

	// Resolve direct job policy
	err := pool.QueryRow(ctx, `
		SELECT rp.strategy, rp.base_delay_ms, rp.max_delay_ms, rp.max_attempts
		FROM jobs j
		JOIN retry_policies rp ON j.retry_policy_id = rp.id
		WHERE j.id = $1
	`, jobID).Scan(&p.Strategy, &p.BaseDelayMs, &p.MaxDelayMs, &p.MaxAttempts)

	if err != nil {
		// Try queue default policy
		_ = pool.QueryRow(ctx, `
			SELECT rp.strategy, rp.base_delay_ms, rp.max_delay_ms, rp.max_attempts
			FROM jobs j
			JOIN queues q ON j.queue_id = q.id
			JOIN retry_policies rp ON q.default_retry_policy_id = rp.id
			WHERE j.id = $1
		`, jobID).Scan(&p.Strategy, &p.BaseDelayMs, &p.MaxDelayMs, &p.MaxAttempts)
	}

	return p
}
