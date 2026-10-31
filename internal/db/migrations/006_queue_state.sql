-- 006_queue_state.sql
-- Migrate is_paused boolean to a proper state machine enum

DO $$ BEGIN
    CREATE TYPE queue_state AS ENUM ('active', 'paused', 'draining', 'maintenance');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

ALTER TABLE queues ADD COLUMN IF NOT EXISTS state queue_state NOT NULL DEFAULT 'active';

UPDATE queues SET state = 'paused' WHERE is_paused = true AND state = 'active';
UPDATE queues SET state = 'active' WHERE is_paused = false;

CREATE OR REPLACE FUNCTION sync_queue_is_paused() RETURNS TRIGGER AS $$
BEGIN
    NEW.is_paused = (NEW.state != 'active');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_queue_is_paused ON queues;
CREATE TRIGGER trg_sync_queue_is_paused
BEFORE INSERT OR UPDATE OF state ON queues
FOR EACH ROW EXECUTE FUNCTION sync_queue_is_paused();
