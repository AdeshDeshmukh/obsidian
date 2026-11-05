# ADR 004: Composite Queue Health Scoring and Diagnostics

## Status
Accepted

## Context
Operators monitoring thousands of distributed queues need an immediate, high-level synthetic health indicator to spot degrading queues before SLA breaches occur. Relying solely on raw DLQ counters or backlog counts forces manual correlation of multiple metrics.

## Decision
We implemented a composite queue health scoring engine (`internal/metrics/health.go`) that combines recent DLQ ingestion rates, execution failure rates, and backlog queue depths into a single 0–100 numerical score and A–F letter grade.

Key implementation details:
1. Penalties are computed dynamically over a rolling 1-hour window:
   - **DLQ Rate Penalty**: Up to -50 points ($DLQRate \times 500$, capped at 50).
   - **Failure Rate Penalty**: Up to -30 points ($FailRate \times 150$, capped at 30).
   - **Queue Depth Backlog Penalty**: Up to -20 points ($QueueDepth / 50$, capped at 20).
2. Letter grades map intuitively:
   - Grade **A**: 90–100
   - Grade **B**: 75–89
   - Grade **C**: 60–74
   - Grade **D**: 40–59
   - Grade **F**: <40
3. Exposes diagnostics via `GET /api/queues/{queueId}/health` using subqueries to prevent Cartesian product blowup across execution logs.

## Consequences
### Positive
- **Instant Triage**: Operators and automated alerting systems immediately know which queues require attention.
- **Predictable Degradation**: Graceful scoring curve clearly distinguishes between temporary backlog growth and catastrophic worker failure.

### Negative / Trade-offs
- Aggregating stats over a rolling 1-hour window requires reading job execution and status timestamps, mitigated by database indexes.
