# KubeVirt Shepherd Roadmap

This roadmap describes the current project direction. It is intentionally
conservative: committed architecture decisions remain in `docs/adr/`, proposed
future capabilities remain in `docs/rfc/`, and this file only summarizes the
delivery order for public readers.

## Current Status

Shepherd is in Alpha. The core governance path has been validated through
internal production use:

- self-service VM request flow
- built-in approval workflow
- platform and resource RBAC
- audit logging
- PostgreSQL-native async execution with River
- KubeVirt VM create, power, modify, delete, manifest, and console entrypoints
- admin catalogs for clusters, namespaces, templates, instance sizes, auth
  providers, rate limits, roles, and users

The Alpha label reflects external-adopter caution, not known instability in the
core flow. More real-world feedback is needed across different KubeVirt
versions, storage classes, auth providers, and organization models.

## Near Term

- Keep public documentation aligned with the current monorepo structure.
- Finish live E2E validation for login, VM request, approval, delivery, power,
  delete, VNC/serial console, and directory-sync paths using the documented
  real-cluster validation SOP.
- Keep deployment documentation, monitoring packaging, and production upgrade
  guidance aligned with the operations runbooks.
- Continue API contract checks, generated type sync, and design-governance
  gates as blocking CI.

## V1 Focus

- Preserve the PostgreSQL-only runtime baseline: no Redis or external message
  queue dependency.
- Keep OpenAPI as the source of truth for backend and frontend contracts.
- Keep KubeVirt provider behavior aligned with KubeVirt `v1.8.x` and
  Kubernetes `v1.34.x` dependencies.
- Treat ResourceVersion-aware adaptive polling as the authoritative VM status
  convergence path.
- Keep snapshot, full clone, live migration, external approval adapters, and
  advanced observability behind RFCs until their contracts are accepted. The
  runtime metrics baseline is accepted by ADR-0054. Starter Prometheus rules,
  rule tests, runbook-link checks, and Grafana dashboard assets are accepted by
  ADR-0055. Optional Compose and Prometheus Operator monitoring packaging is
  accepted by ADR-0056. Default-off OpenTelemetry HTTP tracing plus bounded HTTP
  and River worker correlation logs are accepted by ADR-0057. Live E2E
  completion evidence must use the ADR-0058 machine-readable evidence bundle.

## V2+ Candidates

Future work is tracked in `docs/rfc/`, including:

- external approval adapters
- event archiving and partitioning
- pgbouncer production topology
- VNC proxy hardening and optional active revocation controls
- VM snapshots
- VM clone workflows
- advanced KubeVirt operations
- optional watch acceleration
- preset catalog marketplace
- external notification channels

## Feedback

Please use GitHub Issues for bugs, feature requests, adoption notes, and design
feedback:

https://github.com/kv-shepherd/shepherd/issues
