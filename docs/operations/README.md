# Operations Guides

This directory contains operator-facing procedures for running Shepherd outside
local development.

| Guide | Purpose |
|-------|---------|
| [production-deployment.md](./production-deployment.md) | Production Compose deployment, monitoring overlay, upgrade, rollback, and go-live checks |
| [live-e2e-validation.md](./live-e2e-validation.md) | Real backend and real KubeVirt live E2E validation procedure |
| [database-operations.md](./database-operations.md) | PostgreSQL and River table maintenance requirements |
| [platform-admin-sop.md](./platform-admin-sop.md) | Initial platform-admin handoff and recurring access review |
| [bootstrap-role-sop.md](./bootstrap-role-sop.md) | Compatibility pointer for the retired bootstrap-role procedure |

Operational procedures must not redefine accepted architecture decisions.
Reference ADRs and design docs for decision authority, and keep runbooks focused
on concrete commands, required inputs, evidence, and rollback paths.
