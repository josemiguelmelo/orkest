# Orkest

[![Runtime conformance](https://github.com/josemiguelmelo/orkest/actions/workflows/runtime-conformance.yml/badge.svg)](https://github.com/josemiguelmelo/orkest/actions/workflows/runtime-conformance.yml)

Orkest is a self-hosted platform for coordinating software-development agents.
Teams can manage tasks and repositories, start agent runs, follow activity and chat,
answer approval requests, and review the resulting work. A Go API serves the React
web app, while independent `agentctl` workers run coding agents through ACP.

Start with the [local setup](#run-locally), browse the [documentation map](docs/README.md),
or check the [implementation and verification status](docs/status.md) before choosing
an execution mode.

Orkest is an active prototype. Task management, dispatch, scheduled jobs, and several
execution modes are implemented. Team-member capacity enforcement and platform-to-worker
authentication remain unfinished. Runtime verification varies by provider and scenario;
deterministic fixtures do not establish live model-driven continuation. See the
[status and verification record](docs/status.md#verification-record) for current evidence
and limits.

## Features

- Organization and workspace management with Keycloak/OIDC authentication and
  application-owned roles and authorization.
- Tasks, comments, repositories, workflow rules, artifacts, and direct agent conversations.
- Workflow-independent scheduled jobs with timezone-aware calendars, run history, and durable activity.
- Workspace agent definitions, runtime profiles, skills, HTTP and stdio MCP servers, and environment values.
- GitHub Projects v2 and Jira Server task imports; GitHub and GitLab coding integrations.
- OpenCode, Claude Code, Codex, GitHub Copilot CLI, and Kiro CLI through independent workers.
- Ordered activity, chat, approvals, cancellation, and durable PostgreSQL/session history.
- Docker execution, initial in-cluster Kubernetes execution, optional toolchain
  image builds, and trusted Local execution.

## Prerequisites

- Go matching [go.mod](go.mod).
- Node.js matching [web/.node-version](web/.node-version) and
  [web/package.json](web/package.json), plus npm 11.
- Docker Engine and Compose v2 for local PostgreSQL and Keycloak.
- Python 3 for runtime-boundary checks and selected tooling.

- Docker agents need Engine socket access; Local needs pinned host runtime/dependencies.
- Kubernetes prerequisites and current limits are in the [executor reference](docs/reference/executors/kubernetes.md).

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

- Open <http://127.0.0.1:8080>; `/auth/login` uses imported Keycloak credentials `local` / `local`.
- `make dev`: Air backend reload; `make dev-once`: one run. Rebuild `web/dist` after
  frontend changes or use [Vite](docs/development/webapp.md#development).
- Settings: `.env.example`, overridden by shell variables; [networking/credentials](docs/operations/configuration.md).
- Before agents: build [Docker images](docs/reference/executors/docker.md#build-and-configure-images)
  or configure [Local](docs/reference/executors/local.md#setup), which provides no task
  isolation and is for trusted single-user development. [Compose setup](docs/guides/local-setup.md).
- `make db-down` retains PostgreSQL volume. Initialization settings affect only new
  volumes; removing a volume deletes its data.

## Run with Compose

To run the platform and its infrastructure in containers, configure `.env` as above
and use the [Compose setup guide](docs/guides/local-setup.md):

```sh
docker compose up --build
```

This starts PostgreSQL, Keycloak, and the local registry. To include the Orkest
platform container, add `--profile orkest`. Build and publish the
[runtime images](docs/reference/executors/docker.md#build-and-configure-images)
before running tasks.

## First task

1. Create organization/workspace; register repositories; define an agent.
2. Create/assign task; assignment never starts work.
3. Start explicitly or through [workflow rules](docs/guides/workflows.md).
4. View history/activity, chat, answer approvals, or cancel in task details.

To run repeatable agent work outside the task board, use [scheduled jobs](docs/guides/scheduled-jobs.md).

API authentication: browser session or verified Keycloak bearer token.

| Endpoint | Purpose |
| --- | --- |
| `GET /healthz` | Public liveness check; returns `{"status":"ok"}` |
| `GET /api/me` | Authenticated user and memberships |
| `GET /docs/` | Interactive platform API documentation |
| `GET /openapi.yaml` | Platform OpenAPI contract |

For standalone worker use, see [agentctl setup](docs/guides/agentctl.md).
Never put real credentials in task content, logs, commits, or examples.

## Development and documentation

```sh
make check
```

- `make build`: both Go binaries/frontend; `make fmt`: Go formatting.
- [Testing](docs/development/testing.md): checks, prerequisites, API/SQL regeneration.
- [Architecture](docs/architecture.md): `cmd/server` builds `bin/orkest`; `cmd/agentctl`
  builds `bin/agentctl`. Server domains, worker domains, adapters, transport values,
  and technical utilities have separate package boundaries.

- Before contributing: [AGENTS.md](AGENTS.md), [requirements](docs/requirements.md),
  [stack](docs/tech-stack.md), [CONTRIBUTING.md](CONTRIBUTING.md) (PR/licensing guidance).
- Keep changes focused; pin dependencies; preserve confirmed/proposed distinctions;
  test changed behavior; update docs.
- [Documentation map](docs/README.md): guides, architecture, references, operations.

Licensed under the [Apache License 2.0](LICENSE.md).
