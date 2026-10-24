# Orkest technology stack

Status: Proposed MVP stack based on the project requirements and initial technology discussion. Exact dependency versions must be selected and pinned during implementation.

## Backend

- **Go** for the core platform and `agentctl`.
- **Chi on top of `net/http`** for HTTP routing and middleware. This keeps standard Go HTTP compatibility while providing route groups and modular routing.
- **Manual constructor dependency injection** with wiring at the application boundary. Services receive explicit dependencies and do not look them up through a container. No DI framework is needed for the MVP; reconsider Uber Fx if wiring and lifecycle management become cumbersome.
- **`log/slog`** for structured logging, with credentials redacted.

Go fits the platform's concurrent executions, process supervision, streaming connections, and independently deployed agent bridge. HTTP framework throughput is not expected to be the main performance constraint; optimize based on measurements.

## Database and background work

- **PostgreSQL** for organizations, memberships, workspaces, tasks, configuration, run history, execution ownership, commands, and platform-side event records.
- **`pgx` + `sqlc`** for PostgreSQL access using explicit SQL and generated Go types.
- **Versioned SQL migrations**, with the migration tool selected during implementation.
- **PostgreSQL-backed background jobs and durable commands** initially. Redis and a separate message broker are not part of the initial stack.

Use transactions and database constraints for atomic member-slot allocation and state transitions. Persist command identities and results so reconnects can reconcile uncertain outcomes before retrying.

## Frontend

- **React + TypeScript + Vite** for the task board, configuration screens, execution activity, task chat, and approval UI.
- **HTTP requests** for commands such as starting work, sending messages, and answering approvals.
- **Server-Sent Events (SSE)** for streaming platform events to the browser, with persisted cursors and replay behavior defined by the application.

## Authentication and authorization

- **Keycloak through OpenID Connect (OIDC)** for authentication, including configured identity providers and SSO.
- **Application-owned authorization** for organization/workspace memberships and roles.

Keycloak authenticates users; Orkest enforces access boundaries across APIs, event streams, integrations, artifacts, and execution controls. The detailed role matrix remains to be defined.

## Agent runtime and execution

- **OpenCode** as the initial runtime, with a pinned release and configurable model provider/model identifier.
- **ACP over local stdio** between `agentctl` and the OpenCode subprocess.
- **Go `agentctl` binary** to own the agent process, ACP connection, durable event journal, and pending interactions independently of the core platform lifecycle.
- **Docker Engine Go SDK** for provisioning and managing isolated task containers behind an executor interface.
- **Retained Docker volumes or equivalent durable task storage** for repository working trees, runtime sessions, configuration identity, events, and pending interaction metadata.
- **Authenticated WebSocket** as the proposed platform-to-`agentctl` transport, with stable identities, acknowledgements, replay, and reconnect reconciliation implemented at the application level.

The Go ACP implementation remains to be selected and validated against the pinned OpenCode release. Validate approvals, session restoration, and interrupted-work continuation before accepting the runtime integration.

Core platform shutdown must disconnect from agents without terminating their work. Agent environments retain independent lifecycles. Container/process restart recovery assumes durable task storage remains intact; complete executor-host or disk loss recovery is deferred by the MVP requirements.

## Tools and integrations

- **HTTP MCP connections** for external agent capabilities; stdio MCP servers are outside the MVP.
- **Official MCP Go SDK** where the platform implements MCP clients or servers. Whether built-in platform tools use an internal HTTP MCP endpoint remains open.
- **GitHub Issues** as the first task-provider integration, accessed through a provider-independent integration contract.
- Separate interfaces for **task providers, executors, and agent runtimes** so provider-specific behavior stays outside core orchestration.

Agents create and update pull requests through their permitted runtime capabilities and configured integrations. Imported issue content remains source-managed, while Orkest owns local workflow and execution state.

## Architecture and local development

- **Modular monolith** for the core platform, with clear boundaries for identity/access, workspaces, tasks, orchestration, integrations, and execution.
- **Independent `agentctl` process** inside each execution environment.
- **Docker Compose** for local development services, including PostgreSQL, Keycloak, and the platform.

Avoid introducing microservices or a distributed workflow engine for the initial implementation. Durable execution state, ownership, event replay, and reconciliation are explicit application responsibilities.

## Initial validation milestone

Before building the full UI, implement a small Go execution prototype that:

1. Launches OpenCode in a Docker task environment and exchanges ACP messages.
2. Completes a user approval round trip.
3. Keeps the agent running across a core platform restart and replays missed events.
4. Restores files and the supported agent session after a container/process restart.
5. Reconciles execution and command identities without duplicate work.

This validates the highest-risk integration and recovery requirements before committing to the complete implementation.

## References

- [Project requirements](requirements.md)
- [Chi](https://github.com/go-chi/chi)
- [pgx](https://github.com/jackc/pgx)
- [sqlc](https://sqlc.dev/)
- [Docker Engine SDK](https://docs.docker.com/reference/api/engine/sdk/)
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- [ACP protocol and libraries](https://github.com/agentclientprotocol/agent-client-protocol)
