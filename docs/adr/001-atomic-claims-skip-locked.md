# ADR 001: Atomic Job Claims via PostgreSQL `FOR UPDATE SKIP LOCKED`

## Status
Accepted

## Context
In a distributed background job scheduling platform with multiple competing worker nodes, ensuring safe, concurrent job claims without race conditions or duplicate execution is critical. Traditional approaches often rely on external distributed lock managers (such as Redis/Redlock, ZooKeeper, or etcd), which introduces operational complexity, split-brain failure modes, and cache-database synchronization drift.

## Decision
We utilize PostgreSQL's native row-level locking via `SELECT ... FOR UPDATE SKIP LOCKED` within transactional batches inside `ClaimBatch`.

Key implementation details:
1. A transaction selects up to `batch_size` jobs in `queued` status that have reached their `run_at` timestamp.
2. `SKIP LOCKED` instructs Postgres to skip rows currently locked by other concurrent transactions rather than waiting, eliminating lock contention and worker wait-states.
3. The query updates the job status to `running`, assigns `claimed_by = worker_id`, and sets `lease_expires_at = now() + lease_duration`.
4. In-flight jobs are tracked via periodic worker heartbeats. If a worker dies, its lease expires, and the scheduler janitor reclaims the abandoned job without external distributed lock managers.

## Consequences
### Positive
- **Zero Double Claims**: Guaranteed atomic isolation at the database storage engine layer.
- **High Concurrency**: Sub-millisecond queue throughput with near-zero lock waiting contention across competing workers.
- **Architectural Simplicity**: Single source of truth in PostgreSQL with transactional ACID consistency.

### Negative / Trade-offs
- Increased write and locking load on PostgreSQL under extreme sustained concurrency, necessitating appropriate connection pooling (`pgxpool`) and partial indexing on `(queue_id, status, priority DESC, run_at)`.
