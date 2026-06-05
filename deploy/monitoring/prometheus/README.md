# Shepherd Prometheus Monitoring

This directory contains optional deployment assets for Prometheus-based
monitoring.

| File | Purpose |
|------|---------|
| `shepherd-recording-rules.yml` | Baseline recording rules for HTTP, OpenAPI, River, and approval/audit business metrics |
| `shepherd-alerts.yml` | Minimal alert rule pack including approval backlog and approval failure alerts |
| `shepherd-rules.test.yml` | Prometheus rule unit tests for the recording and alert rule pack |

Load `shepherd-recording-rules.yml` before `shepherd-alerts.yml`. The alert file
assumes the Prometheus scrape job label is `shepherd`; deployments using another
job name should adjust only the `up{job="shepherd"}` selector. Alertmanager
routing and receivers remain deployment-specific. The repository-owned optional
Grafana dashboard JSON is under `deploy/monitoring/grafana/` for manual import.

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
