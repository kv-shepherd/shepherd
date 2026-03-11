# Local Dev Docker Workflow

## Goal

Provide a single-command local development workflow that starts and resets backend, frontend, and database together for fast early-stage iteration.

## Entry Points

- `./start-dev.sh` (project entry wrapper)
- `deploy/dev/start-dev.sh` (actual script)
- Compose file: `deploy/dev/docker-compose.yml`

## Layout

- `deploy/dev/docker-compose.yml`: integrated development stack
- `deploy/dev/nginx/default.conf`: single ingress reverse proxy
- `deploy/dev/web.Dockerfile`: frontend dev runtime image
- `web/.dockerignore`: build context filter for web image

## Runtime Topology

- `nginx` exposed at `:3000` as the single browser ingress
- `web` (Next.js dev server) internal only
- `server` (Go API) internal + optional direct `:8080` for diagnostics
- `db` (PostgreSQL) exposed at `:5432` for local DB tooling

## Why Nginx In Front

- Browser traffic uses a single origin (`http://<host>:3000`) for both UI and API path (`/api/v1`)
- This avoids ad-hoc wildcard CORS exceptions for remote device access
- Reverse proxy settings preserve host/proto headers for accurate backend behavior

## Reset Policy

`deploy/dev/start-dev.sh` intentionally performs full reset on each run:

- remove running compose services
- `down --volumes --remove-orphans`
- rebuild backend/frontend images
- re-seed development data (`cmd/seed`)
- by default also seed extended local fixtures (`cmd/e2e-seed`)

## Seeded Accounts

- bootstrap admin created by `cmd/seed`: `admin/admin`
- local dev startup then immediately rotates that account to: `admin/admin123`
- extended local fixture admin: `e2e-admin/e2e-admin-123`

Set `DEV_ADMIN_PASSWORD=<password>` to override the post-seed local admin password.
Set `DEV_INCLUDE_E2E_SEED=0` when you want the minimal baseline seed only.

This is optimized for early development consistency over state persistence.

## Future Evolution

When project maturity requires faster partial restarts or stable local data, split profiles can be added (for example, keep DB persistent while hot-reloading app containers).
