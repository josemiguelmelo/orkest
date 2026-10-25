# Orkest

SDLC agent orchestration. This repository contains a Go platform API, a standalone
OpenCode/ACP `agentctl` service, and a React/TypeScript/Vite shell. Organization and
workspace membership APIs use PostgreSQL; task services and platform-to-agent
integration are not implemented.
Read [requirements](docs/requirements.md) and the
[proposed stack](docs/tech-stack.md) before extending the project.

## Local development

Prerequisites: Go 1.26.2, Node.js 22.19.0 (npm 11), Docker Engine with Compose v2.
Dependencies are pinned in `web/package.json` and `web/package-lock.json`.

1. Copy `.env.example` to `.env` and set `POSTGRES_PASSWORD` to a local password. The app accepts either `DATABASE_URL` or all five `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USERNAME`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` values; `DATABASE_URL` takes precedence.
2. Run `make setup` to install frontend dependencies.
3. Run `make db-up` to start PostgreSQL and wait for its health check.
4. Run `make auth-up` to start Keycloak locally.
5. Run `make dev` for the API at `http://127.0.0.1:8080` with Air auto-reload.
6. In another terminal, run `make dev-web` for the frontend (Vite prints its URL).

`GET /healthz` reports process liveness only. The platform connects to PostgreSQL
and applies pending Goose migrations on startup. Set `HTTP_ADDR` to override its
listen address. The frontend is a static shell and does not yet call the API.
Visit `http://127.0.0.1:8080/auth/login` to sign in. The local Keycloak realm has
username `local` and password `local`; `GET /api/me` returns the internal user
profile and organization memberships. The API supports organization, workspace,
and membership management. The callback sets an HTTP-only
session cookie; API clients can also send the verified Keycloak ID token as a
bearer token. Local OIDC values are in `.env.example`. These credentials and
HTTP settings are for local development only.

The sample `.env` configures PostgreSQL at `127.0.0.1:5432`; the component-variable
form requires all five values and does not add runtime defaults.
The database and username come from `POSTGRES_DB` and `POSTGRES_USERNAME`; the password
comes from `.env`. Set `DATABASE_URL` instead to use a complete PostgreSQL connection URL. Compose runs
PostgreSQL and Keycloak. Data persists in named volumes
at PostgreSQL 18's `/var/lib/postgresql` mount. `make db-down` retains the volume.
Postgres initialization variables apply only when the volume is first created;
changing `.env` does not change existing database credentials. The version-specific
volume keeps PostgreSQL 18 separate from any older `postgres_data` volume; existing
older databases require an explicit dump/restore or supported upgrade before use. Do not commit
credentials or remove the volume unless you intend to delete local data.

## Calling an agent

`agentctl` launches pinned OpenCode 2.1.4 over ACP stdio, creates a session,
selects the configured system prompt and model, and accepts prompts, permission choices, and cancellation over REST, with responses
streamed over a receive-only WebSocket.
Reconnect replay is available while agentctl stays alive; restart recovery is unimplemented.
See [agentctl usage and limitations](docs/agentctl-api.md) for `.env` configuration,
launch requests, persistent storage, and verification instructions.

## Verification and builds

- `make check`: Go lint/formatting checks, tests, frontend type checks, and all builds.
- `make lint`: run golangci-lint v2.15.0 with the standard linters and gofumpt.
- `make fmt`: format Go sources with gofumpt, including blank lines between
  multiline functions.
- `make sqlc`: regenerate PostgreSQL query code with the pinned sqlc version.
- `make lint-install`: install the pinned linter under `bin/tools/` (requires curl
  and network access). `lint`, `fmt`, and `check` install it automatically if absent.
- `make build`: create `bin/orkest`, `bin/agentctl`, and `web/dist`.
- `make test-integration`: real OpenCode handshake; requires the pinned executable
  via `ORKEST_OPENCODE_BINARY`. Makes no model request.
- `make test-recovery`: explicitly fails; recovery verification is not implemented.

The platform API uses Chi, pgx, sqlc, and Goose. PostgreSQL schema migrations
live in `db/migrations`; the startup runner embeds and applies them. Validate the
pinned OpenCode/ACP prototype and recovery behavior before building the full UI.
