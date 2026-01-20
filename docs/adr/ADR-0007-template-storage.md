# ADR-0007: Template Storage - Pure Database Replacing Git

> **Status**: Accepted  
> **Date**: 2026-01-14  
> **Supersedes**: [ADR-0002](./ADR-0002-git-library.md)

---

## Decision

Remove Git dependency. Templates and system presets are stored in PostgreSQL.

```
┌─────────────────────────────────────────────────────────────────────┐
│                       Pure Database Approach                         │
│                                                                      │
│  Template table                   SystemTemplate table               │
│  ├── name                         ├── name                          │
│  ├── version                      ├── version                       │
│  ├── content (TEXT)               ├── content (TEXT)                │
│  ├── status (lifecycle)           ├── category                      │
│  ├── created_by                   ├── created_at                    │
│  └── created_at                   └── ...                           │
│                                                                      │
│  Change audit → AuditLog table                                       │
│  Version history → TemplateRevision table (optional)                 │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Context

### Problem Analysis

| Issue | Description |
|-------|-------------|
| **Complexity** | go-git OOM risk, CLI fallback, distributed lock |
| **Maintenance burden** | clone/pull/push, conflict handling, large files |
| **Usage frequency** | Templates "unchanged for years", over-engineered |
| **Code volume** | Git-related code ~500+ lines |

### Actual Requirements

- Template changes are extremely infrequent
- No external collaboration needs (no PR/Review workflow)
- AuditLog table already provides audit trail
- VMRevision table already provides version history

---

## Template Status Lifecycle

```
          ┌─────────┐
          │  DRAFT  │  ← Default for new templates
          │         │  → Allows editing, Dry-Run testing
          └────┬────┘  → Prohibits VM creation using it
               │
               │ Publish
               ▼
          ┌─────────┐
          │ ACTIVE  │  ← Only this status allows new VM creation
          │         │  → Only one Active per template name
          └────┬────┘
               │
               │ Deprecate
               ▼
          ┌──────────┐
          │DEPRECATED│  ← Existing VMs continue running
          │          │  → Prohibits new VM creation
          └────┬─────┘
               │
               │ Archive
               ▼
          ┌─────────┐
          │ARCHIVED │  ← Soft delete, audit history only
          └─────────┘
```

| Status | Editable | Create New VM | Existing VMs | Description |
|--------|----------|---------------|--------------|-------------|
| **draft** | ✅ | ❌ | N/A | Draft, can modify and test |
| **active** | ❌ | ✅ | ✅ | Published, only one per name |
| **deprecated** | ❌ | ❌ | ✅ | Deprecated, smooth transition |
| **archived** | ❌ | ❌ | ❌ | Archived, audit only |

---

## Removed Components

| Component | Status |
|-----------|--------|
| `GitAsyncService` | Removed |
| `GitSyncTaskHandler` | Removed |
| `go-git` dependency | Removed |
| Git distributed lock | Removed |

## Preserved Capabilities

| Capability | Implementation |
|------------|----------------|
| Version control | version field + TemplateRevision table |
| Audit trail | AuditLog table |
| Import/Export | API provides YAML export |
| Initialization | Import from files at startup (one-time) |

---

## Consequences

### Positive

- ✅ Reduced maintenance complexity
- ✅ Eliminated go-git related risks
- ✅ Unified data access layer
- ✅ Simplified deployment (no Git credentials needed)
- ✅ Immediate effect (no pull/push delay)
- ✅ Transaction safety (template update and VM creation in same transaction)

### Negative

- 🟡 Lost external Git collaboration (mitigated by import/export)
- 🟡 Lost PR/Review workflow (replaced by in-platform approval)

---

## Helm Export (Roadmap)

> **Roadmap**: See [RFC-0003 Helm Export](../rfc/RFC-0003-helm-export.md)
>
> **Trigger**: When users need to export templates as standard Helm Charts.

---

## References

- Removed dependency: `github.com/go-git/go-git/v5`
- [ADR-0011: SSA Apply Strategy](./ADR-0011-ssa-apply-strategy.md)
