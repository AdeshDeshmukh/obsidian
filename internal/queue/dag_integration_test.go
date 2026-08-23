package queue_test

import (
	"context"
	"testing"
	"time"

	"obsidian/internal/queue"
	"obsidian/internal/testutil"

	"github.com/google/uuid"
)

// TestDAGDependencyBlocking verifies that a child job with an unsatisfied
// dependency is NOT claimed by workers, and only becomes claimable once
// its parent job is marked as 'completed'.
func TestDAGDependencyBlocking(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	ctx := context.Background()

	// Create org, project, queue
	var orgID, projID, queueID string
	err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ('dag@test.io', 'hash') RETURNING id`).Scan(&orgID)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	err = pool.QueryRow(ctx, `INSERT INTO organizations (name, owner_id) VALUES ('Dag Org', $1) RETURNING id`, orgID).Scan(&orgID)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	err = pool.QueryRow(ctx, `INSERT INTO projects (org_id, name, api_key) VALUES ($1, 'Dag Project', $2) RETURNING id`,
		orgID, "dagkey-"+uuid.New().String()[:8]).Scan(&projID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	err = pool.QueryRow(ctx, `INSERT INTO queues (project_id, name, priority, concurrency_limit) VALUES ($1, 'dag-queue', 1, 10) RETURNING id`,
		projID).Scan(&queueID)
	if err != nil {
		t.Fatalf("create queue: %v", err)
	}

	// Insert parent job (queued, ready to run)
	var parentJobID string
	err = pool.QueryRow(ctx, `
		INSERT INTO jobs (queue_id, job_type, payload, status, priority, run_at)
		VALUES ($1, 'noop', '{}', 'queued', 5, now())
		RETURNING id
	`, queueID).Scan(&parentJobID)
	if err != nil {
		t.Fatalf("insert parent job: %v", err)
	}

	// Insert child job (queued, depends on parent)
	var childJobID string
	err = pool.QueryRow(ctx, `
		INSERT INTO jobs (queue_id, job_type, payload, status, priority, run_at)
		VALUES ($1, 'noop', '{}', 'queued', 5, now())
		RETURNING id
	`, queueID).Scan(&childJobID)
	if err != nil {
		t.Fatalf("insert child job: %v", err)
	}

	// Register dependency: child depends on parent
	_, err = pool.Exec(ctx, `
		INSERT INTO job_dependencies (job_id, depends_on_job_id)
		VALUES ($1, $2)
	`, childJobID, parentJobID)
	if err != nil {
		t.Fatalf("insert dependency: %v", err)
	}

	workerID := uuid.New().String()
	// Register worker so job_executions FK is satisfied
	_, err = pool.Exec(ctx, `INSERT INTO workers (id, hostname, status) VALUES ($1, 'dag-test-host', 'active')`, workerID)
	if err != nil {
		t.Fatalf("register worker: %v", err)
	}

	// Attempt to claim — should only get the parent, NOT the child
	claimed, err := queue.ClaimBatch(ctx, pool, queueID, workerID, 10, 30*time.Second)
	if err != nil {
		t.Fatalf("claim batch: %v", err)
	}

	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job (parent only), got %d", len(claimed))
	}

	if claimed[0].ID != parentJobID {
		t.Errorf("expected parent job %s to be claimed, got %s", parentJobID, claimed[0].ID)
	}

	// Mark parent as completed
	_, err = pool.Exec(ctx, `UPDATE jobs SET status = 'completed', updated_at = now() WHERE id = $1`, parentJobID)
	if err != nil {
		t.Fatalf("mark parent completed: %v", err)
	}

	// Now the child should be claimable
	workerID2 := uuid.New().String()
	// Register second worker
	_, err = pool.Exec(ctx, `INSERT INTO workers (id, hostname, status) VALUES ($1, 'dag-test-host-2', 'active')`, workerID2)
	if err != nil {
		t.Fatalf("register worker2: %v", err)
	}

	claimed2, err := queue.ClaimBatch(ctx, pool, queueID, workerID2, 10, 30*time.Second)
	if err != nil {
		t.Fatalf("second claim batch: %v", err)
	}

	if len(claimed2) != 1 {
		t.Fatalf("expected 1 claimed job (child), got %d", len(claimed2))
	}

	if claimed2[0].ID != childJobID {
		t.Errorf("expected child job %s to be claimed, got %s", childJobID, claimed2[0].ID)
	}
}
