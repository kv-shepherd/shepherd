# KubeVirt Shepherd Design

> **Project Status**: Active
> **Last audited**: 2026-04-24
> **Authority**: Implementation design layer under ADR-0030.

This directory contains the implementation design for KubeVirt Shepherd. Accepted
ADRs remain the decision source of truth; these design docs describe the current
implementation contracts, code organization, quality gates, and known gaps.

## Purpose

Use this directory when changing runtime behavior, HTTP contracts, frontend
workflows, database persistence, or CI governance. For current code-vs-design
alignment, start with [CURRENT_STATE.md](./CURRENT_STATE.md).

## Architecture Overview

```
React 19 + Next.js 16
        |
        | OpenAPI-generated TypeScript client
        v
Gin HTTP API + OpenAPI validator
        |
        | generated ServerInterface
        v
Handlers -> UseCases -> Services
        |        |         |
        |        |         +-- KubeVirt provider
        |        |             - official kubevirt client-go v1.8.1
        |        |             - dynamic-client SSA for desired-state submit
        |        |             - KubeVirt-native patch/dry-run for existing VM mutation
        |        |
        |        +-- Ent + sqlc + River InsertTx
        |
        v
PostgreSQL 18
        |
        +-- Ent tables
        +-- sqlc transaction tables
        +-- River queue tables
        +-- system_secrets bootstrap records
```

Current implementation facts:

| Area | Current state |
|------|---------------|
| Backend module | `kv-shepherd.io/shepherd` |
| Go baseline | Go `1.25.10` |
| Database | PostgreSQL 18, shared pgx pool for Ent + sqlc + River |
| HTTP contract | OpenAPI `3.1.0`, 135 operationIds |
| Ent schemas | 33 schema files |
| KubeVirt baseline | `kubevirt.io/client-go` `v1.8.1` |
| Frontend | React 19.2 + Next.js 16.2 + Ant Design 5 |
| Frontend route files | 29 App Router `page.tsx` files |

## ADR Governance

ADRs are immutable after acceptance. Do not edit accepted ADR text to reflect
implementation drift. Use this order instead:

1. If current implementation is compatible with accepted ADRs, update design
   docs and checklists.
2. If implementation clarifies an accepted decision without changing it, link to
   the later ADR or design note that already provides the clarification.
3. If the decision itself must change, create a new ADR that amends or
   supersedes the old one.

The current code-vs-design audit found no need for a new ADR. See
[CURRENT_STATE.md §ADR Assessment](./CURRENT_STATE.md#adr-assessment).

## Project Structure

```
docs/design/
├── README.md                 # This file
├── CURRENT_STATE.md          # Current code-vs-design alignment snapshot
├── DEFERRED_FOLLOWUPS.md     # Non-blocking production, environment, and V2 follow-ups
├── DEPENDENCIES.md           # Version pins and toolchain source of truth
├── CHECKLIST.md              # Acceptance dashboard and ADR constraint index
├── CODING_STYLE.md           # Go coding style guide
├── SECURITY_CODING.md        # Security coding guidance
├── interaction-flows/        # Canonical user-visible interaction outcomes
├── phases/                   # Phase-level implementation specifications
├── checklist/                # Per-phase verification checklists
├── database/                 # Persistence lifecycle and transaction rules
├── frontend/                 # Frontend design and testing rules
├── examples/                 # Reference implementation examples
├── notes/                    # Design notes paired with proposed ADRs
├── traceability/             # Machine-readable master-flow mapping
└── ci/                       # Governance and CI enforcement scripts
```

## Implementation Phases

| Phase | Specification | Current status |
|-------|---------------|----------------|
| Phase 0 | [00-prerequisites.md](./phases/00-prerequisites.md) | Complete for current toolchain and local/dev bootstrap |
| Phase 1 | [01-contracts.md](./phases/01-contracts.md) | Mostly complete: 33 Ent schemas, OpenAPI, Go/TS generation, core domain contracts |
| Phase 2 | [02-providers.md](./phases/02-providers.md) | Runtime provider baseline complete for V1. Snapshot/full clone/live migration remain RFC-backed future scope |
| Phase 3 | [03-service-layer.md](./phases/03-service-layer.md) | Core services, use cases, manual DI, and transaction boundaries implemented |
| Phase 4 | [04-governance.md](./phases/04-governance.md) | Governance baseline implemented: approval, audit, notifications, batch, VNC, cluster policy, status sync |
| Phase 5 | [05-auth-api-frontend.md](./phases/05-auth-api-frontend.md) | Product surface implemented for current scope: auth, external auth, generated API, frontend routes, and mock E2E coverage |

## Drift Decisions

| Topic | Current conclusion |
|-------|--------------------|
| Provider scope | Runtime wiring intentionally depends on narrower interfaces. Snapshot/full clone/live migration contracts remain future scope instead of pretending to be implemented. |
| VM status sync | ADR-0038 adaptive polling is authoritative. Watch/informer support is optional future acceleration only. |
| SSA implementation | Dynamic client + `types.ApplyPatchType` is the runtime SSA implementation. `controller-runtime` is not a runtime dependency. |
| Browser sessions | Shepherd uses JWT plus DB-bootstrapped signing/encryption secrets. Server-side session storage would require a new decision if made mandatory. |

## Technology Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.25.10 |
| HTTP | Gin |
| Database | PostgreSQL 18 |
| ORM | Ent |
| SQL generation | sqlc |
| Async queue | River |
| KubeVirt client | `kubevirt.io/client-go` |
| API contract | OpenAPI 3.1 + oapi-codegen + openapi-typescript |
| Frontend | React 19, Next.js 16, Ant Design 5, TanStack Query 5, Zustand 5 |

Detailed versions are pinned in [DEPENDENCIES.md](./DEPENDENCIES.md).

## Key Documents

| Document | Description |
|----------|-------------|
| [CURRENT_STATE.md](./CURRENT_STATE.md) | Current implementation snapshot and drift decisions |
| [DEFERRED_FOLLOWUPS.md](./DEFERRED_FOLLOWUPS.md) | Centralized non-blocking follow-ups moved out of phase checklists |
| [interaction-flows/master-flow.md](./interaction-flows/master-flow.md) | Product interaction source of truth |
| [database/README.md](./database/README.md) | Database reference layer: schema domains, retention, transaction boundaries |
| [DEPENDENCIES.md](./DEPENDENCIES.md) | Version pinning and toolchain source of truth |
| [CHECKLIST.md](./CHECKLIST.md) | Acceptance dashboard and core ADR constraints |
| [frontend/README.md](./frontend/README.md) | Frontend design docs index |
| [frontend/FRONTEND.md](./frontend/FRONTEND.md) | Frontend engineering baseline |
| [ci/README.md](./ci/README.md) | CI and governance gate documentation |
| [examples/README.md](./examples/README.md) | Reference implementation examples |

## Current Directory Structure

```
internal/
├── api/              # Generated API interface, handlers, middleware, validator
├── app/              # Manual DI composition root and modules
├── config/           # Config loading and defaults
├── domain/           # Domain models and event payloads
├── governance/       # Approval, audit, ticketing
├── infrastructure/   # Database clients and bootstrap security
├── jobs/             # River workers
├── provider/         # KubeVirt and auth-provider contracts/adapters
├── repository/sqlc/  # sqlc queries for transaction-critical paths
├── service/          # Business services
└── usecase/          # Transaction orchestration

web/
└── src/
    ├── app/          # Next.js App Router routes
    ├── components/   # Shared UI components
    ├── features/     # Feature controllers and components
    ├── hooks/        # Shared React hooks
    ├── i18n/         # en and zh-CN resources
    ├── lib/          # API client, auth, storage, utilities
    └── types/        # OpenAPI-generated TypeScript types
```

## Getting Started

```bash
# Start the local development stack
./start-dev.sh

# Regenerate Ent, OpenAPI, and sqlc artifacts
make generate

# Run the main governance/static checks
make ci-governance

# Run backend tests
make test

# Run frontend typecheck/tests/build
npm run typecheck --prefix web
npm run test:run --prefix web
npm run build --prefix web
```

## References

- [ADR Directory](../adr/README.md)
- [RFC Directory](../rfc/README.md)
- [Glossary](../adr/GLOSSARY.md)
