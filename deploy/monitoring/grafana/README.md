# Shepherd Grafana Monitoring

This directory contains optional deployment assets for Grafana-based
observability.

| Path | Purpose |
|------|---------|
| `dashboards/shepherd-overview.json` | Starter overview dashboard accepted by ADR-0055, including ADR-0054 River queue health panels |
| `provisioning/dashboards/shepherd.yml` | Grafana file-provisioning example for the dashboard |
| `provisioning/datasources/prometheus.yml` | Prometheus datasource provisioning for Compose monitoring |

The dashboard uses a Grafana datasource variable named `datasource` and assumes
the Prometheus scrape job label is `shepherd` for the target-availability panel.
River queue panels consume the ADR-0054 metric and recording-rule baseline.
Deployments may adapt the datasource UID, folder, or provisioning path without
changing Shepherd runtime behavior.
