# Design Decisions Document

This document explains the core technical trade-offs and decisions made during the design of the **Obsidian Distributed Job Scheduling Platform**.

---

## 1. Technology Choice: Go + PostgreSQL

### Why Go?
- **First-class concurrency**: Goroutines and channels make it natural to model worker pools, heartbeat loops, and parallel job execution without thread management overhead.
- **Single binary deployment**: Each service (API, Worker, Scheduler) compiles to a static binary — no runtime, no dependency installs, no container-specific language runtimes needed.
- **Performance**: Go's garbage collector is optimized for low-latency server workloads. For a job scheduler where claim latency matters, this is critical.

### Why PostgreSQL over Redis/RabbitMQ?
- **Transactional guarantees**: Job claiming, dependency checking, and status updates happen within ACID transactions — no need for distributed consensus.
- **`FOR UPDATE SKIP LOCKED`**: Postgres provides built-in row-level locking that eliminates duplicate claims without external coordination. Redis would require Lua scripts or Redlock for equivalent safety.
- **Single infrastructure dependency**: Using Postgres for both application state and the queue mechanism means one fewer service to deploy, monitor, and maintain.
- **Trade-off acknowledged**: A dedicated message broker (RabbitMQ, Kafka) would provide higher throughput for very high volume (100k+ jobs/sec). Postgres is appropriate for the 1k-10k/sec range this system targets.

### Why pgx over GORM?
- **Direct SQL control**: The claiming query uses advanced Postgres features (`FOR UPDATE OF j SKIP LOCKED`, partial indexes, advisory locks) that ORMs abstract away or poorly support.
- **Performance**: pgx's native Postgres protocol implementation avoids the reflection overhead of ORMs.
- **Trade-off**: Developers must write SQL manually, which increases boilerplate but gives full control over query plans.

---

## 2. Concurrency Model: `SELECT ... FOR UPDATE SKIP LOCKED`

### Context
In a distributed job scheduler, multiple workers poll a shared database for jobs in the `QUEUED` state. The naive approach to claiming a job is:
```sql
UPDATE jobs SET status = 'claimed' WHERE id = (
  SELECT id FROM jobs WHERE status = 'queued' ORDER BY priority DESC LIMIT 1
) RETURNING *;
```

### The Problem (Race Conditions)
Under high concurrency, two workers running this transaction simultaneously can read the same "queued" job. Worker A reads job `J1` and is about to lock/update it. Before Worker A commits, Worker B also reads job `J1`. Both workers will claim and execute job `J1`, leading to duplicate execution and violation of safety invariants.

### The Solution: Postgres Row Locks with `SKIP LOCKED`
Obsidian implements atomic claiming inside `internal/queue/claim.go`:
```sql
SELECT j.id FROM jobs j
JOIN queues q ON j.queue_id = q.id
WHERE j.status = 'queued'
  AND q.is_paused = false
ORDER BY j.priority DESC
FOR UPDATE OF j SKIP LOCKED
LIMIT $1
```
- `FOR UPDATE` locks the returned rows so no other transaction can read or write to them.
- `OF j` scopes the lock to the `jobs` table only — without this qualifier, the JOIN would also lock the `queues` row, creating a single-worker bottleneck since all workers share the same queue row.
- `SKIP LOCKED` instructs Postgres that if a row is already locked by another worker's query, skip it and immediately return the next available row.
- This results in a completely wait-free, collision-free, atomic queue processing mechanism capable of scaling linearly with the number of worker processes.

---

## 3. Distributed Locking for Cron: Postgres Advisory Locks

### Context
Unlike immediate tasks, recurring cron jobs must be evaluated on a schedule and enqueued *exactly once* per tick. If the scheduler daemon runs in high-availability mode (multiple replicas), we must prevent multiple scheduler nodes from enqueuing duplicate instances of the same cron tick.

### The Trade-off: External KV Store vs. Advisory Locks
- **Option A: Redis / ZooKeeper**: Requires adding an external dependency to handle locks. While standard, it complicates deployment and introduces network partition risks.
- **Option B: Postgres Advisory Locks (Chosen)**: We use Postgres session-level advisory locks via `pg_try_advisory_lock(key)`. We hash the `queue_id` to generate a lock key.
- **Result**: Only one scheduler node can hold the lock for a given queue's cron dispatcher tick. It ensures that cron jobs are dispatched exactly once, without needing any external coordination stores.

### Double-dispatch prevention
Even with advisory locks, a race exists: two schedulers could both read a `next_run_at` in the past before either updates it. We guard against this by:
1. Acquiring the advisory lock per queue.
2. Re-reading `next_run_at` inside the transaction with `FOR UPDATE`.
3. Checking if it's still in the past before inserting and advancing.

---

## 4. Reliability: Heartbeat Leases and Cleaners

### Context
If a worker crashes, loses power, or goes offline mid-execution, its active jobs will remain stuck in the `RUNNING` status indefinitely.

### The Solution: Lease Heartbeats
- When a worker claims a job, it sets a `lease_expires_at` timestamp (e.g. `now() + 30 seconds`).
- While executing the job, the worker runs a background goroutine updating the lease every 10 seconds.
- If the worker crashes, heartbeats stop.
- The scheduler janitor loop runs `ReclaimExpired` on every tick:
  ```sql
  UPDATE jobs
  SET status = 'queued', claimed_by = NULL, lease_expires_at = NULL, attempt = attempt + 1
  WHERE status = 'running' AND lease_expires_at < now();
  ```
- Unfinished jobs are returned to the queue automatically. This guarantees **at-least-once execution** in the face of worker hardware failures.

### Chaos Testing Proof
The `chaos/kill-worker-demo.sh` script proves this mechanism end-to-end:
1. Starts a worker, enqueues a 20-second sleep job.
2. Kills the worker with `SIGKILL` after 3 seconds.
3. Monitors the job status — observes transition from `running` → `queued` (lease expired) → `running` (reclaimed by another worker if started) → `completed`.

---

## 5. Normalization vs. Performance

- **Hot Path Caching**: The `jobs` table carries a partial index:
  ```sql
  CREATE INDEX idx_jobs_claim_scan ON jobs (queue_id, status, priority DESC, run_at)
  WHERE status IN ('queued', 'scheduled');
  ```
  This index is small and fast because it ignores completed/failed/cancelled jobs, keeping queue claims under 1ms even as the job history table grows to millions of rows.
- **Separation of Concerns**: Log strings are pushed into the append-only `job_logs` table rather than stored directly in the `jobs` rows, keeping the hot path table compact.
- **Execution History**: `job_executions` is separate from `jobs` because a single job may have multiple execution attempts. This 1:N relationship is normalized correctly.

---

## 6. Cascade Deletion Trade-offs

- `ON DELETE CASCADE` from `queues → jobs → job_executions → job_logs` is intentional: deleting a queue removes all its job history.
- **Trade-off**: This means hard-deleting a queue destroys audit history. A production system would implement soft-delete (`deleted_at` timestamps) to preserve audit trails.
- **Why CASCADE was chosen**: For this submission, operational simplicity was prioritized over audit retention. The schema is easy to extend with soft-delete later.

---

## 7. Graceful Shutdown Design

### The Problem
When a worker receives `SIGTERM`, it must:
1. Stop claiming new jobs (stop the polling loop).
2. Let in-flight jobs finish (don't corrupt running work).
3. Eventually force-kill if jobs hang (don't block shutdown forever).

### The Solution: Decoupled Contexts
- The **polling loop** uses the signal-aware context. When cancelled, it stops the `for/select` loop.
- **Active job executions** run under a separate `jobCtx` derived from `context.Background()`, NOT from the signal context. This means cancelling the signal doesn't immediately kill running jobs.
- A **10-second drain timeout** is enforced: if jobs don't finish within 10 seconds of shutdown signal, the `jobCtx` is cancelled, forcing them to abort.

---

## 8. Idempotency Key Design

- Jobs can include an `idempotency_key` field. The `jobs` table enforces `UNIQUE (queue_id, idempotency_key)`.
- On `CreateJob`, if a matching key exists, the API returns the existing job (HTTP 200) instead of creating a duplicate (HTTP 201).
- This allows clients to safely retry failed API calls without risk of double-enqueuing work.
- **Trade-off**: Idempotency keys are scoped per queue, not globally. This is intentional — the same logical operation in two different queues may be two distinct jobs.

---

## 9. DAG Dependency Resolution

- Jobs can declare dependencies via the `job_dependencies` junction table.
- The `ClaimBatch` query includes a `NOT EXISTS` subquery that filters out jobs with incomplete dependencies:
  ```sql
  AND NOT EXISTS (
      SELECT 1 FROM job_dependencies jd
      JOIN jobs dep ON dep.id = jd.depends_on_job_id
      WHERE jd.job_id = j.id AND dep.status != 'completed'
  )
  ```
- **No polling overhead**: When a parent job completes, the child automatically becomes claimable on the next poll — the predicate naturally evaluates to true.
- **Trade-off**: Circular dependency detection is not implemented at the application level. It relies on the client to construct valid DAGs.
