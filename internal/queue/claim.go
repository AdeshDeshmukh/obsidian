package queue

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ClaimedJob struct {
	ID      string
	QueueID string
	JobType string
	Payload []byte
	Attempt int
}

// ClaimBatch atomically claims up to `limit` due jobs for this worker from the specified queue,
// ensuring the queue is not paused and all parent job dependencies are completed.
func ClaimBatch(ctx context.Context, pool *pgxpool.Pool, queueID, workerID string, limit int, leaseDuration time.Duration) ([]ClaimedJob, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Select due jobs, checking:
	// - queue is not paused
	// - status is queued/scheduled
	// - run_at is in the past/present
	// - all DAG dependencies (if any) are in 'completed' state
	var rows pgx.Rows
	if queueID != "" {
		rows, err = tx.Query(ctx, `
			SELECT j.id, j.queue_id, j.job_type, j.payload, j.attempt
			FROM jobs j
			JOIN queues q ON j.queue_id = q.id
			WHERE j.queue_id = $1
			  AND q.is_paused = false
			  AND j.status IN ('queued', 'scheduled')
			  AND j.run_at <= now()
			  AND (
			      SELECT count(*) FROM jobs rj 
			      WHERE rj.queue_id = j.queue_id AND rj.status IN ('running', 'claimed')
			  ) < q.concurrency_limit
			  AND NOT EXISTS (
			      SELECT 1 FROM job_dependencies jd
			      JOIN jobs dep ON dep.id = jd.depends_on_job_id
			      WHERE jd.job_id = j.id AND dep.status != 'completed'
			  )
			ORDER BY j.priority DESC, j.run_at ASC
			FOR UPDATE OF j SKIP LOCKED
			LIMIT $2
		`, queueID, limit)
	} else {
		rows, err = tx.Query(ctx, `
			SELECT j.id, j.queue_id, j.job_type, j.payload, j.attempt
			FROM jobs j
			JOIN queues q ON j.queue_id = q.id
			WHERE q.is_paused = false
			  AND j.status IN ('queued', 'scheduled')
			  AND j.run_at <= now()
			  AND (
			      SELECT count(*) FROM jobs rj 
			      WHERE rj.queue_id = j.queue_id AND rj.status IN ('running', 'claimed')
			  ) < q.concurrency_limit
			  AND NOT EXISTS (
			      SELECT 1 FROM job_dependencies jd
			      JOIN jobs dep ON dep.id = jd.depends_on_job_id
			      WHERE jd.job_id = j.id AND dep.status != 'completed'
			  )
			ORDER BY j.priority DESC, j.run_at ASC
			FOR UPDATE OF j SKIP LOCKED
			LIMIT $1
		`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []ClaimedJob
	var ids []uuid.UUID
	for rows.Next() {
		var j ClaimedJob
		if err := rows.Scan(&j.ID, &j.QueueID, &j.JobType, &j.Payload, &j.Attempt); err != nil {
			return nil, err
		}
		claimed = append(claimed, j)
		ids = append(ids, uuid.MustParse(j.ID))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return nil, tx.Commit(ctx)
	}

	leaseExpiry := time.Now().Add(leaseDuration)
	_, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status = 'running', 
		    claimed_by = $1, 
		    claimed_at = now(),
		    lease_expires_at = $2, 
		    updated_at = now()
		WHERE id = ANY($3)
	`, workerID, leaseExpiry, ids)
	if err != nil {
		return nil, err
	}

	// Insert into job_executions log for audit
	batch := &pgx.Batch{}
	for _, job := range claimed {
		batch.Queue(`
			INSERT INTO job_executions (job_id, worker_id, attempt, started_at)
			VALUES ($1, $2, $3, now())
		`, job.ID, workerID, job.Attempt)
	}
	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		return nil, err
	}

	return claimed, tx.Commit(ctx)
}

// ReclaimExpired scans for running jobs that have exceeded their heartbeat lease,
// returns them to 'queued' for retry, and increments their attempts.
func ReclaimExpired(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	dlqRows, err := tx.Query(ctx, `
		SELECT j.id, j.queue_id, j.payload, j.attempt, 
		       COALESCE(rp.max_attempts, q_rp.max_attempts, 5) as max_attempts
		FROM jobs j
		LEFT JOIN retry_policies rp ON j.retry_policy_id = rp.id
		LEFT JOIN queues q ON j.queue_id = q.id
		LEFT JOIN retry_policies q_rp ON q.default_retry_policy_id = q_rp.id
		WHERE j.status IN ('running', 'claimed') AND j.lease_expires_at < now()
	`)
	if err != nil {
		return 0, err
	}

	type expiredJob struct {
		id          string
		queueID     string
		payload     []byte
		attempt     int
		maxAttempts int
	}
	var expiredList []expiredJob
	for dlqRows.Next() {
		var ej expiredJob
		if err := dlqRows.Scan(&ej.id, &ej.queueID, &ej.payload, &ej.attempt, &ej.maxAttempts); err == nil {
			expiredList = append(expiredList, ej)
		}
	}
	dlqRows.Close()

	var reclaimedCount int64
	for _, ej := range expiredList {
		if ej.attempt+1 >= ej.maxAttempts {
			// Move to DLQ
			_, _ = tx.Exec(ctx, `
				UPDATE jobs SET status = 'dead_letter', updated_at = now() WHERE id = $1
			`, ej.id)
			_, _ = tx.Exec(ctx, `
				INSERT INTO dead_letter_queue (original_job_id, queue_id, payload, failure_reason, attempts_made, moved_at)
				VALUES ($1, $2, $3, 'Worker lease expired and max attempts reached', $4, now())
			`, ej.id, ej.queueID, ej.payload, ej.attempt+1)
			_, _ = tx.Exec(ctx, `
				INSERT INTO job_logs (job_id, level, message)
				VALUES ($1, 'error', 'Lease expired and max attempts reached. Moved to DLQ')
			`, ej.id)
		} else {
			// Requeue
			tag, _ := tx.Exec(ctx, `
				UPDATE jobs
				SET status = 'queued',
				    claimed_by = NULL,
				    lease_expires_at = NULL,
				    attempt = attempt + 1,
				    updated_at = now()
				WHERE id = $1
			`, ej.id)
			reclaimedCount += tag.RowsAffected()
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return reclaimedCount, nil
}
