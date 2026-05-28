# Prometheus Config Validation Baseline

> **Decision**: [ADR-0056](../../adr/ADR-0056-observability-deployment-packaging-baseline.md)

## Scope

The Prometheus config validation baseline protects the repository-owned
Prometheus entrypoint used by the optional Compose monitoring overlay.
Individual rule files and rule-unit tests are already validated separately; this
gate validates the config that loads those files and scrapes Shepherd.

Config file:

```text
deploy/monitoring/prometheus/prometheus.yml
```

## Runtime Contract

The deployment config keeps container-mounted rule paths because Prometheus runs
inside the Compose service:

```yaml
rule_files:
  - /etc/prometheus/rules/shepherd-recording-rules.yml
  - /etc/prometheus/rules/shepherd-alerts.yml
```

The config must keep:

| Field | Required value |
|-------|----------------|
| `global.scrape_interval` | `30s` |
| `global.evaluation_interval` | `30s` |
| rule load order | recording rules before alert rules |
| scrape job | `job_name: shepherd` |
| scrape path | `/metrics` |
| scrape target | `server:8080` |

## Local Validation Rendering

`promtool check config` validates referenced rule files. Because the repository
config uses container paths, the governance script renders a temporary local
copy with these path substitutions before invoking `promtool`:

| Container path | Local validation path |
|----------------|-----------------------|
| `/etc/prometheus/rules/shepherd-recording-rules.yml` | `<repo>/deploy/monitoring/prometheus/shepherd-recording-rules.yml` |
| `/etc/prometheus/rules/shepherd-alerts.yml` | `<repo>/deploy/monitoring/prometheus/shepherd-alerts.yml` |

The source config is not rewritten. Only the temporary validation copy receives
absolute local paths.

## Validation

The governance check is:

```bash
bash docs/design/ci/scripts/check_prometheus_config.sh
```

If `promtool` is installed, the check runs:

```bash
promtool check config --lint=duplicate-rules <rendered-temp-config>
```

When `promtool` is unavailable, fallback structural checks remain mandatory but
are weaker than release evidence. CI and release validation run with:

```bash
PROMTOOL_REQUIRED=1 make ci-governance
```

`make ci-monitoring-assets` includes this config check alongside recording
rules, alert rules, rule tests, Prometheus Operator assets, Compose overlay
assets, and Grafana dashboard assets.

## Deferred Work

This baseline does not define Alertmanager config validation, remote write
validation, retention policy, TLS, authentication, or production receiver
routing. Those remain deployment-specific RFC-0010 follow-ups.
