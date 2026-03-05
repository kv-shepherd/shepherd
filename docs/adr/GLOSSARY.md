# Glossary

> Technical terminology mapping for KubeVirt Shepherd project.
> This glossary serves as the authoritative source for consistent terminology usage.

---

## Architecture & Design

| Term | Description |
|------|-------------|
| Architecture Decision Record (ADR) | Document capturing significant architectural decisions |
| Request for Comments (RFC) | Proposal for future features or changes |
| Domain Event | Event representing a significant business occurrence |
| Aggregate | Domain-driven design concept for consistency boundary |
| Unit of Work | Pattern for tracking changes in a business transaction |
| Server-Side Apply (SSA) | K8s declarative resource management approach |

---

## Database & Persistence

| Term | Description |
|------|-------------|
| ORM (Object-Relational Mapping) | Technique for converting between incompatible type systems |
| Transaction | Atomic unit of database operations |
| ACID | Database transaction properties |
| Connection Pool | Pre-established database connections for reuse |
| Advisory Lock | PostgreSQL cooperative locking mechanism |
| Dead Tuple | PostgreSQL term for outdated row versions |
| Autovacuum | PostgreSQL automatic maintenance process |

---

## Asynchronous Processing

| Term | Description |
|------|-------------|
| Job Queue | System for asynchronous task execution |
| Worker | Process that executes queued jobs |
| Idempotency | Property where multiple executions produce same result |
| Compensation | Recovery mechanism for failed operations |
| Orphan Event | Event without corresponding job in queue |

---

## KubeVirt & Kubernetes

| Term | Description |
|------|-------------|
| Virtual Machine (VM) | Emulated computer system |
| VirtualMachineInstance (VMI) | Running VM in KubeVirt |
| Cluster | Group of nodes running containerized applications |
| Namespace | Kubernetes resource isolation boundary |
| Feature Gate | Toggle for enabling/disabling features |
| Dry Run | Validation without actual execution |

---

## Governance & Workflow

| Term | Description |
|------|-------------|
| Approval Ticket | Request requiring administrative approval |
| Tenant | Isolated organizational unit |
| Quota | Resource usage limit |
| Audit Log | Record of system activities |
| System | Top-level organizational grouping |
| Service | Logical grouping under a System |

---

## Infrastructure

| Term | Description |
|------|-------------|
| Provider | Interface abstraction for infrastructure operations |
| Template | Reusable configuration blueprint |
| Reconciler | Component ensuring desired vs actual state consistency |
| Health Check | System readiness verification |
| Rate Limiting | Request throttling mechanism |

---

## Status Values

| Value | Context |
|-------|---------|
| Pending | Awaiting action |
| Processing | Currently being handled |
| Completed | Successfully finished |
| Failed | Encountered error |
| Cancelled | Manually or automatically aborted |
| Approved | Governance approval granted |
| Rejected | Governance approval denied |

---

## ADR/RFC Status Values

| Value | Description |
|-------|-------------|
| Proposed | Under discussion |
| Accepted | Decision approved and active |
| Superseded | Replaced by newer decision |
| Deprecated | No longer recommended |
| Rejected | Not approved |
| Deferred | Postponed for future consideration |

---

## Abbreviations

| Abbreviation | Full Form |
|--------------|-----------|
| ADR | Architecture Decision Record |
| RFC | Request for Comments |
| DI | Dependency Injection |
| SSA | Server-Side Apply |
| CRUD | Create, Read, Update, Delete |
| DTO | Data Transfer Object |
| API | Application Programming Interface |
| CI | Continuous Integration |
| CD | Continuous Deployment |
