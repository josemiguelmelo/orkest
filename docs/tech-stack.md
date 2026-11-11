# Technology stack

The platform and agentctl share one Go module. The UI is a React application.
[Architecture](architecture.md) explains ownership; [status](status.md) records limits.

| Area | Technology | Purpose |
| --- | --- | --- |
| Backend | Go, `net/http`, Chi | Platform and agentctl servers |
| Composition | Explicit constructors | Select concrete dependencies at startup |
| Logging | `log/slog` | Structured, redacted diagnostics |
| Persistence | PostgreSQL, pgx | Durable product and execution state |
| Schema and queries | Goose, sqlc | Versioned migrations and generated query code |
| Identity | Keycloak, OIDC | Authentication; Orkest owns authorization |
| Frontend | React, TypeScript, Vite | Management workbench |
| UI state and forms | TanStack Router/Query, React Hook Form, Zod | Navigation, scoped caches, validation |
| UI primitives | Tailwind, Radix | Owned components and accessible interactions |
| API contracts | OpenAPI, Hey API | Served contracts and generated frontend client |
| Coding provider APIs | go-github, GitLab client-go | Typed REST authentication and repository discovery behind provider adapters |
| Execution | Docker Engine; Local processes; Kubernetes Pods | Independent agentctl workers |
| Agent protocol | ACP over stdio | Runtime commands and callbacks |
| Worker transport | REST, receive-only WebSocket | Commands, status, ordered activity |
| Tools | Streamable HTTP and stdio MCP | Built-in HTTP tools; external HTTP or stdio tools |
| Image builds | BuildKit or Docker CLI | Compose and publish execution profile images |

- Excluded: Redis and message broker. Kubernetes execution is an initial in-cluster
  adapter with shared-PVC requirements and documented qualification gaps.
- Browser activity polls; SSE is proposed. Worker transport authentication is unfinished.

## Authoritative inventories

| Inventory | Source |
| --- | --- |
| Go toolchain and libraries | [go.mod](../go.mod), [go.sum](../go.sum) |
| Frontend and Node constraints | [package.json](../web/package.json), [lockfile](../web/package-lock.json) |
| Runtime versions | [versions.json](../internal/contracts/runtimecatalog/versions.json) |
| Local infrastructure | [Compose](../compose.yaml) |
| Configuration | [.env.example](../.env.example) |
| Build and verification commands | [Makefile](../Makefile) |
| Platform API | [OpenAPI](../internal/adapters/http/server/openapi.yaml) |
| Worker API | [OpenAPI](../internal/adapters/http/agentctl/openapi.yaml) |

- Pin dependencies; regenerate SQL/API artifacts from inputs.
- Coding provider SDKs stay inside adapters; domain contracts retain Orkest types
  and sanitized errors. GitLab SDK retries are disabled to preserve request
  behavior. Git checkouts use the existing credential-safe Git CLI path; GitHub
  Projects imports retain their separate GraphQL client.
- Keep generated rows/SDK types in adapters; static queries in `db/queries`.
- PostgreSQL adapters own locks, transactions, consistency enforcement and row→domain mapping.
  Domain planners supply business effects. Workflow dispatch uses short claim and
  acknowledgement transactions with external delivery in the use case.
- Composition lives in `cmd/server` and `cmd/agentctl`; `bin/orkest` remains the
  platform binary. [Architecture](architecture.md) defines server/worker, adapter,
  transport-contract and platform utility boundaries.

See [testing](development/testing.md), [runtime setup](reference/runtimes/README.md),
and [webapp development](development/webapp.md) for practical details.
