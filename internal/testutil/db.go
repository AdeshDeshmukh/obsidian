package testutil

import (
	"context"
	"os"
	"testing"
	"time"

	"obsidian/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SetupTestDB connects to the Postgres database, truncates all tables for a clean slate,
// and registers a cleanup handler to truncate tables again after the test completes.
func SetupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:obsidian@localhost:5432/obsidian?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Ensure database schema is migrated and up-to-date
	if err := db.AutoMigrate(ctx, pool); err != nil {
		t.Fatalf("Failed to auto migrate test database: %v", err)
	}

	// Truncate tables for a fresh state
	cleanTables(t, pool);

	// Register cleanup teardown
	t.Cleanup(func() {
		cleanTables(t, pool)
		pool.Close()
	})

	return pool
}

func cleanTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		TRUNCATE TABLE 
			users, 
			organizations, 
			projects, 
			queues, 
			retry_policies, 
			jobs, 
			job_dependencies, 
			scheduled_jobs, 
			workers, 
			worker_heartbeats, 
			job_executions, 
			job_logs, 
			dead_letter_queue 
		CASCADE;
	`)
	if err != nil {
		t.Fatalf("Failed to truncate database tables: %v", err)
	}
}
