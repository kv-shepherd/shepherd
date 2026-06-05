# Shepherd Grafana Monitoring

This directory contains optional Grafana assets for operators who want to import
Shepherd dashboards into their own Grafana instance. Shepherd does not start or
embed Grafana by default.

| Path | Purpose |
|------|---------|
| `dashboards/shepherd-overview.json` | Starter overview dashboard including River queue health, current firing alert status, and approval/audit business monitoring panels |
| `provisioning/dashboards/shepherd.yml` | Grafana file-provisioning example for the dashboard |
| `provisioning/datasources/prometheus.yml` | Prometheus and Tempo datasource provisioning example |

The dashboard uses a Grafana datasource variable named `datasource` and assumes
the Prometheus scrape job label is `shepherd` for the target-availability panel
and the `service="shepherd"` alert label for firing-alert panels. River queue,
OpenAPI, and approval/audit business panels render healthy zero states as `0`
instead of Grafana `No data` where the underlying metric series may be absent
until work or validation failures occur. Deployments may adapt the datasource
UID, folder, or provisioning path without changing Shepherd runtime behavior.

The default Docker Compose and local development stacks do not expose Grafana
below the Shepherd origin. Import the dashboard JSON manually, or copy the
provisioning examples into an operator-managed Grafana deployment. Shepherd's
administrator observability page renders operational summaries natively instead
of embedding a Grafana iframe.

This Compose monitoring bundle installs Tempo as the built-in trace backend and
uses OpenTelemetry Collector as the application trace ingest point. It does not
install Loki, Elasticsearch, or generic log-derived alert rules. Approval/audit
business monitoring is implemented through database-backed metrics from
Shepherd tables, not through a log search backend.
