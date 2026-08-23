package worker

import (
	"context"
	"log/slog"
	"os"
	"time"

	"obsidian/internal/api/ws"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StartHeartbeat renews the job's lease periodically while it is running.
// If the worker crashes or freezes, heartbeats stop, the lease expires,
// and the lease cleanup task will reclaim it.
func StartHeartbeat(ctx context.Context, pool *pgxpool.Pool, jobID string, interval, leaseDuration time.Duration) func() {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				_, err := pool.Exec(ctx, `
					UPDATE jobs 
					SET lease_expires_at = $1, updated_at = now() 
					WHERE id = $2 AND status = 'running'
				`, time.Now().Add(leaseDuration), jobID)
				if err != nil {
					slog.Error("Failed updating job heartbeat lease", "job_id", jobID, "error", err)
				}
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	return func() { close(done) }
}

// RegisterWorker inserts a new worker record into the database
func RegisterWorker(ctx context.Context, pool *pgxpool.Pool, workerID, status string) error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO workers (id, hostname, started_at, last_seen_at, status)
		VALUES ($1, $2, now(), now(), $3)
		ON CONFLICT (id) DO UPDATE 
		SET last_seen_at = now(), status = $3
	`, workerID, hostname, status)
	return err
}

// StartWorkerHeartbeat starts a loop to periodically update the worker's own status
func StartWorkerHeartbeat(ctx context.Context, pool *pgxpool.Pool, workerID string, interval time.Duration) func() {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				_, err := pool.Exec(ctx, `
					UPDATE workers
					SET last_seen_at = now()
					WHERE id = $1
				`, workerID)
				if err != nil {
					slog.Error("Failed updating worker heartbeat status", "worker_id", workerID, "error", err)
				}
				_, _ = pool.Exec(ctx, `
					INSERT INTO worker_heartbeats (worker_id, reported_at)
					VALUES ($1, now())
				`, workerID)
				ws.Broadcast("worker.heartbeat", map[string]string{"id": workerID})
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	return func() { close(done) }
}
