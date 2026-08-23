package scheduler

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WithQueueLock ensures only one scheduler replica dispatches due cron jobs
// for a given queue at a time, so recurring jobs never double-fire even
// when the scheduler itself runs as multiple replicas.
// It acquires a dedicated connection from the pool to keep the session-level advisory lock consistent.
func WithQueueLock(ctx context.Context, pool *pgxpool.Pool, queueID string, fn func() error) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection for lock: %w", err)
	}
	defer conn.Release()

	key := hashKey(queueID)
	var acquired bool
	err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&acquired)
	if err != nil {
		return fmt.Errorf("error calling pg_try_advisory_lock: %w", err)
	}
	if !acquired {
		// Lock was not acquired (held by another scheduler process)
		return nil
	}

	defer func() {
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, key)
	}()

	return fn()
}

func hashKey(s string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return int64(h.Sum64())
}
