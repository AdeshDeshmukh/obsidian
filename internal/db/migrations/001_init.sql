-- 001_init.sql

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'member', 'viewer')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE organizations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    owner_id   UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name    TEXT NOT NULL,
    api_key TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE retry_policies (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    strategy      TEXT NOT NULL CHECK (strategy IN ('fixed','linear','exponential')),
    base_delay_ms INT NOT NULL DEFAULT 1000,
    max_delay_ms  INT NOT NULL DEFAULT 300000,
    max_attempts  INT NOT NULL DEFAULT 5
);

CREATE TABLE queues (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id              UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name                    TEXT NOT NULL,
    priority                SMALLINT NOT NULL DEFAULT 0,
    concurrency_limit       INT NOT NULL DEFAULT 10,
    is_paused               BOOLEAN NOT NULL DEFAULT false,
    default_retry_policy_id UUID REFERENCES retry_policies(id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE TYPE job_status AS ENUM
    ('queued','scheduled','claimed','running','completed','failed','dead_letter','cancelled');

CREATE TABLE jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    queue_id         UUID NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
    idempotency_key  TEXT,
    job_type         TEXT NOT NULL,
    payload          JSONB NOT NULL DEFAULT '{}',
    status           job_status NOT NULL DEFAULT 'queued',
    priority         SMALLINT NOT NULL DEFAULT 0,
    run_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempt          INT NOT NULL DEFAULT 0,
    retry_policy_id  UUID REFERENCES retry_policies(id) ON DELETE SET NULL,
    claimed_by       UUID,
    claimed_at       TIMESTAMPTZ,
    lease_expires_at TIMESTAMPTZ,
    batch_id         UUID,
    parent_job_id    UUID REFERENCES jobs(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (queue_id, idempotency_key)
);

CREATE INDEX idx_jobs_claim_scan
    ON jobs (queue_id, status, priority DESC, run_at)
    WHERE status IN ('queued','scheduled');

CREATE INDEX idx_jobs_lease_expiry
    ON jobs (lease_expires_at)
    WHERE status = 'running';

CREATE TABLE job_dependencies (
    job_id            UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    depends_on_job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    PRIMARY KEY (job_id, depends_on_job_id)
);

CREATE TABLE scheduled_jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    queue_id         UUID NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
    cron_expr        TEXT NOT NULL,
    job_type         TEXT NOT NULL,
    payload_template JSONB NOT NULL DEFAULT '{}',
    next_run_at      TIMESTAMPTZ NOT NULL,
    is_active        BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE workers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname     TEXT NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','draining','dead'))
);

CREATE TABLE worker_heartbeats (
    id          BIGSERIAL PRIMARY KEY,
    worker_id   UUID NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
    job_id      UUID REFERENCES jobs(id) ON DELETE SET NULL,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE job_executions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id        UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    worker_id     UUID REFERENCES workers(id) ON DELETE SET NULL,
    attempt       INT NOT NULL,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ,
    outcome       TEXT CHECK (outcome IN ('success','failure','timeout')),
    error_message TEXT,
    duration_ms   INT
);

CREATE TABLE job_logs (
    id        BIGSERIAL PRIMARY KEY,
    job_id    UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    level     TEXT NOT NULL DEFAULT 'info',
    message   TEXT NOT NULL,
    logged_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dead_letter_queue (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    queue_id       UUID NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
    payload        JSONB NOT NULL,
    failure_reason TEXT,
    attempts_made  INT NOT NULL,
    moved_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Event-driven LISTEN/NOTIFY trigger for instant job dispatching
CREATE OR REPLACE FUNCTION notify_job_queued() RETURNS TRIGGER AS $$
BEGIN
  PERFORM pg_notify('job_queued', NEW.queue_id::text);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_notify_job_queued
AFTER INSERT ON jobs
FOR EACH ROW
WHEN (NEW.status = 'queued')
EXECUTE FUNCTION notify_job_queued();

-- Auto update timestamp trigger
CREATE OR REPLACE FUNCTION update_updated_at_column() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_update_jobs_updated_at
BEFORE UPDATE ON jobs
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

