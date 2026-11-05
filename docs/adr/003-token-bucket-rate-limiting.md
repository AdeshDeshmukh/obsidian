# ADR 003: Distributed Token-Bucket Rate Limiting per Queue

## Status
Accepted

## Context
Downstream APIs (such as payment gateways, third-party LLMs, or external microservices) often enforce strict rate limits (e.g., 50 requests/second). Without per-queue throttling, workers claiming jobs at maximum concurrency can inadvertently overwhelm third-party dependencies, leading to HTTP 429 cascades and unexpected failures.

## Decision
We implemented a transactional Token Bucket algorithm directly within PostgreSQL (`internal/queue/ratelimit.go`).

Key implementation details:
1. Each queue stores `rate_limit_per_second`, `tokens_available`, and `last_token_refill_at`.
2. When workers claim jobs, `CheckAndConsumeToken` calculates elapsed time since `last_token_refill_at`, replenishes available tokens up to `rate_limit_per_second`, and consumes one token atomically.
3. If no tokens are available, the claim skips execution or delays claiming until tokens refill, shielding downstream systems without external coordination daemons.

## Consequences
### Positive
- **Guaranteed Throttling**: Strict adherence to defined rate budgets across horizontally scaled worker clusters.
- **Zero External Infrastructure**: Operates cleanly within the existing database schema without requiring an auxiliary Redis cluster.
- **Dynamic Reconfiguration**: Operators can adjust `rate_limit_per_second` on the fly via REST API (`PUT /api/queues/{queueId}/rate-limit`) or CLI (`obsidianctl`).

### Negative / Trade-offs
- Refill calculation requires transactional update on `queues` during claims when rate limiting is actively enabled.
