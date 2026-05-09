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

### Recommended First Deploy

Use the deploy script unless you specifically need raw `docker compose`
control. For the default bundled PostgreSQL topology, the only required edit is
the public URL.

```bash
# 1. Prepare the environment file
cp deploy/prod/.env.prod.example deploy/prod/.env.prod
#    Edit .env.prod:
#      - required: SERVER_PUBLIC_BASE_URL
#      - optional: DATABASE_URL + DEPLOY_BUNDLED_POSTGRES=false for external PostgreSQL

# 2. Provide TLS certificates
mkdir -p deploy/prod/tls
cp /path/to/cert.pem deploy/prod/tls/cert.pem
cp /path/to/key.pem  deploy/prod/tls/key.pem

# 3. Build, deploy, and seed the initial admin/bootstrap data
bash deploy/prod/deploy-prod.sh --with-seed
```

On the default bundled PostgreSQL path, `deploy-prod.sh` will automatically:

- generate `POSTGRES_PASSWORD` if it is empty and persist it back to `.env.prod`
- generate `DEV_ADMIN_PASSWORD` if it is empty and persist it back to `.env.prod`
- build `DATABASE_URL` for the bundled `db` service when it is empty
- leave `SECURITY_SESSION_SECRET` and `SECURITY_ENCRYPTION_KEY` empty so the
  server can load existing bootstrap secrets from PostgreSQL, or generate and
  persist them inside PostgreSQL on first startup
- generate a self-signed TLS certificate when `deploy/prod/tls/cert.pem` and
  `deploy/prod/tls/key.pem` are missing

Additional script entry points:

```bash
bash deploy/prod/deploy-prod.sh              # builds + deploys
bash deploy/prod/deploy-prod.sh --with-seed  # first deploy/bootstrap
bash deploy/prod/deploy-prod.sh --with-seed --with-experience-seed
bash deploy/prod/deploy-prod.sh --help       # all options
```

On first run, `deploy-prod.sh` will generate `deploy/prod/.env.prod` from
`deploy/prod/.env.prod.example` if the file is missing. The generated template
stays local to the deployment host; it is not copied into container images.

When `DATABASE_URL` points to an external PostgreSQL host, `deploy-prod.sh`
auto-detects that topology and does not start the bundled `postgres:18`
service. Override with `DEPLOY_BUNDLED_POSTGRES=true|false` only when you need
to force the topology.

### Manual Docker Compose Path

If you prefer plain `docker compose` without `deploy-prod.sh`, fill the blank
credential fields in `.env.prod` yourself first, then run the compose commands
manually.

```bash
# 1. Build images
docker build --target runtime -t shepherd-server:latest .
docker build -t shepherd-web:latest -f deploy/prod/web.Dockerfile web/

# 2. Fill .env.prod manually
cp deploy/prod/.env.prod.example deploy/prod/.env.prod
#    Set at least:
#      - POSTGRES_PASSWORD (or a full external DATABASE_URL)
#      - DATABASE_URL
#      - SERVER_PUBLIC_BASE_URL

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
| `DATABASE_URL` | PostgreSQL connection string |
| `DEPLOY_BUNDLED_POSTGRES` | `auto`, `true`, or `false` to control bundled vs external PostgreSQL topology |
| `SECURITY_SESSION_SECRET` | Optional explicit session-signing override; leave blank to use the PostgreSQL-backed bootstrap secret flow |
| `SECURITY_ENCRYPTION_KEY` | Optional explicit data-encryption override; leave blank to use the PostgreSQL-backed bootstrap secret flow |
| `SERVER_PUBLIC_BASE_URL` | External URL (e.g. `https://shepherd.example.com`) |
| `SERVER_ALLOWED_ORIGINS` | CORS origins (comma-separated) |
| `DATABASE_AUTO_APPLY_VERSIONED_MIGRATIONS` | Apply reviewed Atlas migrations before the server starts (`true` / `false`) |
| `DATABASE_AUTO_MIGRATE` | Ent schema auto-migrate fallback (`true` / `false`, dev-only) |

### Schema Migration Behavior

The production compose template defaults to
`DATABASE_AUTO_APPLY_VERSIONED_MIGRATIONS=true`. On startup, the server inspects
the database before opening readiness:

- Fresh database: the server creates the current Ent schema, records the latest
  Atlas migration as the baseline, and applies River queue migrations.
- Existing schema without Atlas revision history: the server adopts the schema
  through the versioned migration path, then applies River queue migrations.
- Atlas-managed schema: the server applies pending Atlas migrations normally,
  then applies River queue migrations.

Do not run raw `atlas migrate apply` against an empty production database before
the first server startup. The checked-in Atlas files are incremental migrations;
fresh installs should use the server startup path above.

See [`deploy/prod/.env.prod.example`](../deploy/prod/.env.prod.example) for the
complete variable reference. Production security checklist:

- [ ] `SERVER_UNSAFE_ALLOW_ALL_ORIGINS=false`
- [ ] `GIN_MODE=release`
- [ ] Replace self-signed TLS with CA-issued certificates
- [ ] Rotate default admin password after first login

## Generated Credentials and Recovery

The production template deliberately keeps some values empty so the bootstrap
path is simpler and safer.

| Value | How it is created | What it is used for | If you forget or lose it |
|-------|-------------------|---------------------|---------------------------|
| `POSTGRES_PASSWORD` | `deploy-prod.sh` can generate it and write it to `.env.prod` | Authenticates the bundled `postgres:18` service and the app's bundled-DB `DATABASE_URL` | The app cannot reconnect to the bundled database after restart until you restore the password or reset the PostgreSQL password and update `.env.prod` |
| `DEV_ADMIN_PASSWORD` | `deploy-prod.sh` can generate it and write it to `.env.prod` | Used only by `deploy-prod.sh` to rotate the seeded `admin` account away from the default `admin / admin` password during bootstrap | You lose the initial admin login password until it is reset through another platform admin session or direct recovery access |
| `SECURITY_SESSION_SECRET` | Leave blank to let the server load an existing value from PostgreSQL, or generate and persist one there on first startup | Signs login/session tokens | If you set an explicit override and later change or lose it, all existing sessions become invalid and users must sign in again. If you leave it blank and keep the database, Shepherd can recover it automatically |
| `SECURITY_ENCRYPTION_KEY` | Leave blank to let the server load an existing value from PostgreSQL, or generate and persist one there on first startup | Encrypts sensitive data at rest, including stored infrastructure credentials | If you set an explicit override and later change or lose it, encrypted data can become unreadable until the original key is restored or the secrets are re-entered. If you leave it blank and keep the database, Shepherd can recover it automatically |

Practical guidance:

- Back up `.env.prod` because it contains the generated bundled PostgreSQL and initial admin credentials.
- Back up PostgreSQL because it stores the auto-managed bootstrap security secrets when `SECURITY_*` is left blank.
- Prefer leaving `SECURITY_SESSION_SECRET` and `SECURITY_ENCRYPTION_KEY` empty unless you already have an external secret-management process and a rotation plan.

## Management

```bash
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod logs -f   # logs
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod ps        # status
docker compose -f deploy/prod/docker-compose.prod.yml -p shepherd-prod down      # stop
```
