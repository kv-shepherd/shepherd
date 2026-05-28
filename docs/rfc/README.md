# Request for Comments (RFC)

> This directory contains RFCs for future features and enhancements.
>
> In this repository, RFCs are **feature/change proposals and backlog specs**,
> not immutable architecture authorities. Each RFC includes a trigger condition
> that indicates when it should be considered for implementation.

---

## Quick Reference

| ID | Title | Status | Priority | Trigger |
|----|-------|--------|----------|---------|
| [RFC-0001](./RFC-0001-pg-partman.md) | PostgreSQL Table Partitioning | Deferred | P2 | Daily jobs > 10M |
| [RFC-0002](./RFC-0002-temporal.md) | Temporal Workflow Integration | Deferred | P3 | Multi-level approval needed |
| [RFC-0003](./RFC-0003-helm-export.md) | Helm Chart Export | Deferred | P2 | User request |
| [RFC-0004](./RFC-0004-external-approval.md) | External Approval Systems | **Accepted** | **P1** | V1+ optional feature |
| [RFC-0005](./RFC-0005-event-archiving.md) | Physical Event Archiving | Deferred | P2 | DomainEvent table too large |
| [RFC-0006](./RFC-0006-hot-reload.md) | Configuration Admin API | Deferred | P2 | Dynamic config via API |
| [RFC-0007](./RFC-0007-redis-cache.md) | Redis Cache Support | Deferred | P3 | Cache miss causing bottleneck |
| [RFC-0008](./RFC-0008-extended-auth.md) | Extended Auth Providers | Deferred | P2 | MFA/SAML 2.0 or active token revocation required |
| [RFC-0009](./RFC-0009-pgbouncer.md) | PgBouncer Dual Pool | Deferred | P3 | Enterprise deployment |
| [RFC-0010](./RFC-0010-observability.md) | Observability Stack | Accepted for minimal metrics baseline; advanced scope deferred | P2 | Metrics/Tracing required |
| [RFC-0011](./RFC-0011-vnc-console.md) | VNC Console (noVNC) | Deferred | P2 | Embedded noVNC UX, active revoke, or session recording needed |
| [RFC-0012](./RFC-0012-kubevirt-advanced.md) | KubeVirt Advanced Features | Deferred | P2 | Snapshot/Clone/Migration |
| [RFC-0013](./RFC-0013-vm-snapshot.md) | VM Snapshot | Deferred | P2 | Backup/Restore needed |
| [RFC-0014](./RFC-0014-vm-clone.md) | VM Clone | Deferred | P2 | Rapid VM duplication |
| [RFC-0015](./RFC-0015-per-cluster-concurrency.md) | Per-Cluster Concurrency | Deferred | P3 | Distributed semaphore needed |
| [RFC-0016](./RFC-0016-key-rotation.md) | Secret Key Rotation | Proposed | P2 | Compliance or operator request |
| [RFC-0017](./RFC-0017-external-secret-provider.md) | External Secret Provider | Deferred | P2 | Enterprise KMS/Vault requirement |
| [RFC-0018](./RFC-0018-external-notification.md) | External Notification Channels | Deferred | P2 | V1 complete; Email/Webhook/Slack requested |
| [RFC-0019](./RFC-0019-kubevirt-instancetype-adapter.md) | KubeVirt Instancetype Adapter | Deferred | P2 | Instancetype/Preference import-export or cross-cluster migration needed |
| [RFC-0020](./RFC-0020-k8s-watch-acceleration.md) | Optional K8s Watch Acceleration | Deferred | P3 | Near-realtime VM drift visibility or read acceleration needed |
| [RFC-0021](./RFC-0021-preset-catalog-marketplace.md) | Preset Catalog Marketplace for Templates and Instance Sizes | Proposed | P2 | Durable import/export, shared catalog governance, or community preset distribution needed |
| [RFC-0022](./RFC-0022-architecture-aware-catalog-alignment.md) | Architecture-Aware Catalog Alignment for Templates, Instance Sizes, and Clusters | Deferred | P2 | First Arm64 rollout, heterogeneous cluster adoption, or architecture mismatch incidents |

## Implementation Reality Check (2026-05-07)

| RFC | Current State |
|-----|---------------|
| [RFC-0004](./RFC-0004-external-approval.md) | Provider-router, outbound webhook dispatch, registry schema, admin API/UI, runtime wiring, signed callback ingestion, and signed polling-mode ingestion are implemented; native ServiceNow/JIRA connectors remain future work. |
| [RFC-0005](./RFC-0005-event-archiving.md) | Soft archiving via `archived_at` is implemented; this RFC only covers physical archive tables and purge. |
| [RFC-0006](./RFC-0006-hot-reload.md) | Limited runtime hot-reload primitives exist; config admin API and multi-instance sync do not. |
| [RFC-0008](./RFC-0008-extended-auth.md) | Auth-provider admin/plugin-management foundation is implemented; MFA, SAML, and active session revocation remain deferred. |
| [RFC-0010](./RFC-0010-observability.md) | Runtime Prometheus metrics are accepted by ADR-0054; starter rules, rule tests, runbook-link checks, and Grafana assets are accepted by ADR-0055; optional monitoring deployment packaging is accepted by ADR-0056; default-off HTTP tracing and bounded HTTP/River correlation logs are accepted by ADR-0057; deep tracing, provider log correlation, custom business metrics, advanced alert routing, advanced SLOs, and advanced dashboards remain deferred. |
| [RFC-0011](./RFC-0011-vnc-console.md) | Stage 6 VNC baseline is implemented end-to-end; this RFC now tracks optional V2+ noVNC/proxy/session enhancements. |
| [RFC-0012](./RFC-0012-kubevirt-advanced.md), [RFC-0013](./RFC-0013-vm-snapshot.md), [RFC-0014](./RFC-0014-vm-clone.md) | Interfaces and domain types exist, but runtime Snapshot/Clone/Migration provider methods are still deferred. |
| [RFC-0016](./RFC-0016-key-rotation.md) | Verification-side signing-key compatibility exists; full keyring rotation and re-encryption workflow do not. |
| [RFC-0017](./RFC-0017-external-secret-provider.md) | `env`/DB bootstrap secret precedence is implemented; external Vault/KMS providers remain deferred. |
| [RFC-0018](./RFC-0018-external-notification.md) | Internal inbox notification flow is implemented; external email/webhook/slack channels remain deferred. |
| [RFC-0019](./RFC-0019-kubevirt-instancetype-adapter.md) | Optional instancetype/preference adapter remains fully deferred. |
| [RFC-0021](./RFC-0021-preset-catalog-marketplace.md) | Frontend built-in preset catalogs are emerging for admin forms; durable import/export and marketplace semantics remain deferred pending backend persistence. |
| [RFC-0022](./RFC-0022-architecture-aware-catalog-alignment.md) | Architecture-aware catalog alignment remains deferred; current rollout assumes pure x86 and avoids premature backend model expansion. |

---

## Status Definitions

| Status | Description |
|--------|-------------|
| **Proposed** | Under active discussion |
| **Accepted** | Feature direction approved for implementation planning/backlog work; does **not** override ADR authority |
| **Deferred** | Valuable but not currently prioritized |
| **Rejected** | Evaluated and declined |

---

## Directory Semantics

Use the documentation layers like this:

| Directory | Purpose | Authority |
|-----------|---------|-----------|
| `docs/adr/` | Accepted architecture decisions and decision history | **Highest** for architecture; accepted ADRs are immutable |
| `docs/rfc/` | Future feature proposals, optional capability specs, backlog candidates | Lower than ADR; may evolve until implemented or replaced |
| `docs/design/notes/` | Implementation notes and rollout details for accepted ADRs | Supports ADRs, not a replacement for them |

**Important rule**:
- If a feature proposal requires a new architectural decision, create or amend an
  **ADR**.
- If a feature is merely optional/deferred future work under existing ADRs, an
  **RFC** is appropriate.

This means the `rfc/` directory is **not** a misuse in this repository, but it
should be understood as a **feature backlog/spec directory**, not as an
alternative authority system parallel to ADRs.

---

## Priority Levels

| Priority | Description | Typical Timeline |
|----------|-------------|------------------|
| **P1** | Next release candidate | 1-3 months |
| **P2** | Mid-term planning | 3-12 months |
| **P3** | Long-term consideration | 12+ months |

---

## Promoting an RFC

When an RFC's trigger condition is met:

1. Update RFC status from `Deferred` to `Accepted`
2. Create implementation tasks in the relevant project
3. Link RFC to project CHECKLIST.md
4. If implementation requires a new architecture-level commitment, create corresponding ADR

---

## Creating New RFCs

Use the following template:

```markdown
# RFC-NNNN: Title

> **Status**: Proposed  
> **Priority**: P1 | P2 | P3  
> **Trigger**: [Condition that warrants implementation]

## Problem

[What problem does this solve?]

## Proposed Solution

[Technical approach]

## Trade-offs

### Pros
- [Benefit 1]

### Cons
- [Drawback 1]

## Implementation Notes

[High-level implementation guidance]

## References

- [Related ADR or external doc](link)
```

---

## Related Resources

- [ADR Directory](../adr/) - Architecture decisions
- [Core Go Project](../design/) - Implementation details
