# Shepherd OpenTelemetry Collector

This directory contains the Compose OpenTelemetry Collector configuration.

The Collector receives Shepherd OTLP traces on:

| Protocol | Endpoint |
|----------|----------|
| OTLP/gRPC | `otel-collector:4317` |
| OTLP/HTTP | `otel-collector:4318` |

It runs the trace pipeline through `memory_limiter` and `batch` processors, then
exports to Tempo over OTLP/gRPC at `tempo:4317`. Applications should send traces
to the Collector, not directly to Tempo, so the ingest boundary stays stable if
the trace backend changes later.
