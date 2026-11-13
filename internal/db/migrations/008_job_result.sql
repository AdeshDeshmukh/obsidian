-- 008_job_result.sql
-- Add structured output result storage to jobs and job_executions

ALTER TABLE jobs ADD COLUMN IF NOT EXISTS result JSONB;
ALTER TABLE job_executions ADD COLUMN IF NOT EXISTS result JSONB;
