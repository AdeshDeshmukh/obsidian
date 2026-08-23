package worker

import (
	"context"
	"testing"
	"time"

	"obsidian/internal/testutil"

	"github.com/google/uuid"
)

func TestWorkerPoolGracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dbPool := testutil.SetupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Seed User, Org, Project, Queue
	var userID string
	err := dbPool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash)
		VALUES ('test-pool@obsidian.io', 'hash')
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	var orgID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO organizations (name, owner_id)
		VALUES ('Test Pool Org', $1)
		RETURNING id
	`, userID).Scan(&orgID)
	if err != nil {
		t.Fatalf("Failed to seed organization: %v", err)
	}

	var projectID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO projects (org_id, name, api_key)
		VALUES ($1, 'Test Pool Project', 'key-pool')
		RETURNING id
	`, orgID).Scan(&projectID)
	if err != nil {
		t.Fatalf("Failed to seed project: %v", err)
	}

	var queueID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO queues (project_id, name, priority, concurrency_limit)
		VALUES ($1, 'pool-queue', 1, 5)
		RETURNING id
	`, projectID).Scan(&queueID)
	if err != nil {
		t.Fatalf("Failed to seed queue: %v", err)
	}

	// Seed a sleep job (payload specifies 1 second sleep)
	var jobID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO jobs (queue_id, job_type, payload, status, priority, run_at)
		VALUES ($1, 'sleep', '{"seconds":1}', 'queued', 1, now() - interval '1 minute')
		RETURNING id
	`, queueID).Scan(&jobID)
	if err != nil {
		t.Fatalf("Failed to seed job: %v", err)
	}

	workerID := uuid.New().String()

	// Initialize Worker Pool
	wPool := New(dbPool, workerID, queueID, 2)

	// Run worker pool in a separate goroutine
	poolDone := make(chan struct{})
	go func() {
		wPool.Run(ctx)
		close(poolDone)
	}()

	// Wait for the worker pool to start and claim the job (it runs on a 500ms ticker)
	time.Sleep(800 * time.Millisecond)

	// Verify the job is in 'running' state
	var status string
	err = dbPool.QueryRow(context.Background(), "SELECT status FROM jobs WHERE id = $1", jobID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Errorf("Expected job status to be 'running' before shutdown, got %s", status)
	}

	// Trigger Graceful Shutdown (cancel context)
	t.Log("Triggering context cancellation (graceful shutdown)...")
	cancel()

	// Wait for worker pool to terminate
	select {
	case <-poolDone:
		t.Log("Worker pool exited gracefully.")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout: Worker pool failed to shutdown within 3 seconds")
	}

	// Verify the job completed successfully (was not aborted mid-run)
	err = dbPool.QueryRow(context.Background(), "SELECT status FROM jobs WHERE id = $1", jobID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Errorf("Expected job status to be 'completed' after graceful shutdown, got %s", status)
	}
}
