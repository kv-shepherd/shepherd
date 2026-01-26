# Architecture Decision Records

> This directory contains Architecture Decision Records (ADRs) for the KubeVirt Shepherd project.
>
> ADRs are immutable records of significant architectural decisions. Once accepted, they should not be modified. If a decision changes, create a new ADR that supersedes the old one.

---

## Quick Reference

| ID | Title | Status | Superseded By |
|----|-------|--------|---------------|
| [ADR-0001](./ADR-0001-kubevirt-client.md) | KubeVirt Client Selection | Accepted | - |
| [ADR-0002](./ADR-0002-git-library.md) | Git Library Selection | Superseded | ADR-0007 |
| [ADR-0003](./ADR-0003-database-orm.md) | Database ORM Selection | Accepted | - |
| [ADR-0004](./ADR-0004-provider-interface.md) | Provider Interface Design | Accepted | - |
| [ADR-0005](./ADR-0005-workflow-extensibility.md) | Workflow Extensibility | Accepted | - |
| [ADR-0006](./ADR-0006-unified-async-model.md) | Unified Async Model | Accepted | - |
| [ADR-0007](./ADR-0007-template-storage.md) | Template Storage | Accepted | - |
| [ADR-0008](./ADR-0008-postgresql-stability.md) | PostgreSQL Stability | Accepted | - |
| [ADR-0009](./ADR-0009-domain-event-pattern.md) | Domain Event Pattern | **Accepted** ¹ | - |
| [ADR-0010](./ADR-0010-transaction-strategy.md) | Transaction Strategy | Superseded | ADR-0012 |
| [ADR-0011](./ADR-0011-ssa-apply-strategy.md) | SSA Apply Strategy | Accepted | - |
| [ADR-0012](./ADR-0012-hybrid-transaction.md) | Hybrid Transaction Strategy | Accepted | - |
| [ADR-0013](./ADR-0013-manual-di.md) | Manual Dependency Injection | Accepted | - |
| [ADR-0014](./ADR-0014-capability-detection.md) | KubeVirt Capability Detection | Accepted | - |
| [ADR-0015](./ADR-0015-governance-model-v2.md) | Governance Model V2 | **Accepted** ² | - |
| [ADR-0016](./ADR-0016-go-module-vanity-import.md) | Go Module Vanity Import | Accepted | - |
| [ADR-0017](./ADR-0017-vm-request-flow-clarification.md) | VM Request and Approval Flow Clarification | **Proposed** | - |

> ⚠️ **¹ ADR-0009 Partial Supersession Notice**:
>
> | Section | Status | Action |
> |---------|--------|--------|
> | DomainEvent schema design | ✅ **Valid** | Must read and follow |
> | EventID pattern (Claim Check) | ✅ **Valid** | Must read and follow |
> | Payload immutability constraints | ✅ **Valid** | Must read and follow |
> | Worker fault tolerance patterns | ✅ **Valid** | Must read and follow |
> | Transaction strategy (eventual consistency) | ❌ **Obsolete** | Skip, see ADR-0012 instead |

> ⚠️ **² ADR-0015 Amendment Notice**:
>
> | Section | Status | Action |
> |---------|--------|--------|
> | §4 VMCreateRequest.ClusterID | ❌ **Incorrect** | See [ADR-0017](./ADR-0017-vm-request-flow-clarification.md) for correct definition |
> | All other sections | ✅ **Valid** | Must read and follow |

---

## Status Definitions

| Status | Description |
|--------|-------------|
| **Proposed** | Under discussion, not yet decided |
| **Accepted** | Decision is active and should be followed |
| **Superseded** | Replaced by a newer ADR (see "Superseded By" column) |
| **Deprecated** | No longer recommended, but not yet replaced |
| **Rejected** | Was proposed but not accepted |

---

## Reading Order

For newcomers, we recommend reading ADRs in this order:

### Foundation Layer
1. **ADR-0003** (Database ORM) → Core data persistence
2. **ADR-0001** (KubeVirt Client) → K8s/KubeVirt interaction

### Async & Transaction Layer  
3. **ADR-0006** (Unified Async) → All writes are async
4. **ADR-0012** (Hybrid Transaction) → Ent + sqlc atomicity

### Application Layer
5. **ADR-0004** (Provider Interface) → Infrastructure abstraction
6. **ADR-0007** (Template Storage) → Template management
7. **ADR-0011** (SSA Apply) → K8s resource submission
8. **ADR-0014** (Capability Detection) → Multi-cluster compatibility

### Historical Context
- **ADR-0002** → Why we moved from Git storage to DB (Superseded by ADR-0007)
- **ADR-0009** → DomainEvent pattern concepts (still valid), transaction section superseded by ADR-0012
- **ADR-0010** → Original eventual consistency model (Superseded by ADR-0012)

---

## Creating New ADRs

Use the following template for new ADRs:

```markdown
# ADR-NNNN: Title

> **Status**: Proposed  
> **Date**: YYYY-MM-DD  
> **Supersedes**: ADR-XXXX (if applicable)

## Context

[Describe the problem and constraints]

## Decision

[State the decision clearly]

## Consequences

### Positive
- [Benefit 1]
- [Benefit 2]

### Negative
- [Drawback 1]
- [Mitigation strategy]

## References

- [Related document](link)
```

---

## Related Resources

- [Glossary](./GLOSSARY.md) - Technical terminology
- [RFC Directory](../rfc/) - Future feature proposals
- [Core Go Project](../design/) - Implementation details
