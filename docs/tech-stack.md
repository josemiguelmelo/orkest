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
| Execution | Docker Engine; Local processes | Independent agentctl workers |
| Agent protocol | ACP over stdio | Runtime commands and callbacks |
| Worker transport | REST, receive-only WebSocket | Commands, status, ordered activity |
| Tools | Streamable HTTP MCP | Built-in and external tools |
| Image builds | BuildKit or Docker CLI | Compose and publish execution profile images |

- Excluded: Redis, message broker, Kubernetes execution.
- Browser activity polls; SSE is proposed. Worker transport authentication is unfinished.

## Authoritative inventories

| Inventory | Source |
| --- | --- |
| Go toolchain and libraries | [go.mod](../go.mod), [go.sum](../go.sum) |
| Frontend and Node constraints | [package.json](../web/package.json), [lockfile](../web/package-lock.json) |
| Runtime versions | [versions.json](../internal/core/runtime/versions.json) |
| Local infrastructure | [Compose](../compose.yaml) |
| Configuration | [.env.example](../.env.example) |
| Build and verification commands | [Makefile](../Makefile) |
| Platform API | [OpenAPI](../internal/app/httpapi/openapi.yaml) |
| Worker API | [OpenAPI](../internal/agentctl/httpapi/openapi.yaml) |

- Pin dependencies; regenerate SQL/API artifacts from inputs.
- Keep generated rows/SDK types in adapters; static queries in `db/queries`.
- PostgreSQL adapters own transactions and row→domain mapping.

See [testing](development/testing.md), [runtime setup](reference/runtimes/README.md),
and [webapp development](development/webapp.md) for practical details.
