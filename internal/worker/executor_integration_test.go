package worker

import (
	"context"
	"testing"
	"time"

	"obsidian/internal/queue"
	"obsidian/internal/testutil"

	"github.com/google/uuid"
)

func TestExecutorRetryAndDLQ(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestDB(t)
	ctx := context.Background()

	// Seed User, Org, Project, Queue
	var userID string
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash)
		VALUES ('test-executor@obsidian.io', 'hash')
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	var orgID string
	err = pool.QueryRow(ctx, `
		INSERT INTO organizations (name, owner_id)
		VALUES ('Test Executor Org', $1)
		RETURNING id
	`, userID).Scan(&orgID)
	if err != nil {
		t.Fatalf("Failed to seed organization: %v", err)
	}

	var projectID string
	err = pool.QueryRow(ctx, `
		INSERT INTO projects (org_id, name, api_key)
		VALUES ($1, 'Test Executor Project', 'key-executor')
		RETURNING id
	`, orgID).Scan(&projectID)
	if err != nil {
		t.Fatalf("Failed to seed project: %v", err)
	}

	var queueID string
	err = pool.QueryRow(ctx, `
		INSERT INTO queues (project_id, name, priority, concurrency_limit)
		VALUES ($1, 'executor-queue', 1, 5)
		RETURNING id
	`, projectID).Scan(&queueID)
	if err != nil {
		t.Fatalf("Failed to seed queue: %v", err)
	}

	workerID := uuid.New().String()
	_, err = pool.Exec(ctx, `
		INSERT INTO workers (id, hostname, started_at, last_seen_at, status)
		VALUES ($1, 'executor-host', now(), now(), 'active')
	`, workerID)
	if err != nil {
		t.Fatalf("Failed to register worker: %v", err)
	}

	// 1. Seed a job using the 'fail' handler
	var jobID string
	err = pool.QueryRow(ctx, `
		INSERT INTO jobs (queue_id, job_type, payload, status, priority, run_at, attempt)
		VALUES ($1, 'fail', '{}', 'queued', 1, now(), 0)
		RETURNING id
	`, queueID).Scan(&jobID)
	if err != nil {
		t.Fatalf("Failed to seed job: %v", err)
	}

	// Helper to claim job
	claimJob := func() queue.ClaimedJob {
		claimed, err := queue.ClaimBatch(ctx, pool, queueID, workerID, 1, 10*time.Second)
		if err != nil {
			t.Fatalf("Claim failed: %v", err)
		}
		if len(claimed) == 0 {
			t.Fatalf("No jobs claimed")
		}
		return claimed[0]
	}

	// Execution 1 (attempt 0 -> increments to 1)
	job := claimJob()
	Execute(ctx, pool, job, workerID)
	_, _ = pool.Exec(ctx, "UPDATE jobs SET run_at = now() - interval '1 minute' WHERE id = $1", jobID)

	var attempt int
	var status string
	err = pool.QueryRow(ctx, "SELECT attempt, status FROM jobs WHERE id = $1", jobID).Scan(&attempt, &status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Errorf("Expected status to revert to queued, got %s", status)
	}
	if attempt != 1 {
		t.Errorf("Expected attempt count to be 1, got %d", attempt)
	}

	// Execution 2 (attempt 1 -> increments to 2)
	job = claimJob()
	Execute(ctx, pool, job, workerID)
	_, _ = pool.Exec(ctx, "UPDATE jobs SET run_at = now() - interval '1 minute' WHERE id = $1", jobID)

	err = pool.QueryRow(ctx, "SELECT attempt, status FROM jobs WHERE id = $1", jobID).Scan(&attempt, &status)
	if err != nil {
		t.Fatal(err)
	}
	if attempt != 2 {
		t.Errorf("Expected attempt count to be 2, got %d", attempt)
	}

	// Execution 3 (attempt 2 -> reaches limit 3 -> moves to DLQ)
	job = claimJob()
	Execute(ctx, pool, job, workerID)

	err = pool.QueryRow(ctx, "SELECT attempt, status FROM jobs WHERE id = $1", jobID).Scan(&attempt, &status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "dead_letter" {
		t.Errorf("Expected status to be dead_letter, got %s", status)
	}

	// Assert DLQ entry exists
	var dlqCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM dead_letter_queue WHERE original_job_id = $1", jobID).Scan(&dlqCount)
	if err != nil {
		t.Fatal(err)
	}
	if dlqCount != 1 {
		t.Errorf("Expected 1 dead letter queue record, got %d", dlqCount)
	}
}
