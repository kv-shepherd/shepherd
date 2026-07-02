# Production Deployment SOP

> **Status**: Active operational guidance
> **Scope**: Single-host or small production Docker Compose deployments using
> `deploy/prod/` assets. Kubernetes-native installs use the public
> `kv-shepherd/helm-charts` repository; this SOP references that path only for
> deployment parity.

## Purpose

This SOP defines the minimum production deployment and upgrade path for
Shepherd. It turns the release script, Compose files, database guide, monitoring
assets, and live E2E runner into one reviewable operating procedure.

This document does not replace accepted ADRs. If deployment behavior requires a
new runtime dependency or changes the PostgreSQL-only baseline, create a new ADR
before changing the procedure.

## Deployment Inputs

| Input | Required | Source |
|-------|----------|--------|
| Release version or explicit images | Yes | `SHEPHERD_VERSION`, `SERVER_IMAGE`, `WEB_IMAGE` |
| Production environment file | Yes | `deploy/prod/.env.prod` copied from `deploy/prod/.env.prod.example` |
| PostgreSQL topology decision | Yes | Bundled PostgreSQL or external `DATABASE_URL` |
| Public base URL and allowed origin | Yes | `SERVER_PUBLIC_BASE_URL`, `SERVER_ALLOWED_ORIGINS` |
| Session and encryption secrets | Yes | `SECURITY_SESSION_SECRET`, `SECURITY_ENCRYPTION_KEY` |
| TLS certificate and key | Production ingress dependent | `deploy/prod/tls/cert.pem`, `deploy/prod/tls/key.pem` or upstream TLS |
| Monitoring image versions | Optional | `PROMETHEUS_IMAGE`, `TEMPO_IMAGE`, `OTEL_COLLECTOR_IMAGE` |
| Live E2E kubeconfig | Go-live validation | `E2E_KUBECONFIG_B64` or `k8s-admin.yaml` |

Keep `SECURITY_ENCRYPTION_KEY` backed up. Losing it breaks decryption for stored
cluster kubeconfigs and console bootstrap material.

## First Deployment

1. Prepare the environment file:

   ```bash
   cp deploy/prod/.env.prod.example deploy/prod/.env.prod
   ```

2. Set the external URL and database topology in `deploy/prod/.env.prod`.

   For bundled PostgreSQL, leave `DATABASE_URL` empty and keep
   `DEPLOY_BUNDLED_POSTGRES=auto` or `true`.

   For external PostgreSQL, set:

   ```env
   DEPLOY_BUNDLED_POSTGRES=false
   DATABASE_URL=postgres://shepherd:<password>@<host>:5432/shepherd_db?sslmode=require
   ```

3. Run the deployment script with release images and bootstrap seed:

   ```bash
   SHEPHERD_VERSION=<version> bash deploy/prod/deploy-prod.sh --release-images --with-seed
   ```

   The seed command uses a 2 minute execution timeout by default. Set
   `SEED_TIMEOUT` to a Go duration such as `5m` when a slow deployment database
   needs a larger bootstrap window.

4. Verify service health:

   ```bash
   docker compose \
     -f deploy/prod/docker-compose.prod.yml \
     --env-file deploy/prod/.env.prod \
     ps

   curl -fsS https://<public-host>/api/v1/health/live
   curl -fsS https://<public-host>/api/v1/health/ready
   ```

5. Complete initial platform-admin handoff using
   [platform-admin-sop.md](./platform-admin-sop.md).

6. Apply and verify PostgreSQL/River autovacuum tuning using
   [database-operations.md](./database-operations.md):

   ```bash
   DATABASE_URL='postgres://shepherd:<password>@<host>:5432/shepherd_db?sslmode=require' \
     make postgres-ops-apply
   ```

## Optional Monitoring Overlay

The optional Compose monitoring overlay is accepted by ADR-0056 and documented
in [compose-monitoring-baseline.md](../design/observability/compose-monitoring-baseline.md).
Prometheus config validation for that overlay is accepted by ADR-0056 and
documented in
[prometheus-config-validation-baseline.md](../design/observability/prometheus-config-validation-baseline.md).

1. Set explicit monitoring image versions:

   ```env
   PROMETHEUS_IMAGE=prom/prometheus:<version>
   PROMETHEUS_PORT=9090
   TEMPO_IMAGE=grafana/tempo:<version>
   TEMPO_PORT=3200
   OTEL_COLLECTOR_IMAGE=otel/opentelemetry-collector-contrib:<version>
   ```

2. Validate the merged Compose configuration before starting it:

   ```bash
   bash docs/design/ci/scripts/check_prometheus_config.sh
   docker compose \
     -f deploy/prod/docker-compose.prod.yml \
     -f deploy/prod/docker-compose.monitoring.yml \
     --env-file deploy/prod/.env.prod \
     config
   ```

3. Start production with the overlay:

   ```bash
   docker compose \
     -f deploy/prod/docker-compose.prod.yml \
     -f deploy/prod/docker-compose.monitoring.yml \
     --env-file deploy/prod/.env.prod \
     up -d
   ```

4. Confirm Prometheus and Tempo:

   ```bash
   curl -fsS http://127.0.0.1:9090/-/ready
   curl -fsS http://127.0.0.1:3200/ready
   curl -fsS http://127.0.0.1:9090/api/v1/query --data-urlencode 'query=up{job="shepherd"}'
   ```

Prometheus, Tempo, and any operator-managed Grafana UI exposure is an operator
decision. Do not expose those UIs publicly without local authentication, TLS,
and network policy controls. Grafana is no longer started by the default Compose
monitoring overlay; import `deploy/monitoring/grafana/dashboards/shepherd-overview.json`
into your own Grafana instance when needed.

## Kubernetes / Helm Monitoring Parity

Kubernetes-native deployments use the public Helm chart repository:

```bash
helm repo add shepherd https://kv-shepherd.github.io/helm-charts
helm repo update
```

For clusters that already run Prometheus Operator, apply the public starter
observability resources from this repository after the Helm release is installed:

```bash
kubectl -n shepherd apply \
  -f deploy/monitoring/prometheus-operator/shepherd-service-monitor.yml \
  -f deploy/monitoring/prometheus-operator/shepherd-prometheus-rule.yml
```

The Shepherd server supports the same `OBSERVABILITY_*` runtime settings as the
production Compose file. The starter `ServiceMonitor` scrapes the server
service's named `http` port at `/metrics`; the starter `PrometheusRule` packages
the accepted Shepherd recording and alert rule baseline. Prometheus,
Alertmanager, Grafana, routing, receiver policy, and retention remain
cluster-owned.

For Docker Compose tracing, the built-in monitoring overlay runs
OpenTelemetry Collector and Tempo. Keep `OBSERVABILITY_TRACING_ENABLED=true`
and point `OTEL_EXPORTER_OTLP_ENDPOINT` /
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` at the Collector defaults from
`.env.prod`. Keep `OBSERVABILITY_TRACE_QUERY_ENABLED=true` and
`OBSERVABILITY_TRACE_QUERY_URL=http://tempo:3200` when the Shepherd
administrator `/admin/observability` trace layer should query the bundled Tempo
service.

## Go-Live Validation

Before declaring a production deployment ready:

1. Run static governance checks from the release candidate:

   ```bash
   bash docs/design/ci/scripts/check_prometheus_config.sh
   make ci-monitoring-assets
   make postgres-ops-check
   bash docs/design/ci/scripts/check_design_doc_governance.sh
   bash scripts/run_e2e_live.sh --preflight-only
   bash docs/design/ci/scripts/check_live_e2e_no_mock.sh
   go run docs/design/ci/scripts/check_master_flow_test_matrix.go
   ```

2. For beta/RC readiness, production go-live, or high-risk provider/runtime
   changes, run live E2E against a real KubeVirt-capable cluster using
   [live-e2e-validation.md](./live-e2e-validation.md). For a normal alpha patch
   or narrow bug fix, document the deferral and require the cheaper gates first:
   backend behavior suites, API contract checks, generated type sync, frontend
   unit/type/build checks, and mock smoke E2E.

3. Capture machine-readable production evidence.

   Prefer the collector so the evidence bundle, command logs, and manifest are
   produced by one repeatable command:

   ```bash
   PRODUCTION_EVIDENCE_REQUIRE_READY=true \
   LIVE_E2E_EVIDENCE_FILE=.run/live-e2e/<date>/<run>/live-e2e.evidence.json \
   ROLLBACK_IMAGE_VERSION=<previous-version-or-image-tag> \
   SERVER_PUBLIC_BASE_URL=https://<public-host> \
     make production-evidence-collect
   ```

   Set `PRODUCTION_EVIDENCE_DIR` to choose the bundle directory. Set
   `DATABASE_URL` or `POSTGRES_OPS_DATABASE_URL` when it is not already present
   in `deploy/prod/.env.prod`. Use `PRODUCTION_EVIDENCE_COLLECT_ARGS` when the
   monitoring overlay or non-default Compose files are part of the deployment:

   ```bash
   PRODUCTION_EVIDENCE_REQUIRE_READY=true \
   PRODUCTION_EVIDENCE_COLLECT_ARGS="--monitoring-overlay" \
   LIVE_E2E_EVIDENCE_FILE=.run/live-e2e/<date>/<run>/live-e2e.evidence.json \
   ROLLBACK_IMAGE_VERSION=<previous-version-or-image-tag> \
   SERVER_PUBLIC_BASE_URL=https://<public-host> \
     make production-evidence-collect
   ```

   The collector runs static governance, monitoring asset validation,
   PostgreSQL/River operations validation, Compose `config --quiet`, Compose
   `ps --all --format json`, Compose `images --format json`, live and ready
   health checks, full live E2E evidence validation, and rollback-record
   capture. It writes `production-deployment-evidence.json` into the evidence
   directory. With `PRODUCTION_EVIDENCE_REQUIRE_READY=true`, the collector also
   enforces `--require-production-ready`, `--require-existing-artifacts`, and
   `--require-live-e2e-pass` before exiting successfully. It must include the
   live E2E evidence manifest path and the manual live E2E evidence bundle
   directory.

   For a manual fallback, start from the checked-in schema example and fill it
   with the release, image, database topology, health-check, full live E2E
   evidence manifest, manual live E2E evidence bundle, and rollback artifact
   paths:

   ```bash
   mkdir -p evidence
   cp docs/operations/production-deployment-evidence.example.json \
     evidence/production-deployment-evidence.json
   ```

   Validate the generated or manually completed manifest before declaring
   go-live complete:

   ```bash
   PRODUCTION_EVIDENCE_FILE=evidence/production-<timestamp>/production-deployment-evidence.json \
     make ci-production-evidence-schema
   ```

   The validator requires production-ready evidence to include a full live E2E
   pass, existing artifacts, successful PostgreSQL/River ops validation, Compose
   config validation, current Compose service/image output, health checks, and a
   rollback record. The default `make ci-production-evidence-schema` invocation
   runs the validator self-test and validates the checked-in schema example; it
   does not claim production readiness.

When live E2E is used for beta/RC or production-readiness evidence, the roadmap
item is not complete until the result comes from a real backend and real
KubeVirt cluster, with cleanup reviewed.

## Upgrade Procedure

1. Back up PostgreSQL before changing images:

   ```bash
   pg_dump "$DATABASE_URL" > "shepherd-$(date +%Y%m%d-%H%M%S).dump"
   ```

2. Record current image references:

   ```bash
   docker compose \
     -f deploy/prod/docker-compose.prod.yml \
     --env-file deploy/prod/.env.prod \
     images
   ```

3. Update `SHEPHERD_VERSION`, `SERVER_IMAGE`, or `WEB_IMAGE`.

4. Render the Compose configuration:

   ```bash
   docker compose \
     -f deploy/prod/docker-compose.prod.yml \
     --env-file deploy/prod/.env.prod \
     config
   ```

5. Deploy:

   ```bash
   SHEPHERD_VERSION=<new-version> bash deploy/prod/deploy-prod.sh --release-images
   ```

6. Verify health, `/metrics`, and the starter dashboard if monitoring is
   enabled.

7. Run live E2E or, at minimum for a narrow emergency patch, document why live
   E2E was deferred and run the backend behavior suites plus frontend smoke
   tests before opening traffic.

## Rollback Procedure

Rollback is image-first and database-aware:

1. Stop traffic at the ingress or load balancer.
2. Restore the previous `SERVER_IMAGE` and `WEB_IMAGE` values in
   `deploy/prod/.env.prod`.
3. Render Compose config and restart services.
4. If the failed upgrade applied incompatible migrations, restore the database
   backup into a new database and update `DATABASE_URL`. Do not run ad hoc down
   migrations against production without a reviewed recovery plan.
5. Verify `/api/v1/health/ready`, login, VM list, approvals, and `/metrics`.
6. Record the rollback image version, database restore point, and operator.

## Related Assets

| Asset | Role |
|-------|------|
| [deploy-prod.sh](../../deploy/prod/deploy-prod.sh) | Production deployment entrypoint |
| [docker-compose.prod.yml](../../deploy/prod/docker-compose.prod.yml) | Base production Compose topology |
| [docker-compose.monitoring.yml](../../deploy/prod/docker-compose.monitoring.yml) | Optional Prometheus, Tempo, and OpenTelemetry Collector overlay |
| [database-operations.md](./database-operations.md) | PostgreSQL/River operational requirements |
| [platform-admin-sop.md](./platform-admin-sop.md) | Initial admin handoff and access review |
| [live-e2e-validation.md](./live-e2e-validation.md) | Real-cluster E2E validation |
