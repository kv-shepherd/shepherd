# Docker Compose Monitoring Baseline

> **Decision**: [ADR-0056](../../adr/ADR-0056-observability-deployment-packaging-baseline.md)

## Scope

The Compose monitoring baseline provides an optional overlay for the production
Docker Compose topology. It wires Prometheus, Tempo, and OpenTelemetry
Collector to the Shepherd server service without making those components a
mandatory Shepherd runtime dependency.

Overlay:

```text
deploy/prod/docker-compose.monitoring.yml
```

## Assets

| File | Purpose |
|------|---------|
| `deploy/prod/docker-compose.monitoring.yml` | Optional Prometheus, Tempo, and OpenTelemetry Collector services |
| `deploy/monitoring/prometheus/prometheus.yml` | Prometheus scrape and rule configuration |
| `deploy/monitoring/grafana/provisioning/datasources/prometheus.yml` | Optional Grafana Prometheus and Tempo datasource example |
| `deploy/monitoring/tempo/tempo.yml` | Tempo local trace backend configuration |
| `deploy/monitoring/otel-collector/otel-collector.yml` | OpenTelemetry Collector OTLP receiver and Tempo exporter |
| `deploy/monitoring/grafana/dashboards/shepherd-overview.json` | Optional starter Shepherd dashboard for manual import |

## Runtime Contract

Prometheus scrapes the Compose server service:

| Field | Value |
|-------|-------|
| job name | `shepherd` |
| target | `server:8080` |
| metrics path | `/metrics` |
| scrape interval | `30s` |

The Prometheus config loads recording rules before alert rules:

```yaml
rule_files:
  - /etc/prometheus/rules/shepherd-recording-rules.yml
  - /etc/prometheus/rules/shepherd-alerts.yml
```

The optional Grafana provisioning example defines a Prometheus datasource with
UID `prometheus` and a Tempo datasource with UID `tempo`. The Compose overlay
does not start Grafana by default; operators who want Grafana can run their own
instance and import the repository-owned dashboard JSON.

Shepherd exports OTLP/HTTP traces to `otel-collector:4318`; the Collector batches traces and
exports them to Tempo over OTLP/gRPC on `tempo:4317`.

The Shepherd server also receives `OBSERVABILITY_TRACE_QUERY_ENABLED=true` and
`OBSERVABILITY_TRACE_QUERY_URL=http://tempo:3200` so the protected
`/admin/observability` page can render administrator trace summaries directly
from Tempo.

Prometheus config validation is governed separately by
[prometheus-config-validation-baseline.md](./prometheus-config-validation-baseline.md).
The source config keeps container-mounted rule paths; the validation gate
renders a temporary local copy so `promtool check config` can load the
repository rule files.

## Usage

Create or update `deploy/prod/.env.prod` with monitoring values:

```env
PROMETHEUS_IMAGE=prom/prometheus:<version>
TEMPO_IMAGE=grafana/tempo:<version>
OTEL_COLLECTOR_IMAGE=otel/opentelemetry-collector-contrib:<version>
OBSERVABILITY_TRACE_QUERY_ENABLED=true
OBSERVABILITY_TRACE_QUERY_URL=http://tempo:3200
```

Then run:

```bash
docker compose \
  -f deploy/prod/docker-compose.prod.yml \
  -f deploy/prod/docker-compose.monitoring.yml \
  --env-file deploy/prod/.env.prod \
  up -d
```

## Validation

The governance check
`docs/design/ci/scripts/check_monitoring_compose_assets.sh` validates the
Compose overlay, Prometheus config, and optional Grafana datasource/dashboard assets. The
check always runs static structure validation. When Docker Compose is available,
it also renders the merged production Compose configuration with dummy required
environment values:

```bash
docker compose \
  -f deploy/prod/docker-compose.prod.yml \
  -f deploy/prod/docker-compose.monitoring.yml \
  config
```

This catches interpolation, merge, and syntax drift without starting
containers. If Docker Compose is unavailable, the static checks remain
mandatory and the render check is skipped.

`make ci-monitoring-assets` also runs
`docs/design/ci/scripts/check_prometheus_config.sh`, which validates the
Prometheus config with `promtool check config` when `promtool` is available and
fails closed under `PROMTOOL_REQUIRED=1`.

## Deferred Work

This baseline does not define:

* Alertmanager routing or receiver configuration.
* Remote write, long-term storage, or retention policy.
* TLS or auth policy for Prometheus, Tempo, or any operator-managed Grafana UI.
* Production backup policy for Prometheus/Tempo volumes.
* Jaeger as an additional built-in trace query UI.
