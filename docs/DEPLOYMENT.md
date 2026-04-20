# Production Deployment

Shepherd ships with a Docker Compose-based production topology built around
`server`, `web`, and `nginx`, with an optional bundled PostgreSQL service when
you do not point `DATABASE_URL` at an external database.

## Service Architecture

| Service | Image | Role |
|---------|-------|------|
| **db** | `postgres:18` | Optional bundled data persistence service |
| **server** | `shepherd-server` | Go API backend (distroless runtime) |
| **web** | `shepherd-web` | Next.js SSR frontend |
| **nginx** | `nginx:1.27-alpine` | TLS termination, reverse proxy, rate limiting |

## Deploying from Source

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
bash deploy/prod/deploy-prod.sh --with-seed --with-experience-seed
bash deploy/prod/deploy-prod.sh --help       # all options
```

On first run, `deploy-prod.sh` will generate `deploy/prod/.env.prod` from
`deploy/prod/.env.prod.example` if the file is missing. The generated template
is not copied into container images; it remains a local deployment input that
you should review and fill before rerunning the script.

When `DATABASE_URL` points to an external PostgreSQL host, `deploy-prod.sh`
auto-detects that topology and does not start the bundled `postgres:18`
service. Override with `DEPLOY_BUNDLED_POSTGRES=true|false` only when you need
to force the topology.

## VPS Experience Seed

If you want a browser-accessible product experience on a VPS, keep the real
production deployment topology and add the extended experience seed instead of
using a separate demo mode:

```bash
bash deploy/prod/deploy-prod.sh --with-seed --with-experience-seed
```

This keeps the normal `server` + `web` + `nginx` + PostgreSQL deployment path
and then injects:

- `admin / admin`
- `test / test`
- sample system, service, template, instance sizes, approval tickets, and notifications

By default, the experience seed registers a stub cluster when no kubeconfig is
provided, which means cluster-backed VM actions remain illustrative until a
real Kubernetes/KubeVirt environment is configured. To seed against a real
cluster, export `E2E_KUBECONFIG_PATH=/path/to/kubeconfig` (or
`E2E_KUBECONFIG_B64`) before running `deploy-prod.sh`.

## Key Configuration

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `DEPLOY_BUNDLED_POSTGRES` | `auto`, `true`, or `false` to control bundled vs external PostgreSQL topology |
| `SECURITY_SESSION_SECRET` | Session signing key (`openssl rand -hex 32`) |
| `SECURITY_ENCRYPTION_KEY` | Data encryption key (`openssl rand -hex 32`) |
| `SERVER_PUBLIC_BASE_URL` | External URL (e.g. `https://shepherd.example.com`) |
| `SERVER_ALLOWED_ORIGINS` | CORS origins (comma-separated) |
| `DATABASE_AUTO_MIGRATE` | Auto-migrate schema on startup (`true` / `false`) |

See [`deploy/prod/.env.prod.example`](../deploy/prod/.env.prod.example) for the
complete variable reference. Production security checklist:

- [ ] `SERVER_UNSAFE_ALLOW_ALL_ORIGINS=false`
- [ ] `GIN_MODE=release`
- [ ] Replace self-signed TLS with CA-issued certificates
- [ ] Rotate default admin password after first login

## Management

```bash
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod logs -f   # logs
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod ps        # status
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod down      # stop
```
