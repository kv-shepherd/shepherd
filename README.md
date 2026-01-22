# KubeVirt Shepherd

[![Licensed under Apache License version 2.0](https://img.shields.io/github/license/kv-shepherd/shepherd.svg)](https://www.apache.org/licenses/LICENSE-2.0)

**KubeVirt Shepherd** is a governance platform for [KubeVirt][kubevirt] virtual 
machines. It enables self-service VM lifecycle management with proper approval 
workflows and audit controls across multiple clusters.

> *Like a shepherd tending a flock, this platform ensures that VMs are properly 
> managed throughout their lifecycle — users enjoy self-service freedom while 
> governance policies prevent resource sprawl and orphaned instances.*

## Governance Model

```
System (Business Line) → Service (Application) → VM Instance
```

| Layer | Example | Self-Service | Approval | Audit |
|-------|---------|--------------|----------|-------|
| System | `demo`, `shop` | ✅ | No | ✅ |
| Service | `redis`, `mysql` | ✅ | No | ✅ |
| VM | `redis-06` | ✅ | **Required** | ✅ |

## Key Capabilities

- **Approval Workflow**: Structured request and approval for VM provisioning
- **Lifecycle Operations**: Start, stop, snapshot, clone, migrate (via KubeVirt)
- **Multi-Cluster**: Manage VMs across multiple Kubernetes clusters
- **Environment Isolation**: Strict separation between test and production
- **Audit Trail**: Complete operation history for compliance

## Design Principles

| Principle | Description |
|-----------|-------------|
| **Governance First** | This is a governance platform, not a scheduling platform. Reliability over speed. |
| **Eventually Consistent** | Batch operations complete reliably via queue processing, not aggressively in parallel. |
| **PostgreSQL Only** | Single database dependency (PostgreSQL 18+). No Redis, no external message queues. |
| **Async by Default** | All write operations return `202 Accepted` and execute asynchronously. |

## Project Status

> ⚠️ **Pre-Alpha**: Planning and design phase.

- [x] Architecture Decision Records
- [x] Implementation specifications  
- [ ] Core implementation

## Community

- [GitHub Issues][issues] - Bug reports and feature requests
- [Contributing](CONTRIBUTING.md) - How to contribute
- [Code of Conduct](CODE_OF_CONDUCT.md) - Community standards
- [Governance](GOVERNANCE.md) - Project governance
- [Security](SECURITY.md) - Security policy

## License

Apache License 2.0. See [LICENSE](LICENSE).

    Copyright The KubeVirt Shepherd Authors.

[kubevirt]: https://kubevirt.io
[issues]: https://github.com/kv-shepherd/shepherd/issues
