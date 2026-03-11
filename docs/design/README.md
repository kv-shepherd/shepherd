# KubeVirt Shepherd - Core Go Refactor Project

> **Project Status**: Active  
> **Language**: Go  
> **Maintained by**: [KubeVirt Shepherd](https://github.com/kv-shepherd) Community

---

## Overview

This project implements the core Go backend for KubeVirt Shepherd, a governance platform for managing KubeVirt virtual machines across multiple clusters.

### Origin Statement

> 🌱 This is an **original design** created from scratch by KubeVirt Shepherd community contributors.
>
> Inspired by real-world governance challenges in Kubernetes/KubeVirt environments, this project represents a **complete rewrite** using cloud-native technologies (Go, Ent, River Queue), not a port or adaptation of any existing codebase.
>
> All code, design documents, and architecture decisions are **100% original work** licensed under Apache 2.0, with intellectual property belonging to the project contributors.

---

## Project Structure

```
docs/design/
├── README.md                 # This file
├── DEPENDENCIES.md           # Dependency versions (single source of truth)
├── CHECKLIST.md              # Acceptance criteria
├── interaction-flows/        # Canonical interaction outcomes (master-flow)
│   ├── README.md             # Interaction flow index
│   └── master-flow.md        # Product interaction source of truth
├── phases/                   # Implementation phase specifications
│   ├── 00-prerequisites.md   # Project setup, toolchain
│   ├── 01-contracts.md       # Interface definitions, schemas
│   ├── 02-providers.md       # KubeVirt provider
│   ├── 03-service-layer.md   # Business logic, transactions
│   └── 04-governance.md      # Approval workflow, River Queue
├── notes/                    # Proposed changes before ADR acceptance
│   └── README.md             # Design notes guide
├── database/                 # Database reference layer (schema/lifecycle/transactions/migrations)
│   ├── README.md             # Database docs index
│   ├── schema-catalog.md     # Canonical table groups and relationships
│   ├── lifecycle-retention.md # Hard delete and retention policy baseline
│   ├── transactions-consistency.md # Transaction and async consistency boundaries
│   └── migrations.md         # Atlas/River migration rules
├── checklist/                # Per-phase acceptance checklists
├── frontend/                 # Frontend design specifications (ADR-0030)
│   ├── README.md             # Frontend docs index
│   ├── FRONTEND.md           # Frontend engineering baseline
│   ├── architecture/         # Frontend architecture decisions
│   ├── features/             # Feature interaction and UX specs
│   ├── contracts/            # API/type contract integration rules
│   └── testing/              # Frontend testing and CI gates
├── examples/                 # Reference implementations
│   ├── README.md             # Example index
│   ├── config/               # Configuration management
│   ├── infrastructure/       # Database connection pool
│   ├── worker/               # Worker pool pattern
│   ├── handlers/             # HTTP handlers
│   ├── domain/               # Domain models, events
│   ├── provider/             # Provider interfaces
│   └── usecase/              # Atomic transaction examples
├── traceability/             # Traceability manifest (ADR-0032)
│   └── master-flow.json      # Master-flow stage mapping (machine-readable)
└── ci/                       # CI check scripts
    ├── README.md             # Script index
    └── scripts/              # Check scripts
```
---

## Architecture Overview

### System Component Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              KubeVirt Shepherd Architecture                          │
├─────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                      │
│  ┌──────────────────────────┐     ┌──────────────────────────────────────────────┐  │
│  │      External Clients    │     │                 Internal Layers              │  │
│  │  ─────────────────────── │     │  ─────────────────────────────────────────── │  │
│  │  • Web UI                │     │                                              │  │
│  │  • CLI                   │────►│  [Handler Layer] Gin HTTP Handlers           │  │
│  │  • API Clients           │     │         │ • Request validation               │  │
│  └──────────────────────────┘     │         │ • Dry-Run calls (sync)             │  │
│                                   │         ▼ • Returns 202 Accepted             │  │
│                                   │  [UseCase Layer] Transaction Orchestration   │  │
│                                   │         │ • sqlc for DomainEvent+River       │  │
│                                   │         │ • Ent for regular CRUD             │  │
│                                   │         ▼ • ADR-0012 atomic commit           │  │
│                                   │  [Service Layer] Business Logic              │  │
│                                   │         │ • No transaction control           │  │
│                                   │         │ • Calls Repository + Provider      │  │
│                                   │         ▼                                    │  │
│                                   │  [Repository Layer] Data Access              │  │
│                                   │         │ • Ent Client queries               │  │
│                                   │         ▼                                    │  │
│                                   └──────────────────────────────────────────────┘  │
│                                                       │                              │
│  ┌────────────────────────────────────────────────────┼──────────────────────────┐  │
│  │                         PostgreSQL 18 (Single Database)                       │  │
│  │  ───────────────────────────────────────────────────────────────────────────  │  │
│  │                                                                               │  │
│  │   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────────┐  │  │
│  │   │ Ent Tables  │   │ sqlc Tables │   │ River Tables│   │ Session Tables  │  │  │
│  │   │  • VM       │   │ • DomainEvent│  │ • river_job │   │ • sessions      │  │  │
│  │   │  • System   │   │ • Approval  │   │ • river_... │   │                 │  │  │
│  │   │  • Service  │   │   Ticket    │   │             │   │                 │  │  │
│  │   │  • Cluster  │   │             │   │             │   │                 │  │  │
│  │   └─────────────┘   └─────────────┘   └─────────────┘   └─────────────────┘  │  │
│  │                                                                               │  │
│  │   ◄─────────────────── Shared pgxpool (ADR-0012) ────────────────────────►   │  │
│  └───────────────────────────────────────────────────────────────────────────────┘  │
│                                           │                                          │
│                                           ▼                                          │
│  ┌───────────────────────────────────────────────────────────────────────────────┐  │
│  │                            River Worker Pool                                   │  │
│  │  ────────────────────────────────────────────────────────────────────────────  │  │
│  │   • Consumes jobs via FOR UPDATE SKIP LOCKED                                  │  │
│  │   • Executes K8s operations asynchronously                                    │  │
│  │   • RIVER_MAX_WORKERS per instance (globally coordinated via DB)              │  │
│  └─────────────────────────────────┬─────────────────────────────────────────────┘  │
│                                    │                                                 │
│                                    ▼                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────────┐  │
│  │                         KubeVirt Provider Layer                                │  │
│  │  ────────────────────────────────────────────────────────────────────────────  │  │
│  │   • SSA Apply (ADR-0011) with FieldOwner: kubevirt-shepherd                   │  │
│  │   • Multi-cluster credential management                                       │  │
│  │   • Adaptive VM status sync (ADR-0038 polling baseline)                       │  │
│  │   • Capability Detection (ADR-0014)                                           │  │
│  └─────────────────────────────────┬─────────────────────────────────────────────┘  │
│                                    │                                                 │
└────────────────────────────────────┼─────────────────────────────────────────────────┘
                                     │
                                     ▼
     ┌───────────────────────────────────────────────────────────────────────────────┐
     │                        Kubernetes Clusters (1..N)                              │
     │  ─────────────────────────────────────────────────────────────────────────────│
     │   Cluster A              Cluster B              Cluster C                     │
     │   ┌──────────────┐       ┌──────────────┐       ┌──────────────┐              │
     │   │ KubeVirt 1.7 │       │ KubeVirt 1.6 │       │ KubeVirt 1.7 │              │
     │   │   • VMs      │       │   • VMs      │       │   • VMs      │              │
     │   │   • VMIs     │       │   • VMIs     │       │   • VMIs     │              │
     │   └──────────────┘       └──────────────┘       └──────────────┘              │
     └───────────────────────────────────────────────────────────────────────────────┘
```

### Request Flow (ADR-0006)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Request Flow (ADR-0006)                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  User ──► Gin Handler ──► UseCase (TX) ──► DomainEvent + ApprovalTicket     │
│                               │                        │                     │
│                               │                        ▼                     │
│                               │              ┌─────────────────┐             │
│                               │              │ River Job Queue │             │
│                               │              └────────┬────────┘             │
│                               │                       │                      │
│                               ▼                       ▼                      │
│                          202 Accepted            River Worker               │
│                                                       │                      │
│                                                       ▼                      │
│                                              KubeVirt Provider               │
│                                                       │                      │
│                                                       ▼                      │
│                                                 K8s API Server               │
│                                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│  Database: PostgreSQL 18 (Ent + sqlc + River share pgxpool)                 │
│  Transaction: ADR-0012 Hybrid Atomic (sqlc for DomainEvent + River InsertTx)│
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## ⛔ Forbidden Patterns (CI Enforcement)

> **These patterns will cause CI to fail. No exceptions.**
>
> For the complete list of CI checks and ADR constraints, see [CHECKLIST.md §Core ADR Constraints](./CHECKLIST.md#core-adr-constraints-single-reference-point).

| Pattern | Use Instead |
|---------|-------------|
| `import "gorm.io/gorm"` | Ent ORM (ADR-0003) |
| `go func() { ... }()` | Worker Pool (ADR-0031) |
| Wire/fx DI | Manual DI (ADR-0013) |
| K8s calls inside DB transaction | Two-phase pattern (ADR-0012) |

---

## Architecture Decisions

All architecture decisions are documented in [docs/adr/README.md](../adr/README.md). For the authoritative list of **enforced constraints** with CI checks, see [CHECKLIST.md §Core ADR Constraints](./CHECKLIST.md#core-adr-constraints-single-reference-point).

---

### Proposed Changes (Pre-ADR)

Proposed changes are documented in `docs/design/notes/` until the ADR is accepted.
Do not update normative specs until acceptance.

---

## Implementation Phases

| Phase | Title | Description | Status |
|-------|-------|-------------|--------|
| [Phase 00](./phases/00-prerequisites.md) | Prerequisites | Project setup, toolchain, CI | ⬜ |
| [Phase 01](./phases/01-contracts.md) | Contracts | Ent schemas, interfaces, DTOs | ⬜ |
| [Phase 02](./phases/02-providers.md) | Providers | KubeVirt provider, provider-side sync boundary | ⬜ |
| [Phase 03](./phases/03-service-layer.md) | Service Layer | Business logic, transactions | ⬜ |
| [Phase 04](./phases/04-governance.md) | Governance | Approval workflow, River Queue | ⬜ |

---

## Code Examples

Reference implementations are in the [examples/](./examples/) directory:

| Example | Description | Related ADR |
|---------|-------------|-------------|
| [config/config.go](./examples/config/config.go) | Configuration management | - |
| [infrastructure/database.go](./examples/infrastructure/database.go) | Shared connection pool | ADR-0012 |
| [worker/pool.go](./examples/worker/pool.go) | Worker pool pattern | - |
| [handlers/health.go](./examples/handlers/health.go) | Health check endpoints | - |
| [domain/vm.go](./examples/domain/vm.go) | Domain models | ADR-0015 §3-4 |
| [domain/event.go](./examples/domain/event.go) | Domain event pattern | ADR-0009, ADR-0015 §6 |
| [provider/interface.go](./examples/provider/interface.go) | Provider interfaces | ADR-0004 |
| [usecase/create_vm.go](./examples/usecase/create_vm.go) | Atomic transaction | ADR-0012, ADR-0015 §3 |

> **Note (ADR-0016)**: All example code uses vanity import paths (`kv-shepherd.io/shepherd/...`).

---

## Technology Stack

| Component | Technology | Notes |
|-----------|------------|-------|
| Language | Go 1.25+ | |
| Database | PostgreSQL 18.x | |
| ORM | Ent | With Atlas migrations |
| Async Queue | River Queue | PostgreSQL-native |
| SQL Code Gen | sqlc | For atomic transactions |
| HTTP Framework | Gin | |
| KubeVirt Client | kubevirt.io/client-go | |
| K8s Client | controller-runtime | For SSA Apply |

---

## Key Documents

| Document | Description |
|----------|-------------|
| ⭐ **[master-flow.md](./interaction-flows/master-flow.md)** | **Single source of truth for product interaction outcomes** (data input/processing/output and user-visible behavior). |
| [database/README.md](./database/README.md) | **Database reference layer** (schema domains, retention lifecycle, transaction boundaries, migration policy) |
| [DEPENDENCIES.md](./DEPENDENCIES.md) | Version pinning (single source of truth for all dependency versions) |
| [CHECKLIST.md](./CHECKLIST.md) | Acceptance criteria and **Core ADR Constraints** (single source of truth for CI enforcement) |
| [CODING_STYLE.md](./CODING_STYLE.md) | Go coding style guide (file/function limits, import order, logging, naming conventions) |
| [SECURITY_CODING.md](./SECURITY_CODING.md) | Security coding practices (secrets, input validation, tenant isolation, RBAC) |
| [frontend/README.md](./frontend/README.md) | Frontend design docs index (required reading before frontend changes) |
| [frontend/FRONTEND.md](./frontend/FRONTEND.md) | Frontend engineering baseline (i18n, API types, schema fallback) |
| [ci/README.md](./ci/README.md) | **Authoritative CI/development governance** (quality gates, enforcement scripts, workflow requirements) |
| [examples/README.md](./examples/README.md) | Code examples index |

---

## Target Directory Structure

When implemented, the project will have this structure:

```
internal/
├── app/              # Composition Root (bootstrap.go)
├── config/           # Configuration management
├── domain/           # Domain models, events
├── governance/       # Approval, audit
│   ├── approval/
│   └── audit/
├── handler/          # HTTP handlers
├── infrastructure/   # Database, external clients
├── jobs/             # River job definitions
├── pkg/              # Internal shared packages
│   ├── errors/
│   ├── logger/
│   └── worker/
├── provider/         # KubeVirt provider
├── repository/       # Data access layer
│   └── sqlc/         # sqlc queries (limited scope)
├── service/          # Business services
└── usecase/          # Atomic transaction orchestration

web/                  # Frontend (ADR-0020, ADR-0027: Monorepo with web/)
├── src/
│   ├── components/   # React components
│   ├── pages/        # Next.js pages
│   ├── hooks/        # Custom React hooks
│   ├── lib/          # Utility functions
│   ├── i18n/         # Internationalization (react-i18next)
│   │   ├── index.ts          # i18next initialization
│   │   ├── config.ts         # Language configuration
│   │   └── locales/          # Translation files
│   │       ├── en/           # English (default)
│   │       │   ├── common.json     # Shared translations
│   │       │   ├── errors.json     # Error messages (from error codes)
│   │       │   ├── approval.json   # Approval workflow
│   │       │   └── vm.json         # VM management
│   │       └── zh-CN/        # Chinese Simplified
│   │           ├── common.json
│   │           ├── errors.json
│   │           ├── approval.json
│   │           └── vm.json
│   └── types/
│       └── api.gen.ts  # Generated from OpenAPI (ADR-0021)
├── public/
└── package.json
```


---

> For the complete list, see [CHECKLIST.md §Core ADR Constraints](./CHECKLIST.md#core-adr-constraints-single-reference-point).

---

## Getting Started

```bash
# Clone repository
git clone git@github.com:kv-shepherd/shepherd.git
cd shepherd

# Install dependencies
go mod download

# Generate Ent code
go generate ./ent/...

# Run migrations
atlas migrate apply --env local
river migrate-up --database-url $DATABASE_URL

# Seed initial data
SEED_ADMIN_PASSWORD=your_password go run ./cmd/seed

# Start development server
go run cmd/server/main.go
```

---

## References

- [ADR Directory](../adr/README.md) - Architecture decisions
- [RFC Directory](../rfc/README.md) - Future feature proposals
- [Glossary](../adr/GLOSSARY.md) - Technical terminology
