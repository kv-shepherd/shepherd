# Local Dev Docker Workflow

## Goal

Provide a single-command local development workflow that starts backend,
frontend, and database together for fast early-stage iteration, while keeping
local data by default unless an explicit clean reset is requested.

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

`deploy/dev/start-dev.sh` preserves the existing dev database by default:

- remove app services (`server`, `web`, `nginx`)
- keep the `db` container and its data volume
- rebuild backend/frontend images
- re-run baseline development seed (`cmd/seed`) unless `--skip-seed` is used
- optionally seed extended local fixtures (`cmd/e2e-seed`)

Use `--clean-all` when you intentionally want a fresh local environment:

- remove all compose services
- `down --volumes --remove-orphans`
- rebuild backend/frontend images
- re-seed from a clean database

## Seeded Accounts

- bootstrap admin created by `cmd/seed`: `admin/admin`
- local dev startup keeps that account at: `admin/admin` by default
- extended local fixture admin: `e2e-admin/e2e-admin-123`

Set `DEV_ADMIN_PASSWORD=<password>` when you want local dev to rotate the
bootstrap admin account after seed.
Set `DEV_INCLUDE_E2E_SEED=1` or pass `--e2e-seed` when you want the extended local fixtures too.

Recommended split:

- local `./start-dev.sh`: baseline bootstrap only, preserving DB state by default
- local clean reset: `./start-dev.sh --clean-all`
- GitHub Codespaces devcontainer: `--clean-all --e2e-seed` on first create, then `--skip-seed` on resume

## Browser Warning Bridge

When started via `./start-dev.sh`, the frontend enables a dev-only browser warning bridge.

- Browser `console.warn` / `console.error` still appear in DevTools as usual.
- The same warning/error payload is also mirrored to the local frontend server log.
- Host frontend mode writes these entries to `tmp/dev-web.log`.
- Docker frontend mode writes them to `docker compose logs web`.

This bridge is development-only and is not intended for production deployments.

This workflow is optimized for fast iteration while keeping local state stable
by default.
