-- 002_soft_delete.sql
-- Add soft delete support to queues to preserve audit history and prevent orphaned jobs.

ALTER TABLE queues ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

-- Partial index for active (non-deleted) queue queries
CREATE INDEX IF NOT EXISTS idx_queues_not_deleted ON queues (project_id) WHERE deleted_at IS NULL;
