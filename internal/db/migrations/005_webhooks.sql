-- 005_webhooks.sql
-- Webhook callback support for job completion/failure events

CREATE TABLE IF NOT EXISTS job_callbacks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    callback_url    TEXT NOT NULL,
    hmac_secret     TEXT NOT NULL DEFAULT gen_random_uuid()::text,
    event_type      TEXT NOT NULL DEFAULT 'all' CHECK (event_type IN ('all', 'completed', 'failed')),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
    attempts        INT NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_job_callbacks_pending ON job_callbacks (status, last_attempt_at)
    WHERE status = 'pending';
