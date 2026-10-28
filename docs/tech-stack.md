# Orkest technology stack

Status: Mixed. This document distinguishes implemented technology from the proposed MVP stack. Dependency versions listed for implemented components are pinned; unimplemented choices remain proposals or open decisions.

## Backend

- **Go 1.26.2** for the core platform and `agentctl`.
- **`net/http`** owns both servers. The platform and the `agentctl` REST/WebSocket APIs use **Chi v5.2.3** on top of `net/http`.
- **Manual constructor dependency injection** with wiring at the application boundary. Services receive explicit dependencies and do not look them up through a container. No DI framework is needed for the MVP; reconsider Uber Fx if wiring and lifecycle management become cumbersome.
- **`log/slog`** for structured logging, with credentials redacted.
- **Structured request correlation** assigns a validated 32-character `trace_id` to each inbound request, includes it in request and backend error logs, and propagates it over platform-to-agentctl HTTP calls using `X-Trace-ID`. Request summaries are emitted for server errors and requests slower than two seconds; routine successes are omitted. This is a log correlation ID; no tracing SDK or span exporter is configured.
- **`godotenv v1.5.1`** loads the optional local `.env` file through shared helpers in `internal/core/environment`; both command entry points use them.
- **OpenAPI 3.1 YAML contracts** live inside their API packages: `internal/app/httpapi/openapi.yaml` (platform/orchestrator) and `internal/agentctl/httpapi/openapi.yaml` (`agentctl`). The shared core OpenAPI package validates and serves each contract at server startup. **kin-openapi v0.133.0** parses and validates the contracts; **swaggest/swgui v1.8.9** serves embedded Swagger UI assets, so the UI does not depend on a CDN.

The Makefile pins **golangci-lint v2.15.0** and enables the **gofumpt** formatter. Go and frontend dependencies are locked with `go.sum` and `web/package-lock.json`.

The platform API includes authentication, internal user registration, organization/workspace CRUD, membership management, workspace-scoped agent definition CRUD, and workspace-scoped code repository registration. These features use separate identity, organization, workspace, agent, and code repository domain packages with independent PostgreSQL repository implementations. The application stores durable workspace files below `ORKEST_DATA_DIR/<organizationID>/<workspaceID>/`; shared management checkouts live at `git/<providerName>/<repositoryID>` below that path. At task start, orchestration resolves all registered repositories in the task workspace and the executor prepares isolated task checkouts beneath the durable task work directory. The agent works from the common workspace directory, with each repository on its task-specific branch; review iterations reuse those checkouts. The data root defaults to `./data` and should be mounted to persistent disk in deployed environments. A `CodingProvider` contract and initial GitHub provider define provider schemas, authentication checks, paginated discovery, selection resolution, URL normalization, and credential-explicit checkout operations. Workspace coding integrations are encrypted at rest and managed through workspace APIs; repository registration resolves the provider selection on the server. The executor receives an integration reference separately from public run configuration and resolves the encrypted credential immediately before task checkout. The frontend includes integration setup and a paginated repository picker. Existing registrations without an integration continue through the legacy public-checkout adapter. The database migration adds encrypted integration storage and repository identity/default-branch references; rollback requires exporting integration-backed repository registrations because rollback removes those fields. Additional coding providers and automated pull-request creation remain future support. The agentctl base image includes pinned GitHub CLI 2.103.2. Agentctl receives integrated repository PATs in run configuration, with platform session snapshots encrypted and agentctl plaintext checkpoints omitting token values. It applies the fixed Git identity `Orkest Agent <agent@orkest.local>` per checkout and supplies URL-scoped HTTPS authorization through Git process configuration, not `.git/config`. Platform agent definitions and agentctl run configuration share the runtime enum in `internal/core/runtime`; currently OpenCode is the only available runtime. Model provider/model identifiers remain separate. `internal/orchestration` persists launch lifecycle and ordered events, configures/starts agentctl through its HTTP API, and maintains a replayable WebSocket connection. The Docker executor and task start flow are wired into the platform.

The platform API exposes the skill catalog at global, organization, and workspace scope. It is implemented as a separate `internal/skill` domain package with PostgreSQL persistence. Global administrator access is stored as `users.platform_admin`. After the user has logged in, an operator can provision access with `UPDATE users SET platform_admin = true WHERE id = '<internal-user-id>';`; use `false` to revoke it.

## Database and background work

- **PostgreSQL 18.3** (`postgres:18.3-alpine3.23`) runs through Compose for local development and backs the platform's identity, organization, and workspace repositories.
- **`pgx v5.11.0`**, **Goose v3.29.0**, **sqlc v1.31.1**, and versioned SQL migrations back internal users, organizations, workspaces, and memberships. `PostgresDatabase.Initialize` accepts either `DATABASE_URL` or the complete `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USERNAME`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` set; the URL takes precedence. It owns the shared pool and migration lifecycle and retains non-secret connection details. `UserPostgresRepository`, `OrganizationPostgresRepository`, and `WorkspacePostgresRepository` implement their corresponding domain-owned repository contracts. All static repository queries live in domain-specific files under `db/queries` and execute through sqlc-generated pgx methods. Repositories map generated rows into domain types and own explicit transaction orchestration using `Queries.WithTx`; membership changes retain their row locks and authorization checks. Direct pgx query execution is reserved for concrete needs not served by generated static queries, such as bulk copy or dynamic SQL. Redis and a separate message broker are not part of the proposed initial stack.

The first Goose migration safely replays the schema created by the initial custom runner. A follow-up migration removes only that runner's `schema_migrations` tracking table after Goose records the schema version; application tables and data are retained.

For task and run persistence, use transactions and constraints for atomic member-slot allocation and state transitions. Persist command identities and results so reconnects can reconcile uncertain outcomes before retrying.

## Frontend

- **React 19.2.0**, **TypeScript 5.9.3**, and **Vite 7.3.7** are pinned in `web/package.json`; the frontend now implements the platform management workbench.
- The documented local toolchain is **Node.js 22.22.2** with npm 11; `package.json` requires Node.js 22.22.2, 24.15.0, or supported 26+ releases.
- The management UI uses **TanStack Router/Query**, **React Hook Form/Zod**, **Tailwind v4**, and owned shadcn-style **Radix** components. All added dependency versions are pinned in the npm manifest/lockfile. Hey API generates typed SDK functions and runtime validators from the platform contract. Strict application types, scoped caches, role-aware actions, schema-driven integration forms, frontend lint/tests and desktop/mobile browser journeys are implemented. See [webapp architecture and verification](webapp-implementation.md).
- Go serves the SPA from `ORKEST_WEB_DIR` (default `web/dist`); Vite proxies the API and cookie-based OIDC routes during development. Task detail starts assigned work through the task workflow and exposes scoped run history, activity, chat, permission responses, and cancellation through the platform API.
- Browser HTTP commands and **Server-Sent Events (SSE)** with persisted cursors and replay remain proposed. The implemented agentctl activity stream is WebSocket, described below.

## Authentication and authorization

- **Keycloak through OpenID Connect (OIDC)** authenticates browser sessions and bearer tokens, including configured identity providers and SSO.
- **Application-owned authorization** for organization/workspace memberships and roles. First login creates an internal profile keyed by the verified subject; later logins do not overwrite its username/email. Owner/Admin organization roles inherit workspace access, and workspace roles are enforced by the platform API.

Keycloak authenticates users while Orkest enforces access boundaries. Organization and workspace API authorization is implemented; event streams, integrations, artifacts, and execution controls must apply the same membership checks as those features are added.

## Agent runtime and execution

- **OpenCode v2.1.4** is the pinned runtime in the `agentctl` prototype. Real handshake and session loading are verified; real model continuation and approval round trips still require credentialed validation.
- **ACP over local stdio** uses `github.com/coder/acp-go-sdk v0.13.5` between `agentctl` and the OpenCode subprocess.
- The standalone **Go `agentctl` binary** owns the subprocess and ACP connection. It exposes REST commands and a receive-only activity WebSocket using **`github.com/coder/websocket v1.8.15`**. It journals events and retains task-scoped runtime state.
- The agentctl HTTP server uses **Chi v5.2.3**; WebSocket upgrades use `coder/websocket`. Platform startup reconciles persisted launches, closes confirmed-ended or unconfigured processes, and reconnects healthy endpoints to replay events from the stored sequence. Durable task sessions implement command reconciliation, agentctl restart recovery, session loading, and conservative continuation. Uncertain effects pause for user reconciliation.
- The first **Docker Engine executor** is implemented in `internal/executors/docker` with the pinned Moby Go client v0.6.2. It starts agentctl containers with task-scoped durable host bind mounts and loopback-only dynamic port publication, then gracefully stops and removes containers after terminal run events. Task/run API wiring and repository preparation are implemented; team-member slot allocation is separate unfinished work.
- Docker image build targets create a shared agentctl base image and an OpenCode 2.1.4 runtime image. The mounted work and agentctl state directories persist on the Docker host. Session-enabled containers restore durable sessions after restart and preserve task files. Real model continuation remains separately gated on explicit model configuration.
- An **authenticated platform-to-agentctl transport** remains proposed and unimplemented. The current local transport uses persisted run identities and event cursors for platform restart reconnect and replay, without transport authentication or explicit acknowledgements.

Real OpenCode handshake, session loading, daemon/container restoration, and retained working files have integration coverage. Real model-memory continuation and approval round trips remain unverified without explicit model configuration and credentials.

The conversation service separates ordered event commits from its in-memory state lock, so journal writes and syncs do not block status reads or shutdown admission. Event-producing commands still serialize behind in-flight commits; final shutdown joins outstanding work and storage I/O. The journal owns sequence numbers and commit timestamps. A persistence failure fences the conversation and cancels local prompt/permission waits without selecting an approval option. Local cancellation does not prove runtime termination or permit replacement; reconciliation remains required.

Runtime failures retain wrapped diagnostic causes and subprocess exit status in memory. Error strings and API messages remain sanitized; raw causes and runtime output must not be written to ordinary logs. Unexpected unsuccessful process exits are reported as failed unless the conversation is already interrupted. Durable sessions use these states to trigger restoration or expose blocked recovery.

Core platform shutdown must disconnect from agents without terminating their work. Agent environments retain independent lifecycles. Container/process restart recovery assumes durable task storage remains intact; complete executor-host or disk loss recovery is deferred by the MVP requirements.

## Tools and integrations

- **HTTP MCP connections** for external agent capabilities; workspace configuration is passed to the OpenCode runtime, which owns MCP client connections. Stdio MCP servers are outside the MVP.
- **Official MCP Go SDK** is proposed where the platform implements MCP clients or servers. Whether built-in platform tools use an internal HTTP MCP endpoint remains open.
- **GitHub Cloud Projects v2 and Jira Server** are task providers behind the `internal/taskintegration` contract. Provider-owned typed config schemas are exposed by the provider catalog API; validated provider configs are stored as JSONB. Credentials use the shared AES-GCM utility in `internal/core/secretbox` and a deployment-managed 32-byte hex `ORKEST_INTEGRATION_ENCRYPTION_KEY`. GitHub Cloud sources use a project board URL and GraphQL; GitHub documents classic PATs with `read:project` or GitHub App installation tokens for querying Projects. Jira sources use configured project keys. API responses exclude credentials. Coding integration credentials are not yet wired to API or checkout execution.
- Workspace environment entries are stored per workspace. Secret values and task-session environment snapshots use AES-GCM through the same `ORKEST_INTEGRATION_ENCRYPTION_KEY`; runtime values are sent to agentctl for run configuration and redacted from runtime callbacks.
- Separate interfaces for **task providers, executors, and agent runtimes** so provider-specific behavior stays outside core orchestration.

Agents create and update pull requests through their permitted runtime capabilities and configured integrations. Imported issue content remains source-managed; polling removes imported tasks no longer present in the selected source or matching its filters, and moves tasks for closed source issues to done. Orkest owns local workflow and execution state for all other transitions.

## Architecture and local development

- **Modular monolith** is the proposed core platform shape. Implemented Go packages cover identity, organization, workspace, manual task and component APIs, GitHub Cloud and Jira task integrations, app lifecycle, task dispatch and launch orchestration, Docker agentctl execution, and standalone agentctl/OpenCode runtime code. Task run history and events use the versioned PostgreSQL schema and sqlc. Team-member slot allocation remains unimplemented; session restoration is implemented, with real model continuation still requiring credentialed validation.
- **Independent `agentctl` process** is implemented as a separate binary and can be packaged into the Docker OpenCode runtime image; task-start APIs and platform wiring are implemented.
- **Go code organization** stays in one root Go module, with logical package areas rather than nested Go modules:
  - `internal/app` owns platform wiring and lifecycle; platform features belong under `internal/` packages such as `identity`, `organization`, `workspace`, `task`, and `orchestration`.
  - `internal/agentctl` is organized into domain subpackages: `config` for resolved settings, `interaction` for commands/approvals/activity, `runtime` for normalized contracts, and `run` for execution and recovery. Concrete hosting lives in `runtimehost`; transports, daemon lifecycle, and runtime adapters remain in `httpapi`, `daemon`, and `runtime/acp`/`runtime/opencode`. It runs independently of platform lifecycle.
  - `internal/core` holds only concrete code shared by platform and agentctl, including environment loading (`internal/core/environment`), graceful HTTP server lifecycle (`internal/core/httpserver`), and OpenAPI serving/validation (`internal/core/openapi`).
  - `internal/postgres` owns the shared PostgreSQL database lifecycle and separate per-domain adapters; repository interfaces remain in their domain packages.
  - `cmd/orkest` and `cmd/agentctl` remain thin executable wiring; platform HTTP lives in `internal/app/httpapi` and agentctl HTTP lives in `internal/agentctl/httpapi`. `internal/core/openapi` owns shared serving and validation code.
- **Docker Compose** currently starts PostgreSQL only. Keycloak and the platform are not Compose services.

Avoid introducing microservices or a distributed workflow engine for the initial implementation. Durable execution state, ownership, event replay, and reconciliation are explicit application responsibilities.

## Validation status and next milestones

Deterministic tests cover FIFO execution, restart replay, duplicate prevention,
paused queues, stale approvals, ownership, and continuation decisions. Real
OpenCode 2.1.4 handshake/session loading and Docker daemon/container restoration
have been exercised, including preserved task files and explicit new sessions.
PostgreSQL tests cover atomic task-run reservation, session scope, and prompt
outbox reconciliation. Fresh-schema migrations are tested separately.

Real model-memory continuation and real runtime approval round trips still need
explicit model configuration and credentials. Session loading alone does not
prove arbitrary interrupted work resumes exactly. Complete host/disk loss and
team-member slot allocation remain outside this implementation.

`make test-integration` verifies real handshake/loading when the pinned binary is
supplied. `make test-postgres` runs the repository integrations. `make test-docker`
checks daemon restart, container replacement, and fresh sessions. The deterministic
`make test-recovery-unit` uses race detection; `make test-recovery` additionally
requires an explicitly selected model and runs the real continuation test.

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

## Durable task sessions

Task-scoped session snapshots and command delivery intent use PostgreSQL/sqlc.
agentctl uses private versioned manifests, exclusive OS ownership locks, and
reopenable JSONL journals for runtime restoration and FIFO prompt execution.
OpenCode 2.1.4 / ACP v1 remains pinned. See [task sessions](task-sessions.md) for
implemented behavior, migration effects, verification commands, and recovery
limitations. Team-member capacity allocation remains unimplemented.

Agentctl's `runtimehost.Host` in `internal/agentctl/runtimehost` owns provisioning
application, process and pipe
lifecycle, session storage, ACP setup, recovery evidence, and partial-failure
cleanup. Run execution lives in `internal/agentctl/run` and depends on normalized
contracts in `internal/agentctl/runtime`, resolved configuration in `config`, and
command/approval/event definitions in `interaction`. Executable wiring
selects the concrete host. Checkpoints and session manifests share
`internal/core/durablefile` for atomic durable writes. Runtime profiles render
native configuration plans; compatible
protocols use the shared ACP implementation in `internal/agentctl/runtime/acp`.
The OpenCode 2.1.4 profile remains the only production profile. History coverage
is not qualified for safe interrupted-turn continuation, so restoration pauses
when the journal cannot establish the outcome. See [runtime host and ACP
architecture](runtime-providers.md) for compatibility, qualification, and
verification limits.
