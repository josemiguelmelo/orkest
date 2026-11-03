# Deployment configuration

- Defaults: [.env.example](../../.env.example).
- Both binaries load optional `.env` from the current directory.
- Precedence: worker CLI flags → shell environment → `.env` → defaults.

## Connections and serving

| Settings | Meaning |
| --- | --- |
| `DATABASE_URL` | Full PostgreSQL URL; overrides component settings |
| `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USERNAME`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | All five are required without `DATABASE_URL` |
| `HTTP_ADDR`, `ORKEST_WEB_DIR` | Platform address and built frontend directory |
| `OIDC_ISSUER_URL` | Browser-visible Keycloak issuer |
| `OIDC_INTERNAL_ISSUER_URL` | Optional backend issuer address when browser/backend networking differs |
| `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URL`, `OIDC_COOKIE_SECURE` | Client, callback, and cookie configuration |
| `KEYCLOAK_PORT` | Compose's Keycloak host port (8082 by default) |

The frontend callback must match the browser origin; see [Vite setup](../development/webapp.md#development).

## Execution and credentials

| Settings | Meaning / details |
| --- | --- |
| `ORKEST_EXECUTOR`, `ORKEST_AGENTCTL_BINARY` | Docker (default) or [Local](../reference/executors/local.md#setup); Local requires an absolute worker binary path and provides no task isolation |
| `ORKEST_DATA_DIR` | [Durable files and session state](storage-and-secrets.md#retain-together) |
| `ORKEST_AGENT_PROVIDER_ENV` | Comma-separated host variable names granted to runtimes |
| `ORKEST_INTEGRATION_ENCRYPTION_KEY` | Required for encrypted secrets; [key format and retention](storage-and-secrets.md#protect-credentials) |
| `ORKEST_AGENT_MCP_URL`, `ORKEST_AGENT_MCP_TOKEN_KEY` | Reachable built-in tools endpoint and optional signing-key override; [key storage](storage-and-secrets.md#protect-credentials) |
| `AGENTCTL_*_IMAGE` | [Per-runtime Docker images](../reference/executors/docker.md#build-and-configure-images) |
| `ORKEST_CODEX_AUTH_FILE` | Optional source auth file; [Local](../reference/executors/local.md#credentials) defaults to the host login; [Docker](../reference/executors/docker.md#durable-files-and-credentials) requires a daemon-visible path |
| `AGENTCTL_LISTEN`, `AGENTCTL_WORK_DIR`, `AGENTCTL_STATE_DIR` | [Standalone worker](../guides/agentctl.md#run) address, work directory, and state directory |
| `ORKEST_EXECUTION_IMAGE_REPOSITORY`, `ORKEST_IMAGE_BUILDER`, `ORKEST_BUILDCTL_BINARY`, `ORKEST_BUILDKIT_*` | [Image build configuration](image-builds.md#configuration); unset repository disables builds |

Workspace values, provider overrides, and continued-session resolution follow
[agent environment rules](../guides/agent-configuration.md#environment-and-session-changes).

## Docker networking and paths

For named contexts (such as Colima), set `.env` `DOCKER_HOST` to:

```sh
docker context inspect --format '{{(index .Endpoints "docker").Host}}'
```

- Go reads `DOCKER_HOST` independently of CLI context; export it for Docker tests.
- Compose `ORKEST_DATA_DIR` must be daemon-visible for worker host bind mounts.
