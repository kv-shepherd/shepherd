# Interaction Flows

> **Status**: Draft (Pending ADR-0018 Acceptance)  
> **Source of Truth**: This directory contains the canonical interaction flows for Shepherd platform.

---

## Purpose

This directory serves as the **single source of truth** for all platform interaction flows, used by:

| Role | Usage |
|------|-------|
| **Frontend Developers** | UI/UX implementation reference |
| **Backend Developers** | API and database operation reference |
| **QA Engineers** | Test case design |
| **Architects** | System verification |

---

## Document Index

| Document | Description |
|----------|-------------|
| [master-flow.md](./master-flow.md) | **Canonical Version** - Complete interaction flow (English) |

### Translations

| Language | Location |
|----------|----------|
| 中文 (Chinese) | [i18n/zh-CN/design/interaction-flows/master-flow.md](../../i18n/zh-CN/design/interaction-flows/master-flow.md) |

> **Note**: The English version is the canonical source of truth. Translations are provided for convenience and may lag behind.

---

## Document Structure

```
Part 1: Platform Initialization
├── Stage 1: Developer Operations (Schema/Mask)
├── Stage 1.5: First Deployment Bootstrap (NEW)
├── Stage 2: Security Configuration (RBAC/Auth)
│   ├── Stage 2.A: Built-in Roles
│   ├── Stage 2.A+: Custom Role Management
│   ├── Stage 2.B: Authentication Configuration (OIDC/LDAP)
│   ├── Stage 2.C: IdP Group Mapping
│   ├── Stage 2.D: User Login Flow
│   └── Stage 2.E: External Approval Systems (NEW)
└── Stage 3: Admin Configuration (Cluster/InstanceSize/Template)

Part 2: Resource Management
├── Stage 4.A: Create System
├── Stage 4.A+: Resource-level Member Management
├── Stage 4.B: Create Service
└── Stage 4.C: Delete System/Service

Part 3: VM Lifecycle
├── Stage 5.A: VM Creation Request
├── Stage 5.B: Approval Workflow
├── Stage 5.C: VM Execution
└── Stage 5.D: VM Operation & Deletion

Part 4: State Machines & Data Models
├── State Transition Diagrams
├── Database Relationship Diagram
├── Permission Model (Dual-layer RBAC)
└── Audit Log Design
```

---

## Configuration Storage Strategy

> **2026-01-26 Update**: All runtime configuration is stored in PostgreSQL, managed via Web UI.

| Configuration Type | Storage | Management |
|--------------------|---------|------------|
| Infrastructure (DATABASE_URL, etc.) | config.yaml or env vars | Deployment-time |
| Auth Providers (OIDC/LDAP) | PostgreSQL | Web UI |
| External Approval Systems | PostgreSQL | Web UI |
| Clusters, InstanceSizes, Templates | PostgreSQL | Web UI |
| Users, Roles, Permissions | PostgreSQL | Web UI + IdP sync |

---

## Relationship to ADR-0018

This directory content is **extracted from** ADR-0018 Appendix.

| Source | Target |
|--------|--------|
| ADR-0018 Appendix | `master-flow.md` (canonical) |
| Translation | `i18n/zh-CN/.../master-flow.md` |

> **Note**: ADR-0018 appendix now contains only a summary with links to this directory.

---

## Version Control

| Date | Version | Change |
|------|---------|--------|
| 2026-01-26 | 0.1-draft | CNCF normalization: English as canonical, Chinese to i18n/ |
| 2026-01-26 | 0.1-draft | Added: Stage 1.5 Bootstrap, Stage 2.E External Approval Systems |
| 2026-01-26 | 0.1-draft | Updated: All runtime config via PostgreSQL (removed YAML config) |
| 2026-01-26 | 0.1-draft | Initial extraction from ADR-0018 |

---

## Related Documents

- [ADR-0018: Instance Size Abstraction](../../adr/ADR-0018-instance-size-abstraction.md) - Source ADR
- [ADR-0015: Governance Model V2](../../adr/ADR-0015-governance-model-v2.md) - Governance foundation
- [ADR-0017: VM Request Flow](../../adr/ADR-0017-vm-request-flow-clarification.md) - Request flow clarification
- [i18n/README.md](../../i18n/README.md) - Internationalization guide
