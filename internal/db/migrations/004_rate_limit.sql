-- 004_rate_limit.sql
-- Token bucket rate limiting per queue

CREATE TABLE IF NOT EXISTS queue_rate_limits (
    queue_id        UUID PRIMARY KEY REFERENCES queues(id) ON DELETE CASCADE,
    tokens          DOUBLE PRECISION NOT NULL,
    max_tokens      DOUBLE PRECISION NOT NULL DEFAULT 10,
    refill_rate     DOUBLE PRECISION NOT NULL DEFAULT 10,
    last_refill_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE queues ADD COLUMN IF NOT EXISTS rate_limit_per_second INT NULL;
