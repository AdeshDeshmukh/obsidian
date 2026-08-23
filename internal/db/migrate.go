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
