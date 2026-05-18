# Production Deployment

Shepherd ships with a Docker Compose-based production topology built around
`server`, `web`, and `nginx`, with an optional bundled PostgreSQL service when
you do not point `DATABASE_URL` at an external database.

## Service Architecture

| Service | Image | Role |
|---------|-------|------|
| **db** | `postgres:18` | Optional bundled data persistence service |
| **server** | `ghcr.io/kv-shepherd/shepherd-server` by default | Go API backend (distroless runtime) |
| **web** | `ghcr.io/kv-shepherd/shepherd-web` by default | Next.js SSR frontend |
| **nginx** | `nginx:1.27-alpine` | TLS termination, reverse proxy, rate limiting |

## Deploying from Release Images

Use this path for production or VPS hosts that should run published GHCR images
without a git checkout.

Required host tools:

- Docker with Docker Compose v2
- `curl` or `wget`
- outbound access to GHCR, GitHub raw files, and GitHub release metadata

The shortest first deploy uses bundled PostgreSQL 18, `https://localhost`, and
a generated self-signed TLS certificate:

```bash
mkdir -p shepherd-deploy && cd shepherd-deploy
curl -fsSL https://raw.githubusercontent.com/kv-shepherd/shepherd/main/deploy/prod/deploy-prod.sh | bash -s -- --with-seed
```

`wget` works too:

```bash
wget -qO- https://raw.githubusercontent.com/kv-shepherd/shepherd/main/deploy/prod/deploy-prod.sh | bash -s -- --with-seed
```

All runtime inputs are optional. Pass them before `bash` only when you need to
override the default topology:

```bash
curl -fsSL https://raw.githubusercontent.com/kv-shepherd/shepherd/main/deploy/prod/deploy-prod.sh | \
  SERVER_PUBLIC_BASE_URL=https://shepherd.example.com bash -s -- --with-seed

curl -fsSL https://raw.githubusercontent.com/kv-shepherd/shepherd/main/deploy/prod/deploy-prod.sh | \
  DATABASE_URL='<postgres-18-dsn>' DEPLOY_BUNDLED_POSTGRES=false bash -s -- --with-seed

curl -fsSL https://raw.githubusercontent.com/kv-shepherd/shepherd/main/deploy/prod/deploy-prod.sh | \
  SHEPHERD_VERSION='vX.Y.Z' bash -s -- --with-seed
```

Use PostgreSQL 18 for external databases. The bundled `postgres:18` container
is convenient for evaluation and VPS installs, but an operator-managed
PostgreSQL 18 service is recommended for production. When `DATABASE_URL` points
to an external PostgreSQL host, `deploy-prod.sh` auto-detects that topology and
does not start the bundled database. Override with
`DEPLOY_BUNDLED_POSTGRES=true|false` only when you need to force the topology.

`deploy-prod.sh` resolves release images in this order:

1. explicit `SERVER_IMAGE` and `WEB_IMAGE`
2. explicit `SHEPHERD_VERSION` or `DEPLOY_RELEASE_VERSION`
3. version-like `DEPLOY_ASSET_REF`, such as `vX.Y.Z`
4. the latest published GitHub Release

Resolved image refs and generated secrets are persisted to `.env.prod` in the
deployment directory.

## Deploying from Source Build

Use source-build deployment only when you intentionally want the target host to
build local backend and frontend images from a checkout.

```bash
git clone https://github.com/kv-shepherd/shepherd.git
cd shepherd
git pull --ff-only origin main
bash deploy/prod/deploy-prod.sh --with-seed
```

For source builds, `deploy-prod.sh` uses local image tags by default unless you
set `SHEPHERD_VERSION`, `SERVER_IMAGE`, or `WEB_IMAGE`.

## Deployment Script Behavior

On the default bundled PostgreSQL path, `deploy-prod.sh` will automatically:

- default `SERVER_PUBLIC_BASE_URL` to `https://localhost` when it is empty
- generate `POSTGRES_PASSWORD` if it is empty and persist it back to `.env.prod`
- generate `DEV_ADMIN_PASSWORD` if it is empty and persist it back to `.env.prod`
- generate `SECURITY_SESSION_SECRET` and `SECURITY_ENCRYPTION_KEY` if they are
  empty and persist them back to `.env.prod`
- build `DATABASE_URL` for the bundled `db` service when it is empty
- generate a self-signed TLS certificate when `tls/cert.pem` and `tls/key.pem`
  are missing

Additional script entry points:

```bash
bash deploy-prod.sh              # deploy using release images
bash deploy-prod.sh --with-seed  # first deploy/bootstrap
bash deploy-prod.sh --with-seed --with-experience-seed
bash deploy-prod.sh --help       # all options
```

### Manual Docker Compose Path

If you prefer plain `docker compose` without `deploy-prod.sh`, fill the blank
credential and image fields in `.env.prod` yourself first, then run the compose
commands manually. `deploy-prod.sh` resolves and persists image references for
you; raw compose requires `SERVER_IMAGE` and `WEB_IMAGE` to be set explicitly.

```bash
# 1. Build images
docker build --target runtime -t shepherd-server:latest .
docker build -t shepherd-web:latest -f deploy/prod/web.Dockerfile web/

# 2. Fill .env.prod manually
cp deploy/prod/.env.prod.example deploy/prod/.env.prod
#    Set at least:
#      - SERVER_IMAGE=shepherd-server:latest
#      - WEB_IMAGE=shepherd-web:latest
#      - POSTGRES_PASSWORD (or a full external DATABASE_URL)
#      - DATABASE_URL
#    SERVER_PUBLIC_BASE_URL defaults to https://localhost when blank.

# 3. Launch
docker compose -f deploy/prod/docker-compose.prod.yml \
  --env-file deploy/prod/.env.prod -p shepherd-prod up -d

# 4. Seed initial data (first deploy only)
docker compose -f deploy/prod/docker-compose.prod.yml \
  --env-file deploy/prod/.env.prod -p shepherd-prod \
  exec -T server /usr/local/bin/seed
```

The raw compose path seeds the default `admin / admin` account and leaves the
forced password change to the first interactive login. `DEV_ADMIN_PASSWORD` is
used only by `deploy-prod.sh` when it performs the post-seed password rotation
step for you.

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
| `DATABASE_URL` | PostgreSQL connection string; `deploy-prod.sh` creates a bundled PostgreSQL DSN when blank |
| `DEPLOY_BUNDLED_POSTGRES` | `auto`, `true`, or `false` to control bundled vs external PostgreSQL topology |
| `SECURITY_SESSION_SECRET` | Session-signing secret; `deploy-prod.sh` generates and persists one when blank |
| `SECURITY_ENCRYPTION_KEY` | Hex-encoded AES-256 data-encryption key; `deploy-prod.sh` generates and persists one when blank |
| `SERVER_PUBLIC_BASE_URL` | External URL; defaults to `https://localhost` when blank |
| `SERVER_ALLOWED_ORIGINS` | CORS origins (comma-separated) |
| `DATABASE_AUTO_APPLY_VERSIONED_MIGRATIONS` | Apply reviewed Atlas migrations before the server starts (`true` / `false`) |
| `DATABASE_AUTO_MIGRATE` | Ent schema auto-migrate fallback (`true` / `false`, dev-only) |

### Schema Migration Behavior

Released Docker images and Go runtime archives are built to run startup
migrations automatically. `DATABASE_AUTO_APPLY_VERSIONED_MIGRATIONS` defaults to
`true`; production compose files keep that default. On startup, the server
inspects the database before opening readiness:

- Fresh database: the server creates the current Ent schema, records the latest
  Atlas migration as the baseline, and applies River queue migrations.
- Existing schema without Atlas revision history: the server adopts the schema
  through the versioned migration path, then applies River queue migrations.
- Atlas-managed schema: the server applies pending Atlas migrations normally,
  then applies River queue migrations.

Do not run raw `atlas migrate apply` against production databases before the
server starts. The checked-in Atlas files are incremental migrations and the
release artifacts already include the Atlas executable plus migration directory
needed by the server startup path.

See [`deploy/prod/.env.prod.example`](../deploy/prod/.env.prod.example) for the
complete variable reference. Production security checklist:

- [ ] `SERVER_UNSAFE_ALLOW_ALL_ORIGINS=false`
- [ ] `GIN_MODE=release`
- [ ] Replace self-signed TLS with CA-issued certificates
- [ ] Rotate default admin password after first login

## Generated Credentials and Recovery

The production template deliberately keeps some values empty so the deploy
script can generate strong first-run credentials and persist them before
release-mode services start.

| Value | How it is created | What it is used for | If you forget or lose it |
|-------|-------------------|---------------------|---------------------------|
| `POSTGRES_PASSWORD` | `deploy-prod.sh` can generate it and write it to `.env.prod` | Authenticates the bundled `postgres:18` service and the app's bundled-DB `DATABASE_URL` | The app cannot reconnect to the bundled database after restart until you restore the password or reset the PostgreSQL password and update `.env.prod` |
| `DEV_ADMIN_PASSWORD` | `deploy-prod.sh` can generate it and write it to `.env.prod` | Used only by `deploy-prod.sh` to rotate the seeded `admin` account away from the default `admin / admin` password during bootstrap | You lose the initial admin login password until it is reset through another platform admin session or direct recovery access |
| `SECURITY_SESSION_SECRET` | `deploy-prod.sh` can generate it and write it to `.env.prod` | Signs login/session tokens | Existing sessions become invalid until users sign in again if the value is changed or lost |
| `SECURITY_ENCRYPTION_KEY` | `deploy-prod.sh` can generate it and write it to `.env.prod` | Encrypts sensitive data at rest, including stored infrastructure credentials | Encrypted data can become unreadable until the original key is restored or the secrets are re-entered if the value is changed or lost |

Practical guidance:

- Back up `.env.prod` because it contains the generated bundled PostgreSQL password, initial admin credential, and production security secrets.
- Back up PostgreSQL because it stores application data encrypted with `SECURITY_ENCRYPTION_KEY`.
- Use an external secret-management process only if it can preserve and restore the same `SECURITY_SESSION_SECRET` and `SECURITY_ENCRYPTION_KEY` values during redeployments.

## Management

```bash
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod logs -f   # logs
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod ps        # status
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod down      # stop
```
