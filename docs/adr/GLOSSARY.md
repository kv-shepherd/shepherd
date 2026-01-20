# Glossary

> Technical terminology mapping for KubeVirt Shepherd project.  
> This glossary serves as the authoritative source for consistent terminology usage.

---

## Architecture & Design

| English | 中文 | Description |
|---------|------|-------------|
| Architecture Decision Record (ADR) | 架构决策记录 | Document capturing significant architectural decisions |
| Request for Comments (RFC) | 功能提案 | Proposal for future features or changes |
| Domain Event | 领域事件 | Event representing a significant business occurrence |
| Aggregate | 聚合 | Domain-driven design concept for consistency boundary |
| Unit of Work | 工作单元 | Pattern for tracking changes in a business transaction |
| Server-Side Apply (SSA) | 服务端应用 | K8s declarative resource management approach |

---

## Database & Persistence

| English | 中文 | Description |
|---------|------|-------------|
| ORM (Object-Relational Mapping) | 对象关系映射 | Technique for converting between incompatible type systems |
| Transaction | 事务 | Atomic unit of database operations |
| ACID | 原子性/一致性/隔离性/持久性 | Database transaction properties |
| Connection Pool | 连接池 | Pre-established database connections for reuse |
| Advisory Lock | 咨询锁 | PostgreSQL cooperative locking mechanism |
| Dead Tuple | 死元组 | PostgreSQL term for outdated row versions |
| Autovacuum | 自动清理 | PostgreSQL automatic maintenance process |

---

## Asynchronous Processing

| English | 中文 | Description |
|---------|------|-------------|
| Job Queue | 任务队列 | System for asynchronous task execution |
| Worker | 工作者 | Process that executes queued jobs |
| Idempotency | 幂等性 | Property where multiple executions produce same result |
| Compensation | 补偿机制 | Recovery mechanism for failed operations |
| Orphan Event | 孤儿事件 | Event without corresponding job in queue |

---

## KubeVirt & Kubernetes

| English | 中文 | Description |
|---------|------|-------------|
| Virtual Machine (VM) | 虚拟机 | Emulated computer system |
| VirtualMachineInstance (VMI) | 虚拟机实例 | Running VM in KubeVirt |
| Cluster | 集群 | Group of nodes running containerized applications |
| Namespace | 命名空间 | Kubernetes resource isolation boundary |
| Feature Gate | 特性开关 | Toggle for enabling/disabling features |
| Dry Run | 试运行 | Validation without actual execution |

---

## Governance & Workflow

| English | 中文 | Description |
|---------|------|-------------|
| Approval Ticket | 审批工单 | Request requiring administrative approval |
| Tenant | 租户 | Isolated organizational unit |
| Quota | 配额 | Resource usage limit |
| Audit Log | 审计日志 | Record of system activities |
| System | 系统 | Top-level organizational grouping |
| Service | 服务 | Logical grouping under a System |

---

## Infrastructure

| English | 中文 | Description |
|---------|------|-------------|
| Provider | 提供者 | Interface abstraction for infrastructure operations |
| Template | 模板 | Reusable configuration blueprint |
| Reconciler | 调谐器 | Component ensuring desired vs actual state consistency |
| Health Check | 健康检查 | System readiness verification |
| Rate Limiting | 限流 | Request throttling mechanism |

---

## Status Values

| English | 中文 | Context |
|---------|------|---------|
| Pending | 待处理 | Awaiting action |
| Processing | 处理中 | Currently being handled |
| Completed | 已完成 | Successfully finished |
| Failed | 失败 | Encountered error |
| Cancelled | 已取消 | Manually or automatically aborted |
| Approved | 已批准 | Governance approval granted |
| Rejected | 已拒绝 | Governance approval denied |

---

## ADR/RFC Status Values

| English | 中文 | Description |
|---------|------|-------------|
| Proposed | 提议中 | Under discussion |
| Accepted | 已采纳 | Decision approved and active |
| Superseded | 已取代 | Replaced by newer decision |
| Deprecated | 已弃用 | No longer recommended |
| Rejected | 已拒绝 | Not approved |
| Deferred | 延后 | Postponed for future consideration |

---

## Abbreviations

| Abbreviation | Full Form | 中文 |
|--------------|-----------|------|
| ADR | Architecture Decision Record | 架构决策记录 |
| RFC | Request for Comments | 功能提案 |
| DI | Dependency Injection | 依赖注入 |
| SSA | Server-Side Apply | 服务端应用 |
| CRUD | Create, Read, Update, Delete | 增删改查 |
| DTO | Data Transfer Object | 数据传输对象 |
| API | Application Programming Interface | 应用程序接口 |
| CI | Continuous Integration | 持续集成 |
| CD | Continuous Deployment | 持续部署 |
