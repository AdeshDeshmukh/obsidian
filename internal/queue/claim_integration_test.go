package queue

import (
	"context"
	"sync"
	"testing"
	"time"

	"obsidian/internal/testutil"

	"github.com/google/uuid"
)

func TestClaimBatchConcurrency(t *testing.T) {
	// Skip if running short tests and DB is not available
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestDB(t)
	ctx := context.Background()

	// Seed context dependencies
	var userID string
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash)
		VALUES ('test-claim-concurrency@obsidian.io', 'hash')
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	var orgID string
	err = pool.QueryRow(ctx, `
		INSERT INTO organizations (name, owner_id)
		VALUES ('Concurrency Org', $1)
		RETURNING id
	`, userID).Scan(&orgID)
	if err != nil {
		t.Fatalf("Failed to seed organization: %v", err)
	}

	var projectID string
	err = pool.QueryRow(ctx, `
		INSERT INTO projects (org_id, name, api_key)
		VALUES ($1, 'Concurrency Project', 'key-concurrency')
		RETURNING id
	`, orgID).Scan(&projectID)
	if err != nil {
		t.Fatalf("Failed to seed project: %v", err)
	}

	var queueID string
	err = pool.QueryRow(ctx, `
		INSERT INTO queues (project_id, name, priority, concurrency_limit)
		VALUES ($1, 'concurrency-queue', 1, 20)
		RETURNING id
	`, projectID).Scan(&queueID)
	if err != nil {
		t.Fatalf("Failed to seed queue: %v", err)
	}

	// Insert 15 test jobs
	numJobs := 15
	for i := 0; i < numJobs; i++ {
		_, err := pool.Exec(ctx, `
			INSERT INTO jobs (queue_id, job_type, payload, status, priority, run_at)
			VALUES ($1, 'noop', '{}', 'queued', 1, now() - interval '1 minute')
		`, queueID)
		if err != nil {
			t.Fatalf("Failed to seed jobs: %v", err)
		}
	}

	var dbCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE queue_id = $1 AND status = 'queued'`, queueID).Scan(&dbCount)
	if err != nil {
		t.Fatalf("Failed to count jobs in DB: %v", err)
	}
	t.Logf("Total jobs in DB before claiming: %d", dbCount)

	// Spawn 5 concurrent worker routines claiming 3 jobs each
	numWorkers := 5
	limit := 3

	var wg sync.WaitGroup
	var mu sync.Mutex
	claimedMap := make(map[string]int)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()
			workerID := uuid.New().String()
			
			// Register worker in DB first to satisfy foreign key constraints
			_, err := pool.Exec(ctx, `
				INSERT INTO workers (id, hostname, started_at, last_seen_at, status)
				VALUES ($1, 'test-host', now(), now(), 'active')
			`, workerID)
			if err != nil {
				t.Errorf("Failed to register worker %s: %v", workerID, err)
				return
			}

			// Claim batch of jobs
			claimed, err := ClaimBatch(ctx, pool, queueID, workerID, limit, 10*time.Second)
			if err != nil {
				t.Errorf("Worker %s encountered error: %v", workerID, err)
				return
			}
			t.Logf("Worker %s successfully claimed %d jobs", workerID, len(claimed))

			mu.Lock()
			for _, job := range claimed {
				claimedMap[job.ID]++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Assertions
	totalClaimed := len(claimedMap)
	expectedClaimed := numWorkers * limit // 5 workers * 3 limit = 15 total jobs
	if totalClaimed != expectedClaimed {
		t.Errorf("Expected total claimed jobs to be %d, got %d", expectedClaimed, totalClaimed)
	}

	for jobID, count := range claimedMap {
		if count > 1 {
			t.Errorf("Job %s was claimed %d times (expected exactly 1 claim)", jobID, count)
		}
	}
}

func TestReclaimExpiredLeases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestDB(t)
	ctx := context.Background()

	// Seed User, Org, Project, Queue
	var userID string
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash)
		VALUES ('test-reclaim@obsidian.io', 'hash')
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	var orgID string
	err = pool.QueryRow(ctx, `
		INSERT INTO organizations (name, owner_id)
		VALUES ('Reclaim Org', $1)
		RETURNING id
	`, userID).Scan(&orgID)
	if err != nil {
		t.Fatalf("Failed to seed organization: %v", err)
	}

	var projectID string
	err = pool.QueryRow(ctx, `
		INSERT INTO projects (org_id, name, api_key)
		VALUES ($1, 'Reclaim Project', 'key-reclaim')
		RETURNING id
	`, orgID).Scan(&projectID)
	if err != nil {
		t.Fatalf("Failed to seed project: %v", err)
	}

	var queueID string
	err = pool.QueryRow(ctx, `
		INSERT INTO queues (project_id, name, priority, concurrency_limit)
		VALUES ($1, 'reclaim-queue', 1, 5)
		RETURNING id
	`, projectID).Scan(&queueID)
	if err != nil {
		t.Fatalf("Failed to seed queue: %v", err)
	}

	// Seed a worker
	workerID := uuid.New().String()
	_, err = pool.Exec(ctx, `
		INSERT INTO workers (id, hostname, started_at, last_seen_at, status)
		VALUES ($1, 'reclaim-host', now(), now(), 'active')
	`, workerID)
	if err != nil {
		t.Fatalf("Failed to register worker: %v", err)
	}

	// Seed a job that is 'running' but has an expired lease (lease_expires_at is 5 minutes ago)
	var jobID string
	err = pool.QueryRow(ctx, `
		INSERT INTO jobs (queue_id, job_type, payload, status, priority, run_at, attempt, claimed_by, lease_expires_at)
		VALUES ($1, 'noop', '{}', 'running', 1, now() - interval '10 minutes', 0, $2, now() - interval '5 minutes')
		RETURNING id
	`, queueID, workerID).Scan(&jobID)
	if err != nil {
		t.Fatalf("Failed to seed running job: %v", err)
	}

	// Execute ReclaimExpired
	reclaimed, err := ReclaimExpired(ctx, pool)
	if err != nil {
		t.Fatalf("ReclaimExpired failed: %v", err)
	}
	if reclaimed != 1 {
		t.Errorf("Expected 1 job to be reclaimed, got %d", reclaimed)
	}

	// Assert job status was reverted to 'queued' and attempt incremented to 1
	var status string
	var attempt int
	var claimedBy *string
	var leaseExpiresAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT status, attempt, claimed_by, lease_expires_at
		FROM jobs
		WHERE id = $1
	`, jobID).Scan(&status, &attempt, &claimedBy, &leaseExpiresAt)
	if err != nil {
		t.Fatal(err)
	}

	if status != "queued" {
		t.Errorf("Expected status to be 'queued', got %s", status)
	}
	if attempt != 1 {
		t.Errorf("Expected attempt count to be 1, got %d", attempt)
	}
	if claimedBy != nil {
		t.Errorf("Expected claimed_by to be nil, got %s", *claimedBy)
	}
	if leaseExpiresAt != nil {
		t.Errorf("Expected lease_expires_at to be nil, got %v", *leaseExpiresAt)
	}
}
