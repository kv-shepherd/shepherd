# KubeVirt Shepherd

[![Licensed under Apache License version 2.0](https://img.shields.io/github/license/kv-shepherd/shepherd.svg)](https://www.apache.org/licenses/LICENSE-2.0)
[![Go Version](https://img.shields.io/github/go-mod/go-version/kv-shepherd/shepherd)](go.mod)
[![CI](https://github.com/kv-shepherd/shepherd/actions/workflows/ci.yml/badge.svg)](https://github.com/kv-shepherd/shepherd/actions/workflows/ci.yml)

**[Website](https://kv-shepherd.io)** ·
**[Online Demo](https://demo.kv-shepherd.io)** ·
**[Documentation](docs/README.md)**

KubeVirt Shepherd is a governance-first management platform for
[KubeVirt][kubevirt] virtual machines. It provides self-service VM lifecycle
management with structured approval workflows, RBAC, quota-aware operating
models, and audit trails across multiple Kubernetes clusters.

<p align="center">
  <img src="docs/assets/screenshot.png" alt="KubeVirt Shepherd Dashboard" width="800">
</p>

## Why Shepherd?

KubeVirt solves running VMs on Kubernetes. Shepherd focuses on the operating
model around those VMs: who can request them, who approves the change, how
platform permissions are enforced, which clusters are available, and where the
audit trail lives.

Shepherd is an open-source, vendor-neutral alternative for teams that need VM
governance without tying the workflow to a specific commercial platform stack.

| Capability | OpenShift Virt | Shepherd |
|------------|----------------|----------|
| Multi-cluster management | Yes, commonly with RHACM | Native |
| Approval workflows | Yes | Built in |
| Self-service portal | Operator-driven | Request -> approve -> deliver |
| Audit trail | OpenShift-integrated | Platform-native |
| Vendor lock-in | Strong OpenShift coupling | None |

## Features

- Approval workflows for create, modify, start, stop, restart, and delete
- Multi-cluster Kubernetes/KubeVirt management
- Platform RBAC plus System, Service, and VM membership inheritance
- Environment-scoped global role bindings
- Full audit trail for governed resource changes
- Built-in administrator observability for API traces, audit-derived business
  signals, and operational metrics
- VNC and serial console entrypoints
- Chinese and English UI
- Auth provider plugin SDK for LDAP, OIDC, and custom integrations
- PostgreSQL-only runtime architecture with River Queue for async work

## Architecture

```text
Web UI (React 19 + Next.js 16)
  -> Go Backend (Gin)
  -> PostgreSQL 18
  -> Kubernetes/KubeVirt clusters
```

Shepherd intentionally avoids Redis and external message queues in the default
architecture. PostgreSQL stores business state, audit data, credentials, and
River background jobs. API changes are contract-first through OpenAPI, and
architecture decisions are tracked in [docs/adr/](docs/adr/).

## Project Status

Shepherd is currently **Alpha**. The core governance paths - approval workflows,
RBAC, audit trails, and VM lifecycle management - have been validated through
internal production use in a financial-services Kubernetes/KubeVirt
environment. The Alpha label is intentionally conservative while broader
external feedback is gathered across different clusters, storage classes, auth
providers, and operating policies.

See [CHANGELOG.md](CHANGELOG.md) for release details and
[ROADMAP.md](ROADMAP.md) for planned work.

## Quick Start

### Helm Deployment

Use this path for Kubernetes-native installs. Helm charts are maintained
separately in [kv-shepherd/helm-charts](https://github.com/kv-shepherd/helm-charts),
and the published chart uses public Docker Hub images by default.

Demo install without an Ingress controller or persistent database storage:

```bash
helm repo add shepherd https://kv-shepherd.github.io/helm-charts
helm repo update
helm upgrade --install shepherd shepherd/shepherd \
  --namespace shepherd --create-namespace \
  --set postgresql.persistence.enabled=false

kubectl -n shepherd port-forward svc/shepherd-edge 3443:443
```

Open `https://127.0.0.1:3443`. See the chart repository for NodePort/IP access,
Ingress with TLS, persistent PostgreSQL, external database values, and
managed-cluster RBAC examples. Clusters that run Prometheus Operator can apply
the public starter monitoring manifests from this repository:

```bash
kubectl -n shepherd apply \
  -f deploy/monitoring/prometheus-operator/shepherd-service-monitor.yml \
  -f deploy/monitoring/prometheus-operator/shepherd-prometheus-rule.yml
```

### Docker Compose From Release Images

Use this path for production or VPS installs. It uses GHCR release images and
does not require a git checkout on the target host.

Prerequisites: Docker, Docker Compose v2, and outbound access to GHCR and
GitHub release assets.

```bash
mkdir -p shepherd-deploy && cd shepherd-deploy
curl -fsSL https://raw.githubusercontent.com/kv-shepherd/shepherd/main/deploy/prod/deploy-prod.sh | \
  bash -s -- --release-images --with-seed
```

This starts the latest published GHCR images with bundled PostgreSQL 18 and a
local TLS endpoint. Generated secrets, bootstrap credentials, and resolved image
refs are persisted to `.env.prod`; back up that file.

Domain and external PostgreSQL inputs are optional. When needed, pass
`SERVER_PUBLIC_BASE_URL`, `DATABASE_URL`, and `DEPLOY_BUNDLED_POSTGRES=false`
before `bash`.

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for external PostgreSQL 18,
domain/TLS configuration, image version pinning, manual compose commands,
the built-in `/metrics` endpoint, the optional Prometheus/Tempo/OpenTelemetry
Collector monitoring overlay, and the security checklist.

### Local Development

Use this path when you want to modify Shepherd or run the full stack from the
current source tree.

```bash
git clone https://github.com/kv-shepherd/shepherd.git
cd shepherd
git pull --ff-only origin main
./start-dev.sh
```

Prerequisites: Git, Go 1.25.11 or newer, Node.js 22 or newer with npm, and
Docker 24 or newer with Docker Compose v2.

Default local endpoints:

- Web UI: `http://localhost:3000`
- HTTPS dev ingress: `https://localhost:3443`
- API: `http://localhost:3000/api/v1`

### Docker Compose From Source Build

Use source-build deployment only when you intentionally want the target host to
build local backend and frontend images from a checkout.

```bash
git clone https://github.com/kv-shepherd/shepherd.git
cd shepherd
git pull --ff-only origin main
bash deploy/prod/deploy-prod.sh --source-build --with-seed
```

Both Docker Compose paths use `deploy-prod.sh`, but the mode flag is deliberate:
`--release-images` runs published images and can be used from `curl`; `--source-build`
requires a git checkout and builds local images before starting the stack. See
[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the full deployment guide,
configuration reference, security checklist, and VPS experience seed setup.

## Documentation

- [docs/README.md](docs/README.md) - Documentation index and quick start
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) - Deployment guide and security checklist
- [docs/adr/](docs/adr/) - Architecture Decision Records
- [docs/design/](docs/design/) - Design and implementation notes
- [docs/RELEASE.md](docs/RELEASE.md) - Release process
- [ADOPTERS.md](ADOPTERS.md) - Organizations using Shepherd

## Community

Feedback from real environments is especially useful while the project is in
Alpha.

- [Official Website](https://kv-shepherd.io) - Project homepage and feature overview
- [Discord](https://discord.gg/9P2wtpPMUe) - Chat with the community
- [GitHub Issues][issues] - Bug reports and feature requests
- [GitHub Discussions](https://github.com/kv-shepherd/shepherd/discussions) - Questions and ideas
- [Contributing](CONTRIBUTING.md) - PR workflow, CI gates, and coding standards
- [Code of Conduct](CODE_OF_CONDUCT.md) - Community standards
- [Governance](GOVERNANCE.md) - Project governance
- [Security](SECURITY.md) - Vulnerability reporting

## License

Apache License 2.0 - See [LICENSE](LICENSE).

Copyright The KubeVirt Shepherd Authors.

[kubevirt]: https://kubevirt.io
[issues]: https://github.com/kv-shepherd/shepherd/issues
