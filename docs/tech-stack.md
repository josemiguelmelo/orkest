# Orkest technology stack

Status: Mixed. This document distinguishes implemented technology from the proposed MVP stack. Dependency versions listed for implemented components are pinned; unimplemented choices remain proposals or open decisions.

## Backend

- **Go 1.26.2** for the core platform and `agentctl`.
- **`net/http`** owns both servers. The platform currently routes with the standard library `ServeMux`; the `agentctl` REST and WebSocket APIs use **Chi v5.2.3** on top of `net/http`.
- **Manual constructor dependency injection** with wiring at the application boundary. Services receive explicit dependencies and do not look them up through a container. No DI framework is needed for the MVP; reconsider Uber Fx if wiring and lifecycle management become cumbersome.
- **`log/slog`** for structured logging, with credentials redacted.
- **`godotenv v1.5.1`** loads the optional local `.env` file for `agentctl` configuration.
- **OpenAPI 3.1 YAML contracts** live inside their API packages: `internal/app/httpapi/openapi.yaml` (platform/orchestrator) and `internal/agentctl/httpapi/openapi.yaml` (`agentctl`). The shared core OpenAPI package validates and serves each contract at server startup. **kin-openapi v0.133.0** parses and validates the contracts; **swaggest/swgui v1.8.9** serves embedded Swagger UI assets, so the UI does not depend on a CDN.

The Makefile pins **golangci-lint v2.15.0** and enables the **gofumpt** formatter. Go and frontend dependencies are locked with `go.sum` and `web/package-lock.json`.

The platform API is currently a liveness endpoint and OpenAPI/Swagger UI. Product services and platform-to-agent communication are not implemented.

## Database and background work

- **PostgreSQL 18.3** (`postgres:18.3-alpine3.23`) currently runs through Compose for local development. The application does not connect to it yet.
- **`pgx` + `sqlc`**, versioned migrations, and PostgreSQL-backed jobs/commands remain proposed; none are implemented or pinned. Redis and a separate message broker are not part of the proposed initial stack.

When database access is implemented, use transactions and constraints for atomic member-slot allocation and state transitions. Persist command identities and results so reconnects can reconcile uncertain outcomes before retrying.

## Frontend

- **React 19.2.0**, **TypeScript 5.9.3**, and **Vite 7.3.7** are pinned in `web/package.json`; the current frontend is a static shell and does not call the API.
- The documented local toolchain is **Node.js 22.19.0** with npm 11; `package.json` requires Node.js 22.12.0 or later.
- Browser HTTP commands and **Server-Sent Events (SSE)** with persisted cursors and replay remain proposed. The implemented agentctl activity stream is WebSocket, described below.

## Authentication and authorization

- **Keycloak through OpenID Connect (OIDC)** for authentication, including configured identity providers and SSO; this remains proposed and is not implemented.
- **Application-owned authorization** for organization/workspace memberships and roles.

Under the proposed design, Keycloak authenticates users while Orkest enforces access boundaries across APIs, event streams, integrations, artifacts, and execution controls. The detailed role matrix remains to be defined.

## Agent runtime and execution

- **OpenCode v2.1.4** is the pinned runtime in the `agentctl` prototype. Its compatibility with the real executable still requires integration verification.
- **ACP over local stdio** uses `github.com/coder/acp-go-sdk v0.13.5` between `agentctl` and the OpenCode subprocess.
- The standalone **Go `agentctl` binary** owns the subprocess and ACP connection. It exposes REST commands and a receive-only activity WebSocket using **`github.com/coder/websocket v1.8.15`**. It journals events and retains task-scoped runtime state.
- The agentctl HTTP server uses **Chi v5.2.3**; WebSocket upgrades use `coder/websocket`. Stable command reconciliation, restart recovery, session restoration, and continuation are not implemented.
- **Docker Engine Go SDK** and the executor abstraction remain proposed; there is no task-container provisioning implementation yet.
- **Retained Docker volumes or equivalent durable task storage** for repository working trees, runtime sessions, configuration identity, events, and pending interaction metadata remain proposed. The current prototype expects durable host directories for its state and OpenCode HOME/XDG data.
- An **authenticated platform-to-agentctl transport** with stable identities, acknowledgements, replay, and reconnect reconciliation remains proposed and unimplemented.

The Go ACP library is selected, but real-runtime protocol compatibility, approvals, session restoration, and interrupted-work continuation are not yet verified.

Core platform shutdown must disconnect from agents without terminating their work. Agent environments retain independent lifecycles. Container/process restart recovery assumes durable task storage remains intact; complete executor-host or disk loss recovery is deferred by the MVP requirements.

## Tools and integrations

- **HTTP MCP connections** for external agent capabilities; stdio MCP servers are outside the MVP. No MCP client or server implementation exists yet.
- **Official MCP Go SDK** is proposed where the platform implements MCP clients or servers. Whether built-in platform tools use an internal HTTP MCP endpoint remains open.
- **GitHub Issues** as the first task-provider integration, accessed through a provider-independent integration contract; no provider implementation exists yet.
- Separate interfaces for **task providers, executors, and agent runtimes** so provider-specific behavior stays outside core orchestration.

Agents create and update pull requests through their permitted runtime capabilities and configured integrations. Imported issue content remains source-managed, while Orkest owns local workflow and execution state.

## Architecture and local development

- **Modular monolith** is the proposed core platform shape. Implemented Go packages currently cover app lifecycle, HTTP API scaffolding, and standalone agentctl/OpenCode runtime code; identity, workspace, task, orchestration, and integration services are not implemented.
- **Independent `agentctl` process** is implemented as a separate binary, but deployment inside executor environments is not.
- **Go code organization** stays in one root Go module, with logical package areas rather than nested Go modules:
  - `internal/app` owns platform wiring and lifecycle; platform features belong under `internal/` packages such as `identity`, `workspace`, `task`, and `orchestration`.
  - `internal/agentctl` owns agentctl run state, provider contracts, transport APIs, daemon lifecycle (`internal/agentctl/daemon`), and runtime adapters (`internal/agentctl/runtime`). It runs independently of platform lifecycle.
  - `internal/core` holds only concrete code shared by platform and agentctl, currently graceful HTTP server lifecycle (`internal/core/httpserver`).
  - `cmd/orkest` and `cmd/agentctl` remain thin executable wiring; platform HTTP lives in `internal/app/httpapi` and agentctl HTTP lives in `internal/agentctl/httpapi`. `internal/core/openapi` owns shared serving and validation code.
- **Docker Compose** currently starts PostgreSQL only. Keycloak and the platform are not Compose services.

Avoid introducing microservices or a distributed workflow engine for the initial implementation. Durable execution state, ownership, event replay, and reconciliation are explicit application responsibilities.

## Validation status and next milestones

The Go agentctl/OpenCode prototype exists. Unit tests use a subprocess fixture; the real OpenCode handshake has a documented integration target but remains unverified unless run with the pinned executable. Before building the full UI, validate the remaining high-risk behavior:

1. Launch OpenCode in a Docker task environment and exchange ACP messages.
2. Completes a user approval round trip.
3. Keeps the agent running across a core platform restart and replays missed events.
4. Restores files and the supported agent session after a container/process restart.
5. Reconciles execution and command identities without duplicate work.

The recovery and approval scenarios remain acceptance work; a successful subprocess fixture or session reload alone does not prove real runtime continuation.

`make test-integration` runs the real OpenCode handshake when the pinned executable is supplied. `make test-recovery` currently reports that the suite is not implemented.

## References

- [Project requirements](requirements.md)
- [Chi](https://github.com/go-chi/chi)
- [kin-openapi](https://github.com/getkin/kin-openapi)
- [swaggest/swgui](https://github.com/swaggest/swgui)
- [pgx](https://github.com/jackc/pgx)
- [sqlc](https://sqlc.dev/)
- [Docker Engine SDK](https://docs.docker.com/reference/api/engine/sdk/)
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- [ACP protocol and libraries](https://github.com/agentclientprotocol/agent-client-protocol)
