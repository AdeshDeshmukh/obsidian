package db

import (
	"context"
	_ "embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_init.sql
var initSQL string

// AutoMigrate runs the startup schema initialization if the tables are not already present.
func AutoMigrate(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("Verifying database schema...")

	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_tables 
			WHERE schemaname = 'public' 
			AND tablename = 'jobs'
		)
	`).Scan(&exists)
	if err != nil {
		return err
	}

	if exists {
		slog.Info("Schema is already initialized. Ensuring incremental updates...")
		_, _ = pool.Exec(ctx, `
			ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'admin';
			ALTER TABLE queues ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;
			ALTER TABLE queues ADD COLUMN IF NOT EXISTS rate_limit_per_second INT NULL;
			CREATE INDEX IF NOT EXISTS idx_queues_not_deleted ON queues (project_id) WHERE deleted_at IS NULL;
			ALTER TABLE retry_policies DROP CONSTRAINT IF EXISTS retry_policies_strategy_check;
			ALTER TABLE retry_policies ADD CONSTRAINT retry_policies_strategy_check CHECK (strategy IN ('fixed', 'linear', 'exponential', 'exponential_jitter'));
			CREATE TABLE IF NOT EXISTS queue_rate_limits (
				queue_id        UUID PRIMARY KEY REFERENCES queues(id) ON DELETE CASCADE,
				tokens          DOUBLE PRECISION NOT NULL,
				max_tokens      DOUBLE PRECISION NOT NULL DEFAULT 10,
				refill_rate     DOUBLE PRECISION NOT NULL DEFAULT 10,
				last_refill_at  TIMESTAMPTZ NOT NULL DEFAULT now()
			);
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
		`)
		return nil
	}

	slog.Info("Schema tables not found. Applying migrations/001_init.sql...")
	_, err = pool.Exec(ctx, initSQL)
	if err != nil {
		return err
	}

	slog.Info("Database schema initialized successfully.")
	return nil
}
