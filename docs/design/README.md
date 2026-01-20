# KubeVirt Shepherd - Core Go Refactor Project

> **Project Status**: Active  
> **Language**: Go  
> **Maintained by**: [CloudPasture](https://github.com/cloudpasture) Community

---

## Overview

This project implements the core Go backend for KubeVirt Shepherd, a governance platform for managing KubeVirt virtual machines across multiple clusters.

### Origin Statement

> 🌱 This is an **original design** created from scratch by CloudPasture community contributors.
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
├── phases/                   # Implementation phase specifications
│   ├── 00-prerequisites.md   # Project setup, toolchain
│   ├── 01-contracts.md       # Interface definitions, schemas
│   ├── 02-providers.md       # KubeVirt provider
│   ├── 03-service-layer.md   # Business logic, transactions
│   └── 04-governance.md      # Approval workflow, River Queue
├── checklist/                # Per-phase acceptance checklists
├── examples/                 # Reference implementations
│   ├── README.md             # Example index
│   ├── config/               # Configuration management
│   ├── infrastructure/       # Database connection pool
│   ├── worker/               # Worker pool pattern
│   ├── handlers/             # HTTP handlers
│   ├── domain/               # Domain models, events
│   ├── provider/             # Provider interfaces
│   └── usecase/              # Atomic transaction examples
└── ci/                       # CI check scripts
    ├── README.md             # Script index
    └── scripts/              # Check scripts
```

---

## Architecture Overview

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

| Pattern | Use Instead | CI Check |
|---------|-------------|----------|
| `import "gorm.io/gorm"` | Ent ORM | `check_no_gorm_import.go` |
| `import "github.com/redis/go-redis"` | PostgreSQL | `check_no_redis_import.sh` |
| `go func() { ... }()` | Worker Pool | `check_naked_goroutine.go` |
| Wire dependency injection | Manual DI | `check_manual_di.sh` |
| Self-built Outbox Worker | River Queue | `check_no_outbox_import.go` |
| sqlc in Service layer | sqlc only in UseCase | `check_sqlc_usage.sh` |
| K8s calls inside DB transaction | Two-phase pattern | `check_k8s_in_transaction.go` |

---

## Architecture Decisions

This project follows the architecture decisions documented in:

| ADR | Title | Status |
|-----|-------|--------|
| [ADR-0001](../../adr/ADR-0001-kubevirt-client.md) | KubeVirt Client Selection | Accepted |
| [ADR-0003](../../adr/ADR-0003-database-orm.md) | Database ORM Selection | Accepted |
| [ADR-0004](../../adr/ADR-0004-provider-interface.md) | Provider Interface Design | Accepted |
| [ADR-0006](../../adr/ADR-0006-unified-async-model.md) | Unified Async Model | Accepted |
| [ADR-0007](../../adr/ADR-0007-template-storage.md) | Template Storage | Accepted |
| [ADR-0008](../../adr/ADR-0008-postgresql-stability.md) | PostgreSQL Stability | Accepted |
| [ADR-0009](../../adr/ADR-0009-domain-event-pattern.md) | Domain Event Pattern | Accepted |
| [ADR-0011](../../adr/ADR-0011-ssa-apply-strategy.md) | SSA Apply Strategy | Accepted |
| [ADR-0012](../../adr/ADR-0012-hybrid-transaction.md) | Hybrid Transaction | Accepted |
| [ADR-0013](../../adr/ADR-0013-manual-di.md) | Manual DI | Accepted |
| [ADR-0014](../../adr/ADR-0014-capability-detection.md) | Capability Detection | Accepted |

---

## Implementation Phases

| Phase | Title | Description | Status |
|-------|-------|-------------|--------|
| [Phase 00](./phases/00-prerequisites.md) | Prerequisites | Project setup, toolchain, CI | ⬜ |
| [Phase 01](./phases/01-contracts.md) | Contracts | Ent schemas, interfaces, DTOs | ⬜ |
| [Phase 02](./phases/02-providers.md) | Providers | KubeVirt provider, watcher | ⬜ |
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
| [domain/vm.go](./examples/domain/vm.go) | Domain models | - |
| [domain/event.go](./examples/domain/event.go) | Domain event pattern | ADR-0009 |
| [provider/interface.go](./examples/provider/interface.go) | Provider interfaces | ADR-0004 |
| [usecase/create_vm.go](./examples/usecase/create_vm.go) | Atomic transaction | ADR-0012 |

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
| [DEPENDENCIES.md](./DEPENDENCIES.md) | Version pinning (single source of truth) |
| [CHECKLIST.md](./CHECKLIST.md) | Acceptance criteria |
| [ci/README.md](./ci/README.md) | CI scripts index |
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
```

---

## Prohibited Patterns

| Pattern | Reason | CI Check |
|---------|--------|----------|
| GORM import | Use Ent only | `check_forbidden_imports.go` |
| Redis import | PostgreSQL only in V1 | `check_no_redis_import.sh` |
| Naked goroutines | Use worker pool | `check_naked_goroutine.go` |
| Wire import | Manual DI only | `check_manual_di.sh` |
| Outbox pattern | Use River directly | `check_no_outbox_import.go` |
| sqlc outside whitelist | Limited to specific dirs | `check_sqlc_usage.sh` |

---

## Getting Started

```bash
# Clone repository
git clone git@github.com:CloudPasture/kubevirt-shepherd.git
cd kubevirt-shepherd

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

- [ADR Directory](../../adr/) - Architecture decisions
- [RFC Directory](../../rfc/) - Future feature proposals
- [Glossary](../../adr/GLOSSARY.md) - Technical terminology
