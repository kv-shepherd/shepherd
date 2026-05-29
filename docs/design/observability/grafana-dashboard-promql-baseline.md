# Grafana Dashboard PromQL Validation Baseline

> **Decision**: [ADR-0055](../../adr/ADR-0055-prometheus-rules-and-grafana-dashboard-baseline.md)

## Scope

The baseline validates PromQL syntax for the starter Grafana dashboard accepted
by ADR-0055:

```text
deploy/monitoring/grafana/dashboards/shepherd-overview.json
```

The check complements `check_grafana_dashboards.sh`, which owns dashboard JSON
shape, panel inventory, datasource wiring, low-cardinality label policy, and
metric allowlists. This baseline proves that every Prometheus target expression
in the reviewed dashboard can be parsed by Prometheus tooling.

## Validation Contract

`docs/design/ci/scripts/check_grafana_dashboard_promql.sh` extracts each
`targets[].expr` value from every dashboard panel, renders a temporary
Prometheus rule file, and runs:

```bash
promtool check rules <temporary-rules.yml>
```

The temporary rule file is shaped like this:

```yaml
groups:
  - name: shepherd.grafana.dashboard
    rules:
      - record: shepherd:grafana_dashboard_panel_<panel-id>_<ref-id>:query
        expr: |
          <dashboard panel PromQL>
```

The generated recording-rule names are local validation wrappers only. They are
not deployment assets, and they must not be copied into the production
Prometheus rule pack.

## Tooling Policy

The checker uses `docs/design/ci/scripts/promtool_lib.sh` for the shared
`PROMTOOL` and `PROMTOOL_REQUIRED` contract:

| Mode | Behavior |
|------|----------|
| `promtool` found | Run `promtool check rules` on the generated rule file |
| `PROMTOOL=/absolute/path` | Use the explicit executable |
| `PROMTOOL_REQUIRED=1` and no `promtool` | Fail closed |
| no `promtool`, not required | Skip parser validation after the structural dashboard gate |

CI and release evidence paths set `PROMTOOL_REQUIRED=1`, so missing Prometheus
tooling cannot silently downgrade this validation.

## Boundaries

This baseline validates syntax only. It does not prove panel rendering, query
cardinality at production scale, datasource availability, panel thresholds, or
dashboard UX. Those checks require either live monitoring environments or later
dashboard E2E work.

No Grafana server or Prometheus server is required for this baseline.

## Governance Entry Points

The validation is blocking through:

```bash
bash docs/design/ci/scripts/check_grafana_dashboard_promql.sh
make ci-grafana-dashboard-promql
make ci-monitoring-assets
PROMTOOL_REQUIRED=1 make ci-governance
```
