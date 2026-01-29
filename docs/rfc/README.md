# Request for Comments (RFC)

> This directory contains RFCs for future features and enhancements.
>
> RFCs are proposals that have been evaluated but are not yet scheduled for implementation. Each RFC includes a trigger condition that indicates when it should be considered for implementation.

---

## Quick Reference

| ID | Title | Status | Priority | Trigger |
|----|-------|--------|----------|---------|
| [RFC-0001](./RFC-0001-pg-partman.md) | PostgreSQL Table Partitioning | Deferred | P2 | Daily jobs > 10M |
| [RFC-0002](./RFC-0002-temporal.md) | Temporal Workflow Integration | Deferred | P3 | Multi-level approval needed |
| [RFC-0003](./RFC-0003-helm-export.md) | Helm Chart Export | Deferred | P2 | User request |
| [RFC-0004](./RFC-0004-external-approval.md) | External Approval Systems | **Proposed** | **P1** | V1+ optional feature |
| [RFC-0005](./RFC-0005-event-archiving.md) | Physical Event Archiving ¹ | Deferred | P2 | DomainEvent table too large |
| [RFC-0006](./RFC-0006-hot-reload.md) | Configuration Admin API ² | Deferred | P2 | Dynamic config via API |
| [RFC-0007](./RFC-0007-redis-cache.md) | Redis Cache Support | Deferred | P3 | Cache miss causing bottleneck |
| [RFC-0008](./RFC-0008-extended-auth.md) | Extended Auth Providers ⁴ | Deferred | P2 | MFA or SAML 2.0 required |
| [RFC-0009](./RFC-0009-pgbouncer.md) | PgBouncer Dual Pool | Deferred | P3 | Enterprise deployment |
| [RFC-0010](./RFC-0010-observability.md) | Observability Stack | Deferred | P2 | Metrics/Tracing required |
| [RFC-0011](./RFC-0011-vnc-console.md) | VNC Console (noVNC) ³⁴ | Deferred | P2 | Browser VM access needed |
| [RFC-0012](./RFC-0012-kubevirt-advanced.md) | KubeVirt Advanced Features ³ | Deferred | P2 | Snapshot/Clone/Migration |
| [RFC-0013](./RFC-0013-vm-snapshot.md) | VM Snapshot ³ | Deferred | P2 | Backup/Restore needed |
| [RFC-0014](./RFC-0014-vm-clone.md) | VM Clone ³ | Deferred | P2 | Rapid VM duplication |
| [RFC-0015](./RFC-0015-per-cluster-concurrency.md) | Per-Cluster Concurrency | Deferred | P3 | Distributed semaphore needed |

> **Notes**:
> - ¹ Soft archiving (`archived_at` field) is implemented in Phase 4; this RFC covers physical archiving to separate tables
> - ² Basic hot-reload (log level, rate limits) is in Phase 0; this RFC covers API-based config changes
> - ³ Provider interfaces defined in Phase 1-2; this RFC covers full implementation
> - ⁴ **Scope reduced by ADR-0015**: Core functionality accepted in [ADR-0015](../adr/ADR-0015-governance-model-v2.md); RFC now covers only advanced features not in ADR. See individual RFC for details.

---

## Status Definitions

| Status | Description |
|--------|-------------|
| **Proposed** | Under active discussion |
| **Accepted** | Approved for implementation (moved to project backlog) |
| **Deferred** | Valuable but not currently prioritized |
| **Rejected** | Evaluated and declined |

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
4. If RFC becomes an architectural decision, create corresponding ADR

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
