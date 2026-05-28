# Docker Compose Monitoring Baseline

> **Decision**: [ADR-0056](../../adr/ADR-0056-observability-deployment-packaging-baseline.md)

## Scope

The Compose monitoring baseline provides an optional overlay for the production
Docker Compose topology. It wires Prometheus and Grafana to the Shepherd server
service without making either component a mandatory Shepherd runtime dependency.

Overlay:

```text
deploy/prod/docker-compose.monitoring.yml
```

## Assets

| File | Purpose |
|------|---------|
| `deploy/prod/docker-compose.monitoring.yml` | Optional Prometheus + Grafana services |
| `deploy/monitoring/prometheus/prometheus.yml` | Prometheus scrape and rule configuration |
| `deploy/monitoring/grafana/provisioning/datasources/prometheus.yml` | Grafana Prometheus datasource |
| `deploy/monitoring/grafana/dashboards/shepherd-overview.json` | Starter Shepherd dashboard |

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

Grafana provisions a Prometheus datasource with UID `prometheus`, matching the
starter dashboard datasource variable default.

Prometheus config validation is governed separately by
[prometheus-config-validation-baseline.md](./prometheus-config-validation-baseline.md).
The source config keeps container-mounted rule paths; the validation gate
renders a temporary local copy so `promtool check config` can load the
repository rule files.

## Usage

Create or update `deploy/prod/.env.prod` with monitoring values:

```env
PROMETHEUS_IMAGE=prom/prometheus:<version>
GRAFANA_IMAGE=grafana/grafana:<version>
GRAFANA_ADMIN_PASSWORD=<strong-password>
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
Compose overlay, Prometheus config, and Grafana datasource provisioning. The
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
* TLS or auth policy for Prometheus/Grafana UIs.
* Production backup policy for Prometheus/Grafana volumes.
