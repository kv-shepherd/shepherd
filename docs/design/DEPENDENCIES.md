# Dependency Version Definitions

> **Purpose**: Authoritative dependency and toolchain version reference.
> **Last audited**: 2026-05-28.
> **Primary sources**: `go.mod`, `Makefile`, `build/api.mk`, `web/package.json`,
> `api/openapi.yaml`.

Other design documents should link here instead of hardcoding dependency
versions.

## Go Version

| Item | Version | Source |
|------|---------|--------|
| Go toolchain | `1.25.11` | `go.mod`, `Makefile` `GO_TOOLCHAIN_VERSION`, `Dockerfile`, `deploy/prod/deploy-prod.sh` |

ADR-0028 requires Go support for `omitzero`; CI currently standardizes on Go
`1.25.11`.

## Core Dependencies

### Backend Runtime

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/gin-gonic/gin` | `v1.12.0` | HTTP router |
| `github.com/gin-contrib/cors` | `v1.7.7` | CORS middleware |
| `github.com/go-playground/validator/v10` | `v10.30.2` | Request/struct validation |
| `entgo.io/ent` | `v0.14.6` | ORM |
| `github.com/jackc/pgx/v5` | `v5.9.2` | PostgreSQL driver and pool |
| `github.com/riverqueue/river` | `v0.37.0` | PostgreSQL-native job queue |
| `github.com/riverqueue/river/riverdriver/riverpgxv5` | `v0.37.0` | River pgx v5 driver |
| `github.com/sqlc-dev/sqlc` | `v1.30.0` | SQL code generation; invoked by `make sqlc-gen` |
| `go.uber.org/zap` | `v1.28.0` | Structured logging |
| `github.com/spf13/viper` | `v1.21.0` | Configuration |
| `github.com/robfig/cron/v3` | `v3.0.1` | Directory-enrichment schedule parsing |
| `github.com/panjf2000/ants/v2` | `v2.12.0` | In-process worker pool |
| `github.com/prometheus/client_golang` | `v1.23.2` | Prometheus metrics registry, collectors, and `/metrics` exposition |
| `github.com/prometheus/client_model` | `v0.6.2` | Prometheus metric DTOs used by collector tests |
| `go.opentelemetry.io/otel` | `v1.44.0` | OpenTelemetry API and context propagation |
| `go.opentelemetry.io/otel/sdk` | `v1.44.0` | OpenTelemetry tracer provider and sampling |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` | `v1.44.0` | OTLP/HTTP trace exporter |
| `go.opentelemetry.io/otel/exporters/stdout/stdouttrace` | `v1.44.0` | Local stdout trace exporter for tests and diagnostics |
| `go.opentelemetry.io/otel/trace` | `v1.44.0` | Trace API helpers used by middleware tests |
| `golang.org/x/sync` | `v0.20.0` | Semaphore/errgroup utilities |

### Database

| Component | Version | Notes |
|-----------|---------|-------|
| PostgreSQL | `18` | Development and production baseline |
| Ent migrations | Ent-generated schema | Clean local bootstrap can use `database.auto_migrate`; production startup applies reviewed Atlas migrations and uses Ent only for empty-database bootstrap |
| Atlas config | `migrations/atlas/atlas.hcl` | Atlas CLI is bundled in Docker images and Go runtime archives for startup migrations; `ariga.io/atlas` appears indirectly through Ent |
| River migrations | River `rivermigrate` | Applied during startup after app schema preparation |

### Kubernetes and KubeVirt

| Package | Version | Purpose |
|---------|---------|---------|
| `k8s.io/api` | `v0.34.3` | Kubernetes API types |
| `k8s.io/apimachinery` | `v0.34.3` | Kubernetes API machinery |
| `k8s.io/client-go` | `v0.34.3` | Kubernetes clients |
| `kubevirt.io/api` | `v1.8.2` | KubeVirt API types |
| `kubevirt.io/client-go` | `v1.8.2` | Official KubeVirt client |
| `kubevirt.io/containerized-data-importer-api` | `v1.64.0` | CDI DataVolume and StorageProfile types |
| `sigs.k8s.io/yaml` | `v1.6.0` | YAML conversion |

Kubernetes core packages are replace-locked in `go.mod` to the same `v0.34.3`
series used by the KubeVirt `v1.8.2` baseline. Do not upgrade one Kubernetes
core package independently.

SSA is implemented through `dynamic.Interface` and
`types.ApplyPatchType` in `internal/provider/ssa_applier.go`. The current
runtime does not depend on `sigs.k8s.io/controller-runtime`.

### API Contract Tooling

| Tool or package | Version | Source |
|-----------------|---------|--------|
| OpenAPI spec | `3.1.0` | `api/openapi.yaml` |
| `github.com/oapi-codegen/oapi-codegen/v2` | `v2.5.1` | `build/api.mk`, CI version check |
| `github.com/oapi-codegen/runtime` | `v1.4.0` | `go.mod` |
| `openapi-typescript` | `^7.13.0` | `web/package.json` |
| `openapi-fetch` | `^0.16.0` | `web/package.json` |
| `github.com/daveshanley/vacuum` | `v0.23.8` | `build/api.mk` |
| `github.com/oasdiff/oasdiff` | `v1.11.10` | `build/api.mk` |
| `github.com/pb33f/libopenapi` | `v0.36.3` | `go.mod` |
| `github.com/pb33f/libopenapi-validator` | `v0.13.4` | `go.mod` |
| `github.com/getkin/kin-openapi` | `v0.134.0` | `go.mod` |

Canonical flow:

```text
api/openapi.yaml
  -> internal/api/specembed/openapi.yaml
  -> internal/api/generated/server.gen.go
  -> web/src/types/api.gen.ts
```

Use `make api-generate` to regenerate Go and TypeScript artifacts. Use
`REQUIRE_OPENAPI_COMPAT=1 make api-check` to enforce the OpenAPI 3.0-compatible
artifact when 3.1 features require it.

### Authentication and Security

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/golang-jwt/jwt/v5` | `v5.3.1` | Shepherd JWTs |
| `golang.org/x/crypto` | `v0.51.0` | bcrypt and crypto utilities |
| `github.com/go-ldap/ldap/v3` | `v3.4.13` | LDAP auth provider |
| `github.com/gorilla/websocket` | `v1.5.4-0.20250319132907-e064f32e3674` | Console websocket handling |

V1 uses JWT auth with a DB-bootstrapped signing secret. PostgreSQL stores
bootstrap secrets and console replay markers; the current runtime does not use
`alexedwards/scs` server-side browser sessions.

### Frontend Runtime

| Package | Version | Purpose |
|---------|---------|---------|
| `next` | `^16.2.3` | Next.js App Router |
| `react` | `19.2.3` | React runtime |
| `react-dom` | `19.2.3` | React DOM |
| `antd` | `^5.29.3` | UI components |
| `@ant-design/pro-components` | `^2.8.10` | Admin/table components |
| `@ant-design/icons` | `^5.6.1` | Icon set |
| `@tanstack/react-query` | `^5.95.2` | Server-state management |
| `zustand` | `^5.0.12` | Client-state management |
| `tailwindcss` | `^4.2.2` | Utility CSS |
| `zod` | `^4.3.6` | Form/data validation |
| `i18next` | `^25.10.9` | i18n core |
| `react-i18next` | `^16.6.6` | React i18n integration |
| `@novnc/novnc` | `^1.6.0` | VNC console frontend |
| `@xterm/xterm` | `^6.0.0` | Serial console frontend |

### Frontend and Test Tooling

| Package | Version | Purpose |
|---------|---------|---------|
| `typescript` | `^5` | Type checking |
| `eslint` | `^9.39.4` | Frontend linting |
| `vitest` | `^4.1.1` | Unit/component tests |
| `@vitest/coverage-v8` | `^4.1.1` | Coverage |
| `@testing-library/react` | `^16.3.2` | React component testing |
| `@testing-library/user-event` | `^14.6.1` | User-event simulation |
| `jsdom` | `^28.1.0` | DOM test environment |
| `@playwright/test` | `^1.58.2` | E2E/smoke tests |
| `knip` | `^6.0.5` | Frontend dependency/dead-code scan |

### Supplemental CI Tools

| Tool | Version | Purpose |
|------|---------|---------|
| `golang.org/x/vuln/cmd/govulncheck` | `v1.1.4` | Go vulnerability scan |
| `gitleaks` | `v8.28.0` | Secret scanning |
| `golangci-lint` | Custom binary from `.custom-gcl.yml` when present | Go lint plus shepherd architecture analyzers |
| `shepherd-lint` | Repository-local build | Custom go/analysis architecture checks |
| `promtool` | Installed from Ubuntu `prometheus` package in CI | Prometheus config, rule, and rule-test validation |

## Middleware Versions

| Middleware | Baseline |
|------------|----------|
| PostgreSQL | `18` |
| Kubernetes API series | `v1.34` / `k8s.io/* v0.34.3` |
| KubeVirt | `v1.8.2` |
| CDI API | `v1.64.0` |

## Configuration Parameters

Defaults are defined in `internal/config/config.go` and shown in
`config/config.yaml.example`.

### Database Connection Pool

| Parameter | Default | Environment variable |
|-----------|---------|----------------------|
| `database.max_conns` | `50` | `DATABASE_MAX_CONNS` |
| `database.min_conns` | `5` | `DATABASE_MIN_CONNS` |
| `database.max_conn_lifetime` | `1h` | `DATABASE_MAX_CONN_LIFETIME` |
| `database.max_conn_idle_time` | `10m` | `DATABASE_MAX_CONN_IDLE_TIME` |

### Runtime Concurrency

| Parameter | Default | Environment variable |
|-----------|---------|----------------------|
| `k8s.operation_timeout` | `5m` | `K8S_OPERATION_TIMEOUT` |
| `river.max_workers` | `10` | `RIVER_MAX_WORKERS` |
| `worker.general_pool_size` | `100` | `WORKER_GENERAL_POOL_SIZE` |
| `worker.k8s_pool_size` | `50` | `WORKER_K8S_POOL_SIZE` |
| `observability.metrics_enabled` | `true` | `OBSERVABILITY_METRICS_ENABLED` |
| `observability.metrics_path` | `/metrics` | `OBSERVABILITY_METRICS_PATH` |
| `observability.database_metrics_enabled` | `true` | `OBSERVABILITY_DATABASE_METRICS_ENABLED` |
| `observability.database_metrics_timeout` | `2s` | `OBSERVABILITY_DATABASE_METRICS_TIMEOUT` |
| `observability.river_metrics_enabled` | `true` | `OBSERVABILITY_RIVER_METRICS_ENABLED` |
| `observability.river_metrics_timeout` | `2s` | `OBSERVABILITY_RIVER_METRICS_TIMEOUT` |
| `observability.business_metrics_enabled` | `true` | `OBSERVABILITY_BUSINESS_METRICS_ENABLED` |
| `observability.business_metrics_timeout` | `2s` | `OBSERVABILITY_BUSINESS_METRICS_TIMEOUT` |
| `observability.tracing_enabled` | `true` | `OBSERVABILITY_TRACING_ENABLED` |
| `observability.tracing_service_name` | `shepherd` | `OBSERVABILITY_TRACING_SERVICE_NAME` |
| `observability.tracing_exporter` | `otlp_http` | `OBSERVABILITY_TRACING_EXPORTER` |
| `observability.tracing_sample_ratio` | `0.10` | `OBSERVABILITY_TRACING_SAMPLE_RATIO` |
| `observability.tracing_shutdown_timeout` | `5s` | `OBSERVABILITY_TRACING_SHUTDOWN_TIMEOUT` |

## HPA Concurrency Constraints Required

River coordinates jobs globally through PostgreSQL row locking, but worker
counts are configured per process. V1 K8s write execution is bounded by River
queue `MaxWorkers`; per-cluster semaphores and hot reload remain deferred to
[RFC-0015](../rfc/RFC-0015-per-cluster-concurrency.md).

| Formula | Recommended upper limit | Reason |
|---------|--------------------------|--------|
| `HPA.maxReplicas * RIVER_MAX_WORKERS` | `<= 50` | Avoid exhausting PostgreSQL connections during job execution |

Tune these values conservatively for production. If a deployment uses PgBouncer
or different PostgreSQL pool sizing, document the local calculation in the
deployment runbook.

## go.mod Template

Use `go.mod` as the exact source of truth. This abbreviated template shows the
runtime-sensitive dependency families that must stay aligned:

```go
module kv-shepherd.io/shepherd

go 1.25.11

require (
    entgo.io/ent v0.14.6
    github.com/gin-gonic/gin v1.12.0
    github.com/jackc/pgx/v5 v5.9.2
    github.com/riverqueue/river v0.37.0
    github.com/oapi-codegen/oapi-codegen/v2 v2.5.1
    k8s.io/api v0.34.3
    k8s.io/apimachinery v0.34.3
    k8s.io/client-go v0.34.3
    kubevirt.io/api v1.8.2
    kubevirt.io/client-go v1.8.2
)

replace (
    k8s.io/api => k8s.io/api v0.34.3
    k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.34.3
    k8s.io/apimachinery => k8s.io/apimachinery v0.34.3
    k8s.io/client-go => k8s.io/client-go v0.34.3
    k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20250710124328-f3f2b991d03b
)
```

`build/api.mk` intentionally runs oapi-codegen `v2.5.1` for generated-code
checks even though `go.mod` carries the module at `v2.5.0`. Keep the generated
artifact check authoritative until the module pin is explicitly bumped.

## Version Upgrade Guide

### KubeVirt and Kubernetes

1. Check the target KubeVirt release notes and its `go.mod`.
2. Update `kubevirt.io/api`, `kubevirt.io/client-go`, and the matching
   `k8s.io/*` package set together.
3. Refresh the `replace` block if KubeVirt pins a newer Kubernetes series.
4. Run `go mod tidy`, `make generate`, `make ci-go-build`, and relevant provider
   tests.
5. Update this document and [CURRENT_STATE.md](./CURRENT_STATE.md).

### OpenAPI Tooling

1. Update `build/api.mk` tool versions.
2. Regenerate API artifacts with `make api-generate`.
3. Run `REQUIRE_OPENAPI_COMPAT=1 make api-check`.
4. Update this document if generated output or compatibility behavior changes.

### Frontend

1. Update `web/package.json`.
2. Run `npm install --prefix web` or `npm ci --prefix web` as appropriate.
3. Run `npm run typecheck --prefix web`, `npm run test:run --prefix web`, and
   `npm run build --prefix web`.
4. Update this document if major framework or test-tool versions change.
