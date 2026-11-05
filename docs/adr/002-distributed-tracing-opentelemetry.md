# ADR 002: Distributed Tracing with OpenTelemetry and W3C Context Propagation

## Status
Accepted

## Context
Jobs in Obsidian span asynchronous boundaries across API ingestion, queue state storage, worker poll/claim routines, and asynchronous task execution. Operators and SREs need end-to-end distributed visibility into job lifecycle latency, bottlenecks, and failure points across separate processes.

## Decision
We adopted the vendor-neutral OpenTelemetry (OTel) Go SDK (`go.opentelemetry.io/otel`) with W3C Trace Context propagation across job lifecycles.

Key implementation details:
1. When jobs are claimed and dispatched, an OpenTelemetry span is initiated or extracted from the parent context.
2. Context is injected into job execution spans (`ExecuteJob`) and batch claims (`ClaimBatch`), tracking worker hostname, queue ID, attempt count, and latency.
3. The telemetry engine provides an HTTP OTLP trace exporter compatible with Jaeger, Prometheus Tempo, and cloud-native APM solutions.
4. If tracing is disabled or exporter connectivity is absent, the system gracefully falls back to a no-op tracer without runtime panics or performance degradation.

## Consequences
### Positive
- **Standardized Observability**: Adheres to CNCF OpenTelemetry standards and W3C trace context standards (`traceparent`).
- **Distributed Lifecycle Correlation**: Correlates API enqueue requests directly to asynchronous worker task executions and database transaction boundaries.
- **Vendor Agnostic**: Seamless integration with Jaeger, Grafana Tempo, Datadog, or AWS X-Ray.

### Negative / Trade-offs
- Slight allocation overhead per job execution for span context creation and attribute serialization.
