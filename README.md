# KubeVirt Shepherd

[![Licensed under Apache License version 2.0](https://img.shields.io/github/license/kv-shepherd/shepherd.svg)](https://www.apache.org/licenses/LICENSE-2.0)
[![Go Version](https://img.shields.io/github/go-mod/go-version/kv-shepherd/shepherd)](go.mod)
[![CI](https://github.com/kv-shepherd/shepherd/actions/workflows/ci.yml/badge.svg)](https://github.com/kv-shepherd/shepherd/actions/workflows/ci.yml)

**[Website](https://www.kv-shepherd.io)** ·
**[Online Demo](https://demo.kv-shepherd.io)** ·
**[Documentation](docs/README.md)**

**KubeVirt Shepherd** is a governance-first management platform for
[KubeVirt][kubevirt] virtual machines. It provides self-service VM lifecycle
management with structured approval workflows, RBAC, and full audit trails
across multiple Kubernetes clusters.

> *Like a shepherd tending a flock, this platform ensures that VMs are properly
> managed throughout their lifecycle — users enjoy self-service freedom while
> governance policies prevent resource sprawl and orphaned instances.*

<p align="center">
  <img src="docs/assets/screenshot.png" alt="KubeVirt Shepherd Dashboard" width="800">
</p>

## Why Shepherd?

KubeVirt solves *"running VMs on Kubernetes"*. Shepherd solves what comes next:
**who** can request a VM, **who** approves it, how are **quotas** enforced,
and where is the **audit trail**?

Today, these governance capabilities are mainly available through Red Hat's
commercial stack (OpenShift Virtualization + RHACM). Shepherd provides an
**open-source, vendor-neutral alternative** with contract-first API design,
declarative schema migrations, structured ADR governance, and enforced CI
gate checks.

| Capability | OpenShift Virt | Shepherd |
|------------|---------------|----------|
| Multi-cluster management | ✔ (requires RHACM) | ✔ Native |
| Approval workflows | ✔ | ✔ Built-in |
| Self-service portal | Operator-driven | ✔ Request → Approve → Deliver |
| Audit trail | OpenShift-integrated | ✔ Platform-native |
| Vendor lock-in | Strong (OpenShift) | None |

## Features

- **Approval Workflow** — Structured request and multi-level approval for VM lifecycle operations
- **VM Operations** — Create, modify, start, stop, restart, and delete through governed workflows
- **Multi-Cluster** — Unified management across Kubernetes clusters
- **Dual-Layer Access Control** — Platform-facing RBAC for global capabilities, plus System membership that inherits down to Services and VMs
- **Environment-Scoped Bindings** — Global role bindings can be limited to approved environments such as test and prod
- **Audit Trail** — Complete operation history for every resource change
- **VM Console Access** — VNC and serial console access with approval-aware entrypoints
- **i18n** — Chinese/English UI, extensible
- **Auth Plugin SDK** — Pluggable authentication (LDAP, OIDC, custom)

## Architecture

```
Web UI (React 19 + Next.js 16)  ──REST──▶  Go Backend (Gin)  ──▶  PostgreSQL 18
                                                               ──▶  KubeVirt clusters (client-go)
```

**Design principles:** PostgreSQL-only (no Redis / external MQ), async-first
via River Queue, contract-first API (OpenAPI), 53 ADRs governing all technical
decisions. See [docs/design/](docs/design/) for details.

## Project Status

> ⚠️ **Alpha** — The core governance capabilities — approval workflows, RBAC,
> audit trails, and VM lifecycle management — have been validated through
> internal production use within a financial-services team. We consider these
> core functions production-capable.
>
> The Alpha designation reflects a conservative assessment for external
> adopters: every environment is different, and UX refinements, peripheral
> features, and operational tooling are still being iterated. We label the
> project Alpha not because the platform is unstable, but because broader
> community feedback is needed to validate it across diverse environments.
>
> **We welcome your feedback** — bug reports, feature requests, and usage
> experiences all help raise the project's maturity. Please open an
> [Issue](https://github.com/kv-shepherd/shepherd/issues).

See [CHANGELOG.md](CHANGELOG.md) for release details.

## Quick Start

### Local Development

Use the local source-based workflow for actual development work.

```bash
# Start all services (frontend + backend + database)
./start-dev.sh

# Start from a clean local database
./start-dev.sh --clean-all

# Or build from source
make generate   # Ent ORM + OpenAPI + sqlc code generation
make build      # Go binary → bin/shepherd
make docker     # Docker image → kubevirt-shepherd:latest

# Frontend
cd web && npm ci && npm run dev
```

| Endpoint | URL |
|----------|-----|
| Web UI | `http://localhost:3000` |
| API | `http://localhost:8080` |

Local development seeds only the platform bootstrap baseline:
- built-in roles
- default admin account: `admin / admin`
- first sign-in requires a password change

Local dev preserves the existing database by default. Use `--clean-all` only
when you intentionally want a fresh local environment.

### GitHub Codespaces

Use Codespaces as a browser-based technical entry point for the real product.
It builds the current source tree, boots the full platform stack, and seeds
sample data so contributors and KubeVirt community users can inspect and debug
the running system directly.

[![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://codespaces.new/kv-shepherd/shepherd?quickstart=1)

#### Try it in Codespaces

1. Click **Open in GitHub Codespaces**.
2. Wait for the container bootstrap to finish.
3. Open the forwarded **Shepherd UI** port if the browser does not open automatically.
4. Sign in with one of the seeded accounts:
   - `admin / admin`
   - `test / test`

Codespaces behavior:

- first create builds `server` and `web` from the checked-out source tree
- the first bootstrap starts from a clean database and seeds baseline data plus extended experience fixtures
- later Codespace restarts reuse the existing data, and automatically rebuild `server` / `web` when the checked-out runtime source changes
- the helper script still supports an explicit rebuild path when you want fresh images without wiping seeded data
- the first start can still take a few minutes because the source images are built and the seed fixtures are loaded inside the Codespace

Default Codespaces sign-in:
- `admin / admin`
- `test / test`
- Codespaces starts with seeded product data, but it does **not** assume a live K8s/KubeVirt cluster is available. VM create, power, modify, delete, and console flows depend on a real cluster connection and can fail with normal availability or cluster-health errors until one is configured.

### Prerequisites

Go 1.25+ · Node.js 22+ · PostgreSQL 18+ · Docker 24+

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full development setup.

## Deployment

Shepherd provides a Docker Compose-based production topology (`server` + `web`
+ `nginx` + optional bundled PostgreSQL) with a one-click deploy script:

```bash
bash deploy/prod/deploy-prod.sh --with-seed   # first deploy
```

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the full deployment guide,
configuration reference, security checklist, and VPS experience seed setup.

## Documentation

- [Website](https://www.kv-shepherd.io) — Project overview and getting started
- [docs/README.md](docs/README.md) — Documentation index & 5-minute quick start
- [docs/adr/](docs/adr/) — 53 Architecture Decision Records
- [docs/design/](docs/design/) — Implementation specifications
- [docs/RELEASE.md](docs/RELEASE.md) — Release process
- [ROADMAP.md](ROADMAP.md) — Project roadmap
- [ADOPTERS.md](ADOPTERS.md) — Who is using Shepherd

## Community

We welcome **all forms of feedback** — bug reports, feature suggestions, usage
stories, and governance ideas. Your input helps shape the project direction.

- [GitHub Issues][issues] — Bug reports and feature requests
- [Contributing](CONTRIBUTING.md) — PR workflow, CI gates, coding standards
- [Code of Conduct](CODE_OF_CONDUCT.md) — Community standards
- [Governance](GOVERNANCE.md) — Project governance
- [Security](SECURITY.md) — Vulnerability reporting
- [Adopters](ADOPTERS.md) — Organizations using Shepherd

## License

Apache License 2.0 — See [LICENSE](LICENSE).

    Copyright The KubeVirt Shepherd Authors.

[kubevirt]: https://kubevirt.io
[issues]: https://github.com/kv-shepherd/shepherd/issues
