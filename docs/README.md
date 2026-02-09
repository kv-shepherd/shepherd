# KubeVirt Shepherd Documentation

> This directory contains all project documentation organized following open source best practices.

---

## Directory Structure

```
docs/
├── README.md                 # This file
├── adr/                      # Architecture Decision Records
│   ├── README.md             # ADR index with status table
│   ├── GLOSSARY.md           # Technical terminology
│   └── ADR-0001 ~ ADR-0030   # Individual ADRs (superseded ones remain here)
│
├── rfc/                      # Request for Comments (Future Features)
│   ├── README.md             # RFC index with priorities
│   └── RFC-0001 ~ RFC-0018   # Individual RFCs
│
└── design/                   # Implementation Design
    ├── README.md             # Core Go Refactor Project overview
    ├── interaction-flows/    # Canonical interaction outcomes
    │   ├── README.md
    │   └── master-flow.md
    ├── DEPENDENCIES.md       # Version pinning (single source of truth)
    ├── CHECKLIST.md          # Acceptance criteria (dashboard)
    ├── phases/               # Implementation phase specifications
    │   ├── 00-prerequisites.md
    │   ├── 01-contracts.md
    │   ├── 02-providers.md
    │   ├── 03-service-layer.md
    │   └── 04-governance.md
    ├── checklist/            # Per-phase acceptance checklists
    │   ├── README.md
    │   ├── phase-0-checklist.md
    │   ├── phase-1-checklist.md
    │   ├── phase-2-checklist.md
    │   ├── phase-3-checklist.md
    │   └── phase-4-checklist.md
    ├── examples/             # Reference implementations
    │   ├── README.md
    │   ├── config/
    │   ├── domain/
    │   ├── infrastructure/
    │   ├── handlers/
    │   ├── provider/
    │   ├── usecase/
    │   └── worker/
    ├── database/             # Database reference layer
    │   └── README.md
    ├── frontend/             # Frontend specification layer
    │   └── README.md
    ├── notes/                # Design notes (pre-ADR)
    │   └── README.md
    ├── traceability/         # Machine-readable traceability manifest (ADR-0032)
    │   └── master-flow.json
    └── ci/                   # CI check scripts
        ├── README.md         # Script index
        └── scripts/          # Check scripts
│
├── i18n/                     # Translation docs mirror
│   └── README.md
```

---

## Quick Navigation

### ⚡ 5-Minute Quick Start

> **New to this project?** Read these 3 sections first (~5 min total):
>
> 1. **[design/interaction-flows/master-flow.md](./design/interaction-flows/master-flow.md)** (product interaction source of truth)
> 2. **[design/README.md → Architecture Overview](./design/README.md#architecture-overview)** (request flow diagram)
> 3. **[adr/README.md → Reading Order](./adr/README.md#reading-order)** (which ADRs to read)
>
> After this, you'll understand: **PostgreSQL-only stack**, **async-first writes**, **Ent + sqlc hybrid transactions**.

### Implementation Guide

**Recommended reading order for implementing this project:**

1. **[design/DEPENDENCIES.md](./design/DEPENDENCIES.md)** - Understand version constraints FIRST (single source of truth)
2. **[adr/README.md](./adr/README.md)** - Follow the "Reading Order" section for architectural decisions
3. **[design/README.md](./design/README.md)** - Project overview and structure
4. **[design/README.md → Implementation Phases](./design/README.md#implementation-phases)** - Sequential implementation (00 → 01 → 02 → 03 → 04)
5. **[design/examples/README.md](./design/examples/README.md)** - Reference implementations
6. **[design/checklist/README.md](./design/checklist/README.md)** - Verification criteria for each phase

### For Architects

Start with [ADRs](./adr/README.md) to understand the architectural decisions:
1. [ADR-0003: Database ORM](./adr/ADR-0003-database-orm.md) - Core data persistence
2. [ADR-0006: Unified Async Model](./adr/ADR-0006-unified-async-model.md) - All writes are async
3. [ADR-0012: Hybrid Transaction](./adr/ADR-0012-hybrid-transaction.md) - Ent + sqlc atomicity

### For Developers

Start with the [Design](./design/README.md) directory:
1. [design/README.md](./design/README.md) - Project overview
2. [Phase 00](./design/phases/00-prerequisites.md) - Project setup
3. [Examples](./design/examples/README.md) - Reference implementations

### For Future Planning

Check [RFCs](./rfc/) for proposed features:
- [RFC Index](./rfc/README.md) - All future features with priorities

---

## Document Types

| Type | Location | Purpose |
|------|----------|---------|
| **ADR** | `adr/` | Immutable architectural decisions |
| **RFC** | `rfc/` | Proposed future features |
| **Design** | `design/` | Implementation specifications |

### ADR vs RFC Decision Guide

> **When to create which document type?**
>
> ```
> Question: Does this involve...
> 
> ├── Technology selection? (e.g., ORM, database, framework)
> │   └── ✅ Create ADR
> │
> ├── Architectural pattern? (e.g., async model, transaction strategy)
> │   └── ✅ Create ADR
> │
> ├── New user-facing feature? (e.g., VNC console, Helm export)
> │   └── ✅ Create RFC
> │
> ├── Performance optimization? (e.g., caching, partitioning)
> │   └── ✅ Create RFC (unless it changes architecture)
> │
> └── Implementation detail change? (same architecture)
>     └── ✅ Update Design docs only
> ```

---

## Contributing

When contributing documentation:

1. **New architectural decisions**: Create an ADR in `adr/`
2. **New feature proposals**: Create an RFC in `rfc/`
3. **Implementation details**: Update files in `design/`

See [CONTRIBUTING.md](../CONTRIBUTING.md) for detailed guidelines.

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [RELEASE.md](./RELEASE.md) | Release process and versioning |
| [CONTRIBUTING.md](../CONTRIBUTING.md) | Contribution guidelines |
| [SECURITY.md](../SECURITY.md) | Security policy |
