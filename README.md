# Orkest

SDLC agent orchestration. This repository currently contains a minimal Go HTTP
server, an unimplemented `agentctl` entry point, and a React/TypeScript/Vite shell.
Product services, authentication, database access, and OpenCode/ACP execution are
not implemented. Read [requirements](docs/requirements.md) and the
[proposed stack](docs/tech-stack.md) before extending the project.

## Local development

Prerequisites: Go 1.26.2, Node.js 22.19.0 (npm 11), Docker Engine with Compose v2.
Dependencies are pinned in `web/package.json` and `web/package-lock.json`.

1. Copy `.env.example` to `.env` and set `POSTGRES_PASSWORD` to a local password.
2. Run `make setup` to install frontend dependencies.
3. Run `make db-up` to start PostgreSQL and wait for its health check.
4. Run `make dev` for the API at `http://127.0.0.1:8080`.
5. In another terminal, run `make dev-web` for the frontend (Vite prints its URL).

`GET /healthz` reports process liveness only. The API does not connect to the
database yet. Set `HTTP_ADDR` to override its listen address. The frontend is a
static shell and does not yet call the API.

PostgreSQL listens on localhost, port 5432 (override `POSTGRES_PORT` in `.env`).
Database and user are both `orkest`; the password comes from `.env`. Compose is
limited to PostgreSQL for now. Data persists in the `postgres18_data` named volume
at PostgreSQL 18's `/var/lib/postgresql` mount. `make db-down` retains the volume.
Postgres initialization variables apply only when the volume is first created;
changing `.env` does not change existing database credentials. The version-specific
volume keeps PostgreSQL 18 separate from any older `postgres_data` volume; existing
older databases require an explicit dump/restore or supported upgrade before use. Do not commit
credentials or remove the volume unless you intend to delete local data.

## Verification and builds

- `make check`: Go formatting, vet, tests, frontend type checks, and all builds.
- `make fmt`: format Go sources.
- `make build`: create `bin/orkest`, `bin/agentctl`, and `web/dist`.
- `make test-integration` and `make test-recovery`: explicitly fail until real
  suites and prerequisites exist. No recovery behavior is claimed by this scaffold.

The Go server currently uses `net/http` routing. Add Chi, pgx/sqlc, migrations,
and feature packages as their first implementations need them. No schema or
empty feature packages are included. Validate the pinned OpenCode/ACP prototype
and recovery behavior before building the full UI.
