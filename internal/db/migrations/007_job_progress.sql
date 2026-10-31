-- 007_job_progress.sql
-- Add progress tracking to job executions

ALTER TABLE job_executions ADD COLUMN IF NOT EXISTS progress INT NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100);

CREATE INDEX IF NOT EXISTS idx_job_executions_active ON job_executions (job_id) WHERE finished_at IS NULL;

