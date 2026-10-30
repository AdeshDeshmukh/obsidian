package queue

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func ConsumeToken(ctx context.Context, db DBTX, queueID string) (bool, error) {
	var tokens, maxTokens, refillRate float64
	var lastRefillAt time.Time

	err := db.QueryRow(ctx, `
		SELECT tokens, max_tokens, refill_rate, last_refill_at
		FROM queue_rate_limits
		WHERE queue_id = $1
		FOR UPDATE
	`, queueID).Scan(&tokens, &maxTokens, &refillRate, &lastRefillAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, err
	}

	elapsed := time.Since(lastRefillAt).Seconds()
	tokens = math.Min(maxTokens, tokens+elapsed*refillRate)

	if tokens < 1.0 {
		_, _ = db.Exec(ctx, `
			UPDATE queue_rate_limits
			SET tokens = $1, last_refill_at = now()
			WHERE queue_id = $2
		`, tokens, queueID)
		return false, nil
	}

	_, err = db.Exec(ctx, `
		UPDATE queue_rate_limits
		SET tokens = $1, last_refill_at = now()
		WHERE queue_id = $2
	`, tokens-1.0, queueID)
	if err != nil {
		return false, err
	}

	return true, nil
}

func SetQueueRateLimit(ctx context.Context, db DBTX, queueID string, ratePerSec float64) error {
	_, err := db.Exec(ctx, `
		INSERT INTO queue_rate_limits (queue_id, tokens, max_tokens, refill_rate, last_refill_at)
		VALUES ($1, $2, $2, $2, now())
		ON CONFLICT (queue_id) DO UPDATE
		SET max_tokens = $2, refill_rate = $2, last_refill_at = now()
	`, queueID, ratePerSec)
	if err != nil {
		return err
	}

	_, err = db.Exec(ctx, `
		UPDATE queues
		SET rate_limit_per_second = $1
		WHERE id = $2
	`, int(ratePerSec), queueID)
	return err
}
