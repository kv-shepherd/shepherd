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
| **Async by Default** | Operations with external side effects return `202 Accepted` and execute asynchronously via River; pure PostgreSQL transactional writes may remain synchronous for atomicity. |
| **Platform RBAC** | RBAC managed in PostgreSQL, not Kubernetes. Permission queries isolated from cluster control plane. |

## Decision Documents

- **ADRs** record accepted architectural decisions
- **Design Notes** capture proposed changes before ADR acceptance
- Normative design specs are updated only after ADRs are accepted

## Documentation Map

- [docs/README.md](docs/README.md) - documentation index
- [docs/adr/README.md](docs/adr/README.md) - ADR catalog and reading order
- [docs/design/interaction-flows/master-flow.md](docs/design/interaction-flows/master-flow.md) - canonical interaction flow
- [docs/design/README.md](docs/design/README.md) - implementation design index
- [docs/design/frontend/README.md](docs/design/frontend/README.md) - frontend design layer
- [docs/design/database/README.md](docs/design/database/README.md) - database design layer
- [docs/design/ci/README.md](docs/design/ci/README.md) - CI/governance checks

## Project Status

> ⚠️ **Pre-Alpha**: Planning and design phase.

- [x] Architecture Decision Records
- [x] Implementation specifications  
- [ ] Core implementation

## Local Development

Use the integrated Docker workflow to start/reset frontend, backend, and database together:

```bash
./start-dev.sh
```

- Web ingress: `http://localhost:3000`
- API direct (diagnostic): `http://localhost:8080`
- PostgreSQL: `localhost:5432`

See `docs/design/frontend/local-dev-docker.md` for topology and workflow details.

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
