# Orkest

[![Runtime conformance](https://github.com/josemiguelmelo/orkest/actions/workflows/runtime-conformance.yml/badge.svg)](https://github.com/josemiguelmelo/orkest/actions/workflows/runtime-conformance.yml)

Orkest is a platform for orchestrating software-development agents. It combines a Go API, a React management workbench, PostgreSQL-backed project state, and an independent `agentctl` service that runs coding agents in isolated environments.

Orkest is intended for developers and teams who want to manage organizations, workspaces, tasks, repositories, agent definitions, integrations, and agent-run activity from one application. The current implementation is an active platform prototype: core management, task dispatch, Docker execution, durable session handling, and runtime contracts are implemented, while some credentialed runtime behaviors and product requirements remain under validation.

## Contents

- [Features](#features)
- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Run locally](#run-locally)
- [Configuration](#configuration)
- [Usage](#usage)
- [Development](#development)
- [Contributing](#contributing)
- [Documentation](#documentation)
- [Project status and license](#project-status-and-license)

## Features

- Organization, workspace, membership, role, task, comment, and repository management.
- React/TypeScript/Vite management workbench served by the Go API.
- Keycloak/OpenID Connect authentication with application-owned authorization.
- Workspace-scoped agent definitions, runtime profiles, skills, MCP servers, and environment entries.
- GitHub Cloud and Jira Server task integrations, plus GitHub and GitLab coding-provider support.
- Task launch orchestration with ordered activity, chat, cancellation, and permission responses.
- Independent `agentctl` workers for OpenCode 2.1.4, Claude Code ACP 0.99.0, and Codex ACP 2.3.0.
- PostgreSQL migrations and durable task-session journals.
- Docker-based agent execution and optional execution-profile image builds with BuildKit or Docker.

Execution and recovery behavior is deliberately conservative. Real model execution, approval round trips, and interrupted-work continuation require separate credentialed verification; see the [runtime provider documentation](docs/runtime-providers.md).

## Architecture

Orkest is a modular monolith with two Go binaries and one web application:

```text
React/Vite workbench
        │ HTTP/OIDC
        ▼
Go Orkest API ─── PostgreSQL (state, migrations, sessions)
        │
        ├── Docker executor ─── agentctl container
        ├── Local executor ─── host agentctl process (trusted development only)
        │                              │ ACP over stdio
        │                              ▼
        │                       OpenCode / Claude Code / Codex
        │
        └── Keycloak (local OIDC) and optional registry
```

- `cmd/orkest` starts the platform API, applies pending Goose migrations, and serves `web/dist` when it exists.
- `internal/app` wires platform services; domain packages under `internal/` own business rules, while `internal/postgres` contains persistence adapters.
- `cmd/agentctl` starts the standalone agent worker. It exposes REST commands and a receive-only activity WebSocket and owns the provider process independently of the platform.
- `web/` contains the React workbench. Its typed API client and validators are generated from `internal/app/httpapi/openapi.yaml`.
- `db/migrations` and `db/queries` contain the PostgreSQL schema inputs. The startup runner embeds and applies migrations.

For more detail, see the [technology stack](docs/tech-stack.md), [webapp guide](docs/webapp-implementation.md), [agentctl guide](docs/agentctl.md), and [project requirements](docs/requirements.md).

## Prerequisites

- Go 1.26.2
- Node.js 22.22.2 and npm 11 (Node.js 24.15.0 or supported Node.js 26+ releases are also accepted by `web/package.json`)
- Docker Engine with Docker Compose v2
- Python 3 for runtime-boundary checks and selected tooling

Docker is required for the local PostgreSQL and Keycloak services. Agent execution and the full Compose profile additionally require access to the Docker Engine socket.

## Run locally

From the repository root:

```sh
cp .env.example .env
# Edit .env and set POSTGRES_PASSWORD.

make setup
make db-up
make auth-up
make build
make dev
```

### Run agents with the Local executor

Local runs `agentctl` directly on the Orkest host. It is intended for trusted,
single-user development and does not isolate tasks from the host or from each
other. Docker is still needed for the local PostgreSQL and Keycloak services;
the Docker executor itself is not needed for Local agent runs.

Build/install the host `agentctl` binary and frontend, then configure `.env` to
select it. `make build` creates `bin/agentctl` as well as the Orkest binary and
web assets:

```sh
make build
```

Set these values in `.env`, replacing the binary path with the absolute path to
this repository's `bin/agentctl`:

```dotenv
ORKEST_EXECUTOR=local
ORKEST_AGENTCTL_BINARY=/absolute/path/to/orkest/bin/agentctl
```

Install the pinned runtime you select for an agent definition on the host and
make it available on `PATH` (for example, OpenCode 2.1.4). Local passes the
host `PATH` to the runtime automatically so runtime launchers can find their
dependencies. Local does not use the runtime Docker images or Docker execution profiles. Then start the
infrastructure and Orkest as usual with `make db-up`, `make auth-up`, and
`make dev`. See the [Local executor guide](docs/executors/local.md) and
[runtime provider guides](docs/runtime-providers.md) for runtime requirements.

The API listens on `http://127.0.0.1:8080` by default. In a separate terminal, open the management workbench at that address and sign in at `/auth/login` with the imported local Keycloak account:

```text
username: local
password: local
```

Useful endpoints are:

- `GET /healthz` — process liveness check.
- `GET /api/me` — the authenticated internal user and memberships.
- `GET /docs/` — interactive platform API documentation.
- `GET /openapi.yaml` — the platform OpenAPI contract.

`make dev` uses Air for backend auto-reload. To run the backend once, use `make dev-once`. The backend serves the built frontend from `ORKEST_WEB_DIR` (default `web/dist`), so run `make build` after frontend changes.

For frontend iteration, keep the API running and use a second terminal:

```sh
make dev-web
```

Vite runs on `http://127.0.0.1:5173` and proxies `/api` and `/auth` to the backend. Set `OIDC_REDIRECT_URL=http://127.0.0.1:5173/auth/callback` before starting the backend, and use the same hostname throughout the login flow.

To run the full containerized stack:

```sh
docker compose up --build                         # infrastructure services
docker compose --profile orkest up --build        # infrastructure plus Orkest
```

The default Compose command starts PostgreSQL, Keycloak, and the local registry. The `orkest` profile also builds and starts the platform, mounts the host Docker socket, and exposes Orkest on port 8080. Build the runtime images before starting task execution:

```sh
make docker-agentctl-images
make docker-agentctl-push-local  # also pushes images to the local registry
```

`make db-down` stops services but retains the PostgreSQL volume. PostgreSQL initialization variables apply only when that volume is first created; do not remove it unless deleting local data is intentional.

## Configuration

`.env.example` is the authoritative local configuration template. Both binaries load an optional `.env` from the current directory; existing shell variables take precedence.

| Variable | Purpose |
| --- | --- |
| `DATABASE_URL` | Complete PostgreSQL URL; takes precedence over the component variables. |
| `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USERNAME`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | PostgreSQL connection components; all five are required when `DATABASE_URL` is unset. |
| `HTTP_ADDR` | Platform listen address; defaults to `127.0.0.1:8080`. |
| `OIDC_ISSUER_URL` | Browser-visible Keycloak issuer; defaults to the local realm URL. |
| `OIDC_INTERNAL_ISSUER_URL` | Optional server-side issuer URL, useful when the browser and backend use different network addresses. |
| `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URL`, `OIDC_COOKIE_SECURE` | OIDC client and callback settings. |
| `KEYCLOAK_PORT` | Local Keycloak host port; defaults to `8082` in Compose. |
| `ORKEST_WEB_DIR` | Built frontend directory served by Go; defaults to `web/dist`. |
| `ORKEST_DATA_DIR` | Durable host directory for workspace files, task checkouts, and agent state. |
| `ORKEST_EXECUTOR` | Agent executor: `docker` (default) or `local`. Local does not isolate task processes. |
| `ORKEST_AGENTCTL_BINARY` | Absolute path to the installed `agentctl` executable; required when using Local. `make build` creates `bin/agentctl`. |
| `ORKEST_AGENT_PROVIDER_ENV` | Comma-separated host environment variable names explicitly passed to agent runtimes. |
| `ORKEST_INTEGRATION_ENCRYPTION_KEY` | Required for encrypted integration or workspace-secret storage; use a unique 32-byte hex key. |
| `AGENTCTL_*_IMAGE` | Agent runtime images for OpenCode, Claude Code, and Codex. Defaults are pinned in `.env.example`. |
| `ORKEST_EXECUTION_IMAGE_REPOSITORY` | Registry repository used by execution profiles. Leave unset when execution-profile builds are not needed. |
| `ORKEST_IMAGE_BUILDER` | Execution-profile builder: `buildkit` (default) or `docker`. |
| `ORKEST_BUILDCTL_BINARY`, `ORKEST_BUILDKIT_*` | BuildKit binary, address, daemonless mode, and optional TLS settings. |
| `AGENTCTL_LISTEN`, `AGENTCTL_WORK_DIR`, `AGENTCTL_STATE_DIR` | Standalone `agentctl` address and required task/state directories. |
| `ORKEST_CODEX_AUTH_FILE` | Optional Codex auth file override. Local defaults to `~/.codex/auth.json` when present; Docker requires this path to be visible to the daemon. |

When using Colima or another named Docker context, set `DOCKER_HOST` in `.env` to the endpoint returned by `docker context inspect --format '{{.Endpoints.docker.Host}}'`. Orkest's Docker client reads this variable independently of the Docker CLI's active context. For Compose, `ORKEST_DATA_DIR` must be a path visible to the Docker daemon because nested agent containers use host bind mounts.

## Usage

After signing in, use the workbench to create an organization and workspace, register repositories, define an agent, and create and assign tasks. A task must be explicitly started by a user; assignment alone does not start execution. Task details expose run history, ordered activity, chat, permission responses, and cancellation where a run supports them.

The platform API is authenticated by the browser session or a verified Keycloak bearer token. For example, the public health endpoint can be checked without authentication:

```sh
curl http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

For API request and response schemas, use the local [OpenAPI UI](http://127.0.0.1:8080/docs/) or inspect [`internal/app/httpapi/openapi.yaml`](internal/app/httpapi/openapi.yaml). Do not put real credentials in task content, logs, commits, or examples.

### Run `agentctl` independently

`agentctl` runs one provider process and requires an existing work directory and durable state directory:

```sh
go run ./cmd/agentctl \
  -runtime opencode \
  -listen 127.0.0.1:8081 \
  -work-dir /absolute/path/to/project \
  -state-dir /absolute/path/to/agentctl-state
```

The pinned runtime executable must be installed separately for standalone use. `make dev-agentctl` is equivalent and supports `AGENTCTL_RUNTIME=opencode|claude-code|codex` and additional flags through `AGENTCTL_ARGS`. The service has no authentication and defaults to loopback; expose it only through a trusted platform boundary. See the [agentctl API guide](docs/agentctl-api.md) for REST endpoints, prompt delivery, the WebSocket stream, and recovery behavior.

## Development

Install dependencies and run the standard checks with:

```sh
make setup
make check
```

Common commands:

| Command | Purpose |
| --- | --- |
| `make build` | Build `bin/orkest`, `bin/agentctl`, and `web/dist`. |
| `make lint` | Run the pinned Go linters and formatting checks. |
| `make fmt` | Apply the configured Go formatting. |
| `make check` | Run runtime-boundary checks, Go tests/lint, frontend generated-code checks, frontend lint/tests/type checks, and builds. |
| `make test-postgres` | Run PostgreSQL repository tests against a configured database. |
| `make test-integration` | Run integration tests; requires `ORKEST_OPENCODE_BINARY`. |
| `make test-runtime-conformance` | Run deterministic provider and recovery contracts with race detection. |
| `make test-recovery-unit` | Run deterministic session, queue, and recovery tests with race detection. |
| `make test-recovery` | Add real OpenCode continuation testing; requires an explicit model, credentials, and provider environment. |
| `npm --prefix web run test:e2e` | Run Playwright desktop/mobile journeys; install Chromium first with `web/node_modules/.bin/playwright install chromium`. |
| `make sqlc` | Regenerate PostgreSQL query code from `db/queries`. |

Frontend API artifacts are generated from the platform OpenAPI contract. After changing that contract, run `npm --prefix web run generate:api`, then `make check`.

## Contributing

Before making changes, read [AGENTS.md](AGENTS.md), the [requirements](docs/requirements.md), and the [technology stack](docs/tech-stack.md). Keep changes focused, preserve confirmed/proposed distinctions, pin dependencies, and never commit secrets. Add tests beside changed Go packages or under `web/` as appropriate, and update documentation when behavior or commands change.

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, verification, pull request, and licensing guidance.

## Documentation

- [Project requirements](docs/requirements.md)
- [Technology stack and architecture](docs/tech-stack.md)
- [Management webapp](docs/webapp-implementation.md)
- [`agentctl` setup and limitations](docs/agentctl.md)
- [`agentctl` REST/WebSocket API](docs/agentctl-api.md)
- [Runtime providers](docs/runtime-providers.md)
- [Durable task sessions](docs/task-sessions.md)
- [Execution profiles](docs/execution-profiles.md)
- [Docker executor](docs/executors/docker.md)
- [Local executor](docs/executors/local.md)
- [Provider guides](docs/providers/)

## Project status and license

Orkest is under active development. The [requirements document](docs/requirements.md) records confirmed scope, proposed behavior, recovery requirements, and open decisions. In particular, real model execution and continuation qualification require provider credentials and are not implied by deterministic tests or session loading alone.

Orkest is licensed under the [Apache License 2.0](LICENSE.md).
