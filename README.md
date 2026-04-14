# KubeVirt Shepherd

[![Licensed under Apache License version 2.0](https://img.shields.io/github/license/kv-shepherd/shepherd.svg)](https://www.apache.org/licenses/LICENSE-2.0)
[![Go Version](https://img.shields.io/github/go-mod/go-version/kv-shepherd/shepherd)](go.mod)
[![CI](https://github.com/kv-shepherd/shepherd/actions/workflows/ci.yml/badge.svg)](https://github.com/kv-shepherd/shepherd/actions/workflows/ci.yml)

**KubeVirt Shepherd** is a governance-first management platform for
[KubeVirt][kubevirt] virtual machines. It provides self-service VM lifecycle
management with structured approval workflows, RBAC, and full audit trails
across multiple Kubernetes clusters.

> *Like a shepherd tending a flock, this platform ensures that VMs are properly
> managed throughout their lifecycle — users enjoy self-service freedom while
> governance policies prevent resource sprawl and orphaned instances.*

## Why Shepherd?

KubeVirt solves *"running VMs on Kubernetes"*. Shepherd solves what comes next:
**who** can request a VM, **who** approves it, how are **quotas** enforced,
and where is the **audit trail**?

Today, these governance capabilities are mainly available through Red Hat's
commercial stack (OpenShift Virtualization + RHACM). Shepherd provides an
**open-source, vendor-neutral alternative** built with production-grade
engineering standards — contract-first API design, declarative schema
migrations, structured ADR governance, and comprehensive CI gate enforcement.

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

> ⚠️ **Alpha Track** — Core governed VM request, approval, RBAC, audit, and
> console workflows are functional. Public deployment guides are available
> below; release automation and operational guidance are still being hardened.

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

### GitHub Codespaces Demo

Use Codespaces as a browser-based product demo, not as the primary source
development environment.

[![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://codespaces.new/kv-shepherd/shepherd?quickstart=1)

#### Try the demo

1. Click **Open in GitHub Codespaces**.
2. Wait for the container bootstrap to finish.
3. Open the forwarded **Shepherd UI** port if the browser does not open automatically.
4. Sign in with `admin / admin`, then follow the password-change prompt if it appears.

Codespaces demo behavior:

- first create resolves the latest published Shepherd release and pulls the matching server/web images from GHCR
- the first bootstrap starts from a clean database and seeds baseline bootstrap data plus extended demo fixtures
- later Codespace restarts reuse the existing demo data and only resume services
- the first start can still take a few minutes because the release images are pulled and the demo fixtures are seeded inside the Codespace

Default demo sign-in:
- `admin / admin` (first sign-in prompts a password change)
- Codespaces starts with demo data, but it does **not** assume a live K8s/KubeVirt cluster is available. VM create, power, modify, delete, and console flows depend on a real cluster connection and can fail with normal availability or cluster-health errors until one is configured.

### Prerequisites

Go 1.25+ · Node.js 22+ · PostgreSQL 18+ · Docker 24+

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full development setup.

## Production Deployment

Shepherd ships with a Docker Compose-based production topology built around
`server`, `web`, and `nginx`, with an optional bundled PostgreSQL service when
you do not point `DATABASE_URL` at an external database.

| Service | Image | Role |
|---------|-------|------|
| **db** | `postgres:18` | Optional bundled data persistence service |
| **server** | `shepherd-server` | Go API backend (distroless runtime) |
| **web** | `shepherd-web` | Next.js SSR frontend |
| **nginx** | `nginx:1.27-alpine` | TLS termination, reverse proxy, rate limiting |

### Deploying from Source

```bash
# 1. Build images
docker build -t shepherd-server:latest .
docker build -t shepherd-web:latest -f deploy/prod/web.Dockerfile web/

# 2. Configure environment
cp deploy/prod/.env.prod.example deploy/prod/.env.prod
#    Edit .env.prod — set DATABASE password, SESSION_SECRET, ENCRYPTION_KEY, PUBLIC_BASE_URL

# 3. Provide TLS certificates
mkdir -p deploy/prod/tls
cp /path/to/cert.pem deploy/prod/tls/cert.pem
cp /path/to/key.pem  deploy/prod/tls/key.pem

# 4. Launch
docker compose -f deploy/prod/docker-compose.prod.yml \
  --env-file deploy/prod/.env.prod -p shepherd-prod up -d

# 5. Seed initial data (first deploy only)
docker compose -f deploy/prod/docker-compose.prod.yml \
  --env-file deploy/prod/.env.prod -p shepherd-prod \
  exec -T server /usr/local/bin/seed
```

Or use the one-click script:

```bash
bash deploy/prod/deploy-prod.sh              # builds + deploys
bash deploy/prod/deploy-prod.sh --with-seed  # first deploy/bootstrap
bash deploy/prod/deploy-prod.sh --help   # all options
```

On first run, `deploy-prod.sh` will generate `deploy/prod/.env.prod` from
`deploy/prod/.env.prod.example` if the file is missing. The generated template
is not copied into container images; it remains a local deployment input that
you should review and fill before rerunning the script.

When `DATABASE_URL` points to an external PostgreSQL host, `deploy-prod.sh`
auto-detects that topology and does not start the bundled `postgres:18`
service. Override with `DEPLOY_BUNDLED_POSTGRES=true|false` only when you need
to force the topology.

### Key Configuration

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `DEPLOY_BUNDLED_POSTGRES` | `auto`, `true`, or `false` to control bundled vs external PostgreSQL topology |
| `SECURITY_SESSION_SECRET` | Session signing key (`openssl rand -hex 32`) |
| `SECURITY_ENCRYPTION_KEY` | Data encryption key (`openssl rand -hex 32`) |
| `SERVER_PUBLIC_BASE_URL` | External URL (e.g. `https://shepherd.example.com`) |
| `SERVER_ALLOWED_ORIGINS` | CORS origins (comma-separated) |
| `DATABASE_AUTO_MIGRATE` | Auto-migrate schema on startup (`true` / `false`) |

See [`deploy/prod/.env.prod.example`](deploy/prod/.env.prod.example) for the
complete variable reference. Production security checklist:

- [ ] `SERVER_UNSAFE_ALLOW_ALL_ORIGINS=false`
- [ ] `GIN_MODE=release`
- [ ] Replace self-signed TLS with CA-issued certificates
- [ ] Rotate default admin password after first login

### Management

```bash
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod logs -f   # logs
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod ps        # status
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod down      # stop
```

## Documentation

- [docs/README.md](docs/README.md) — Documentation index & 5-minute quick start
- [docs/adr/](docs/adr/) — 53 Architecture Decision Records
- [docs/design/](docs/design/) — Implementation specifications
- [docs/RELEASE.md](docs/RELEASE.md) — Release process

## Community

- [GitHub Issues][issues] — Bug reports and feature requests
- [Contributing](CONTRIBUTING.md) — PR workflow, CI gates, coding standards
- [Code of Conduct](CODE_OF_CONDUCT.md) — Community standards
- [Governance](GOVERNANCE.md) — Project governance
- [Security](SECURITY.md) — Vulnerability reporting

## License

Apache License 2.0 — See [LICENSE](LICENSE).

    Copyright The KubeVirt Shepherd Authors.

[kubevirt]: https://kubevirt.io
[issues]: https://github.com/kv-shepherd/shepherd/issues
