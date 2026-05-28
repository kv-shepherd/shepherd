# Shepherd Prometheus Monitoring

This directory contains optional deployment assets for Prometheus-based
monitoring.

| File | Purpose |
|------|---------|
| `shepherd-recording-rules.yml` | Baseline recording rules accepted by ADR-0055 and ADR-0054 |
| `shepherd-alerts.yml` | Minimal alert rule pack accepted by ADR-0055 and ADR-0054 |
| `shepherd-rules.test.yml` | Prometheus rule unit tests accepted by ADR-0055 and ADR-0054 |

Load `shepherd-recording-rules.yml` before `shepherd-alerts.yml`. The alert file
assumes the Prometheus scrape job label is `shepherd`; deployments using another
job name should adjust only the `up{job="shepherd"}` selector. Alertmanager
routing and receivers remain deployment-specific. The repository-owned starter
Grafana dashboard is under `deploy/monitoring/grafana/`.

When `promtool` is available, validate both syntax and sample-series behavior:

```bash
promtool check rules shepherd-recording-rules.yml shepherd-alerts.yml
promtool test rules shepherd-rules.test.yml
```

Repository governance exposes the same checks through:

```bash
make ci-prometheus-rules
PROMTOOL=/path/to/promtool make ci-prometheus-rules
```
