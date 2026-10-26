# Orkest technology stack

Status: Mixed. This document distinguishes implemented technology from the proposed MVP stack. Dependency versions listed for implemented components are pinned; unimplemented choices remain proposals or open decisions.

## Backend

- **Go 1.26.2** for the core platform and `agentctl`.
- **`net/http`** owns both servers. The platform currently routes with the standard library `ServeMux`; the `agentctl` REST and WebSocket APIs use **Chi v5.2.3** on top of `net/http`.
- **Manual constructor dependency injection** with wiring at the application boundary. Services receive explicit dependencies and do not look them up through a container. No DI framework is needed for the MVP; reconsider Uber Fx if wiring and lifecycle management become cumbersome.
- **`log/slog`** for structured logging, with credentials redacted.
- **`godotenv v1.5.1`** loads the optional local `.env` file through shared helpers in `internal/core/environment`; both command entry points use them.
- **OpenAPI 3.1 YAML contracts** live inside their API packages: `internal/app/httpapi/openapi.yaml` (platform/orchestrator) and `internal/agentctl/httpapi/openapi.yaml` (`agentctl`). The shared core OpenAPI package validates and serves each contract at server startup. **kin-openapi v0.133.0** parses and validates the contracts; **swaggest/swgui v1.8.9** serves embedded Swagger UI assets, so the UI does not depend on a CDN.

The Makefile pins **golangci-lint v2.15.0** and enables the **gofumpt** formatter. Go and frontend dependencies are locked with `go.sum` and `web/package-lock.json`.

The platform API includes authentication, internal user registration, organization/workspace CRUD, membership management, workspace-scoped agent definition CRUD, and workspace-scoped code repository registration. These features use separate identity, organization, workspace, agent, and code repository domain packages with independent PostgreSQL repository implementations. The application stores durable workspace files below `ORKEST_DATA_DIR/<organizationID>/<workspaceID>/`; code checkouts are stored at `git/<providerName>/<repositoryID>` below that path. The data root defaults to `./data` and should be mounted to persistent disk in deployed environments. The code repository provider contract currently has a GitHub implementation for public HTTPS repositories. Clone and sync derive their destination from the workspace and repository records; sync fetches remote `main` and hard-resets the checkout, discarding local changes. Authenticated/private repository access and other providers are not implemented. Platform agent definitions and agentctl run configuration share the runtime enum in `internal/core/runtime`; currently OpenCode is the only available runtime. Model provider/model identifiers remain separate. `internal/orchestration` now defines the executor-facing agentctl launcher, persists launch lifecycle and ordered events, configures/starts agentctl through its HTTP API, and maintains a replayable WebSocket connection. A concrete executor and task/run start flow are not yet wired into the platform.

## Database and background work

- **PostgreSQL 18.3** (`postgres:18.3-alpine3.23`) runs through Compose for local development and backs the platform's identity, organization, and workspace repositories.
- **`pgx v5.11.0`**, **Goose v3.29.0**, **sqlc v1.31.1**, and versioned SQL migrations back internal users, organizations, workspaces, and memberships. `PostgresDatabase.Initialize` accepts either `DATABASE_URL` or the complete `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USERNAME`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` set; the URL takes precedence. It owns the shared pool and migration lifecycle and retains non-secret connection details. `UserPostgresRepository`, `OrganizationPostgresRepository`, and `WorkspacePostgresRepository` implement their corresponding domain-owned repository contracts. All static repository queries live in domain-specific files under `db/queries` and execute through sqlc-generated pgx methods. Repositories map generated rows into domain types and own explicit transaction orchestration using `Queries.WithTx`; membership changes retain their row locks and authorization checks. Direct pgx query execution is reserved for concrete needs not served by generated static queries, such as bulk copy or dynamic SQL. Redis and a separate message broker are not part of the proposed initial stack.

The first Goose migration safely replays the schema created by the initial custom runner. A follow-up migration removes only that runner's `schema_migrations` tracking table after Goose records the schema version; application tables and data are retained.

For task and run persistence, use transactions and constraints for atomic member-slot allocation and state transitions. Persist command identities and results so reconnects can reconcile uncertain outcomes before retrying.

## Frontend

- **React 19.2.0**, **TypeScript 5.9.3**, and **Vite 7.3.7** are pinned in `web/package.json`; the current frontend is a static shell and does not call the API.
- The documented local toolchain is **Node.js 22.19.0** with npm 11; `package.json` requires Node.js 22.12.0 or later.
- Browser HTTP commands and **Server-Sent Events (SSE)** with persisted cursors and replay remain proposed. The implemented agentctl activity stream is WebSocket, described below.

## Authentication and authorization

- **Keycloak through OpenID Connect (OIDC)** authenticates browser sessions and bearer tokens, including configured identity providers and SSO.
- **Application-owned authorization** for organization/workspace memberships and roles. First login creates an internal profile keyed by the verified subject; later logins do not overwrite its username/email. Owner/Admin organization roles inherit workspace access, and workspace roles are enforced by the platform API.

Keycloak authenticates users while Orkest enforces access boundaries. Organization and workspace API authorization is implemented; event streams, integrations, artifacts, and execution controls must apply the same membership checks as those features are added.

## Agent runtime and execution

- **OpenCode v2.1.4** is the pinned runtime in the `agentctl` prototype. Its compatibility with the real executable still requires integration verification.
- **ACP over local stdio** uses `github.com/coder/acp-go-sdk v0.13.5` between `agentctl` and the OpenCode subprocess.
- The standalone **Go `agentctl` binary** owns the subprocess and ACP connection. It exposes REST commands and a receive-only activity WebSocket using **`github.com/coder/websocket v1.8.15`**. It journals events and retains task-scoped runtime state.
- The agentctl HTTP server uses **Chi v5.2.3**; WebSocket upgrades use `coder/websocket`. Platform startup reconciles persisted launches, closes confirmed-ended or unconfigured processes, and reconnects healthy endpoints to replay events from the stored sequence. Stable command reconciliation, agentctl restart recovery, session restoration, and continuation are not implemented.
- The first **Docker Engine executor** is implemented in `internal/executors/docker` with the pinned Moby Go client v0.6.2. It starts agentctl containers with task-scoped durable host bind mounts and loopback-only dynamic port publication, then gracefully stops and removes containers after terminal prompt events. It targets a local Engine; permanent task/run API wiring and repository preparation are not implemented.
- Docker image build targets create a shared agentctl base image and an OpenCode 2.1.4 runtime image. The mounted work and agentctl state directories persist on the Docker host. Container restart recovery and OpenCode session continuation are not implemented.
- An **authenticated platform-to-agentctl transport** remains proposed and unimplemented. The current local transport uses persisted run identities and event cursors for platform restart reconnect and replay, without transport authentication or explicit acknowledgements.

The Go ACP library is selected, but real-runtime protocol compatibility, approvals, session restoration, and interrupted-work continuation are not yet verified.

The conversation service separates ordered event commits from its in-memory state lock, so journal writes and syncs do not block status reads or shutdown admission. Event-producing commands still serialize behind in-flight commits; final shutdown joins outstanding work and storage I/O. The journal owns sequence numbers and commit timestamps. A persistence failure fences the conversation and cancels local prompt/permission waits without selecting an approval option. Local cancellation does not prove runtime termination or permit replacement; reconciliation remains required.

Runtime failures retain wrapped diagnostic causes and subprocess exit status in memory. Error strings and API messages remain sanitized; raw causes and runtime output must not be written to ordinary logs. Unexpected unsuccessful process exits are reported as failed unless the conversation is already interrupted. This does not implement restart recovery.

Core platform shutdown must disconnect from agents without terminating their work. Agent environments retain independent lifecycles. Container/process restart recovery assumes durable task storage remains intact; complete executor-host or disk loss recovery is deferred by the MVP requirements.

## Tools and integrations

- **HTTP MCP connections** for external agent capabilities; stdio MCP servers are outside the MVP. No MCP client or server implementation exists yet.
- **Official MCP Go SDK** is proposed where the platform implements MCP clients or servers. Whether built-in platform tools use an internal HTTP MCP endpoint remains open.
- **GitHub Cloud Projects v2 and Jira Server** are task providers behind the `internal/taskintegration` contract. Workspace configurations are stored in PostgreSQL, credentials are AES-GCM encrypted using a deployment-managed 32-byte hex `ORKEST_INTEGRATION_ENCRYPTION_KEY`, and a platform lifecycle runner polls enabled sources using their configured interval (five-minute default). GitHub Cloud sources use a project board URL and GraphQL; GitHub documents classic PATs with `read:project` or GitHub App installation tokens for querying Projects. Jira sources use configured project keys. API responses exclude credentials.
- Separate interfaces for **task providers, executors, and agent runtimes** so provider-specific behavior stays outside core orchestration.

Agents create and update pull requests through their permitted runtime capabilities and configured integrations. Imported issue content remains source-managed; polling removes imported tasks no longer present in the selected source or matching its filters, and moves tasks for closed source issues to done. Orkest owns local workflow and execution state for all other transitions.

## Architecture and local development

- **Modular monolith** is the proposed core platform shape. Implemented Go packages cover identity, organization, workspace, manual task and component APIs, GitHub Cloud and Jira task integrations, app lifecycle, launch orchestration, Docker agentctl execution, and standalone agentctl/OpenCode runtime code. Task persistence uses versioned PostgreSQL migrations and sqlc; task execution wiring remains unimplemented.
- **Independent `agentctl` process** is implemented as a separate binary and can be packaged into the Docker OpenCode runtime image; a task-start API and platform wiring remain future work.
- **Go code organization** stays in one root Go module, with logical package areas rather than nested Go modules:
  - `internal/app` owns platform wiring and lifecycle; platform features belong under `internal/` packages such as `identity`, `organization`, `workspace`, `task`, and `orchestration`.
  - `internal/agentctl` owns agentctl run state, provider contracts, transport APIs, daemon lifecycle (`internal/agentctl/daemon`), and runtime adapters (`internal/agentctl/runtime`). It runs independently of platform lifecycle.
  - `internal/core` holds only concrete code shared by platform and agentctl, including environment loading (`internal/core/environment`), graceful HTTP server lifecycle (`internal/core/httpserver`), and OpenAPI serving/validation (`internal/core/openapi`).
  - `internal/postgres` owns the shared PostgreSQL database lifecycle and separate per-domain adapters; repository interfaces remain in their domain packages.
  - `cmd/orkest` and `cmd/agentctl` remain thin executable wiring; platform HTTP lives in `internal/app/httpapi` and agentctl HTTP lives in `internal/agentctl/httpapi`. `internal/core/openapi` owns shared serving and validation code.
- **Docker Compose** currently starts PostgreSQL only. Keycloak and the platform are not Compose services.

Avoid introducing microservices or a distributed workflow engine for the initial implementation. Durable execution state, ownership, event replay, and reconciliation are explicit application responsibilities.

## Validation status and next milestones

The Go agentctl/OpenCode prototype exists. Unit tests use a subprocess fixture; the real OpenCode handshake has a documented integration target but remains unverified unless run with the pinned executable. Before building the full UI, validate the remaining high-risk behavior:

1. Run `make test-docker` to verify agentctl starts in the Docker OpenCode image; validate OpenCode ACP interaction and subsequent recovery scenarios separately.
2. Completes a user approval round trip.
3. Keeps the agent running across a core platform restart and replays missed events.
4. Restores files and the supported agent session after a container/process restart.
5. Reconciles execution and command identities without duplicate work.

The recovery and approval scenarios remain acceptance work; a successful subprocess fixture or session reload alone does not prove real runtime continuation.

`make test-integration` runs the real OpenCode handshake when the pinned executable is supplied. `make test-postgres` runs domain repository integration tests using either PostgreSQL configuration form (start local PostgreSQL with `make db-up`). `make test-recovery` currently reports that the suite is not implemented.

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
