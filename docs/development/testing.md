# Testing and qualification

Run commands from the repository root after [setup](../../README.md). This page
lists available checks and prerequisites. [Status](../status.md#verification-record)
records dated results; skipped, unavailable or failed checks are not passing results.

## Commands and prerequisites

| Command | Covers | Prerequisites |
| --- | --- | --- |
| `make check` | Go lint/tests, boundaries, frontend generated API/lint/tests/types, both binaries and frontend builds | Go, Node/npm, Python 3 and installed project dependencies |
| `make lint` / `make fmt` | Pinned Go lint and formatting | Go tooling; first install may need network access |
| `make check-runtime-boundaries` | Parsed layer imports, leaf model dependencies, generic exported names, concrete runtime restrictions and layer-rule tests | Go, Python 3 |
| `make test-postgres` | Real repositories, reservations, outboxes, delivery claims and migrations | Exported connection settings for an isolated PostgreSQL database |
| `make test-runtime-conformance` | Deterministic provider/recovery contracts and runtime-rule tests, with race detection | Go, Python 3 |
| `make test-recovery-unit` | Deterministic restore, queue, session and delivery contracts, with race detection | Go |
| `make test-integration` | Real OpenCode handshake/create/load; model continuation when explicitly configured | Pinned executable through `ORKEST_OPENCODE_BINARY` |
| `make test-recovery` | Recovery unit suite plus real OpenCode restore and successful follow-up | Pinned executable, explicit provider/model, credentials if required |
| `make test-docker` | Real launch, daemon/container restart, fresh sessions and retained files | Docker Engine; builds runtime images; explicit Docker test model settings |
| `make test-image-composition` | BuildKit/Docker composition, publishing, digest recovery | Docker, registry, foundation images and pinned buildctl/BuildKit for BuildKit cases |
| `npm --prefix web run test:e2e` | Desktop/mobile journeys against API mocks | Installed Playwright Chromium |

Go targets use finite timeouts. `make check` does not enable integration/Docker
build tags or establish live provider qualification. Without database settings,
ordinary Go tests may skip PostgreSQL cases; `make test-postgres` rejects missing
connection settings instead of silently skipping them.

## PostgreSQL and Docker environment

Both application binaries load optional `.env`; the test targets do not. Export
`DATABASE_URL`, or all five `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USERNAME`,
`POSTGRES_PASSWORD`, and `POSTGRES_DB` values, before running repository tests.
Point them at a dedicated test database: suites apply migrations, alter data and
drain shared pending events/commands. A running platform must not consume that
same test database. `make db-up` starts Compose PostgreSQL; it does not export
settings or create a separate test database.

For changed delivery/concurrency paths, run the real suite with race detection:

```sh
go test -race ./internal/adapters/postgres \
  -run 'TestPostgresRepositories|TestSessionMigrations|TestExecutionProfileMigration' \
  -count=1 -timeout=3m
```

Docker clients in Go read `DOCKER_HOST` independently of the CLI's selected
context. Export the [daemon socket](../operations/configuration.md#docker-networking-and-paths)
for Docker tests. Also export `ORKEST_DOCKER_TEST_MODEL_PROVIDER` and
`ORKEST_DOCKER_TEST_MODEL_ID` to an available explicit model before
`make test-docker`. The storage test confirms selection but sends no model prompts;
a stale hardcoded model cannot establish a successful restore.

For Docker-only image composition without buildctl:

```sh
go test -tags=docker ./internal/adapters/imagebuild/docker \
  -run '^TestDockerImageComposition/docker$' -count=1 -timeout=10m -v
```

## Real continuation

1. Set `ORKEST_OPENCODE_BINARY`, `ORKEST_TEST_MODEL_PROVIDER` and `ORKEST_TEST_MODEL_ID`.
2. If the provider requires credentials, export their values and grant only their
   variable names through comma-separated `ORKEST_TEST_PROVIDER_ENV`. Leave it
   empty for a model that needs no credentials.
3. Run `make test-recovery`.

The target checks its explicit model prerequisites and runs the deterministic
recovery suite, handshake and `TestOpenCodeContinuation`. Continuation covers both
the current native manifest and a historical v1 manifest envelope paired with
real saved runtime history. It must receive a successful next model response and
preserve the untracked test file; loading alone is insufficient.

`make test-integration` runs the same integration package, but continuation skips
when explicit model settings are absent. Its handshake currently selects
`opencode/big-pickle` without sending a model prompt. Catalogue availability still
matters. Do not report handshake success or a skipped continuation as inference
qualification. Live continuation may incur provider costs; no paid/default model
is selected implicitly.

The real `TestOpenCodeStdioMCPHandshake` integration test uses the test executable
as a small MCP server. With `ORKEST_OPENCODE_BINARY` set to the pinned executable,
it checks stdio initialization, literal arguments/environment delivery, restored
session identity after credential rotation, and graceful subprocess shutdown.
It makes no model request and does not qualify model-driven tool calls.

## Runtime qualification

Select one profile with `make qualify-runtime QUALIFY_RUNTIME=<runtime>`.
`make qualify-runtime` without a filter selects all profiles. The target sets
`ORKEST_RUNTIME_CONFORMANCE=live`; deterministic conformance normally uses fixtures.

| Runtime | Executable setting | CI model setting mapped to `ORKEST_TEST_MODEL_ID` |
| --- | --- | --- |
| OpenCode | `ORKEST_OPENCODE_BINARY` | `ORKEST_TEST_MODEL_ID` |
| Claude Code | `ORKEST_CLAUDE_CODE_BINARY` | `ORKEST_CLAUDE_CODE_TEST_MODEL_ID` |
| Codex | `ORKEST_CODEX_BINARY` | `ORKEST_CODEX_TEST_MODEL_ID` |
| Copilot | `ORKEST_GITHUB_COPILOT_BINARY` | `ORKEST_GITHUB_COPILOT_TEST_MODEL_ID` |
| Kiro | `ORKEST_KIRO_BINARY` | `ORKEST_KIRO_TEST_MODEL_ID` |

For local qualification, set the shared `ORKEST_TEST_MODEL_PROVIDER` and
`ORKEST_TEST_MODEL_ID`, plus the provider environment allow-list. The strict suite
also requires `ORKEST_TEST_APPROVAL_PROMPT`, `ORKEST_TEST_RECOVERY_PROMPT`,
`ORKEST_TEST_CANCELLATION_PROMPT`, `ORKEST_TEST_QUESTION_PROMPT`,
`ORKEST_TEST_QUESTION_EXPECTED`, `ORKEST_TEST_MCP_URL`, `ORKEST_TEST_MCP_PROMPT`
and `ORKEST_TEST_MCP_EXPECTED`. Stdio qualification also requires
`ORKEST_TEST_STDIO_MCP_COMMAND` (absolute executable), `ORKEST_TEST_STDIO_MCP_ARGS`
(JSON string array, including `[]`), `ORKEST_TEST_STDIO_MCP_ENV` (JSON string map,
including `{}`), `ORKEST_TEST_STDIO_MCP_PROMPT` and `ORKEST_TEST_STDIO_MCP_EXPECTED`.
Provision that executable and pinned dependencies in the qualification environment.
MCP probes run before and after restore. Deterministic stdio probes verify configuration
propagation, not a real server connection. Missing prerequisites fail rather than substitute
fixture evidence. Exact requirements are in the [conformance tests](../../internal/agentctl/runtime/conformance_test.go).

Verify the real response/model, instructions/skills, MCP headers, bound approval
round trip, cancellation, restore and follow-up. Interrupted replay needs command
identity and side-effect evidence. Ordinary memory continuation qualifies a
narrower scenario than this suite.

The question prompt must ask one structured string question accepting `pnpm`; the
suite supplies that answer and checks the configured acknowledgement substring
before and after restore. See [question verification](user-questions.md#verification).

CI uses the `runtime-qualification` environment and
`ORKEST_RUN_LIVE_QUALIFICATION=true`. See [credential/endpoint mappings](../../.github/workflows/runtime-conformance.yml).
The version-inventory tool compares the current path with the pre-migration path
when reading older Git revisions; moving the inventory does not change a runtime pin.

## Frontend and generated artifacts

| Command | When to use |
| --- | --- |
| `npm --prefix web run generate:api` | After platform OpenAPI changes; regenerate SDK JavaScript/declarations and validators, then run `make check` |
| `npm --prefix web run check:generated` | Generate separately and compare without changing committed output |
| `make sqlc` | After SQL/config changes; regenerate `internal/adapters/postgres/sqlc` |
| `npm --prefix web run format` / `format:check` | Apply/check frontend formatting |
| `web/node_modules/.bin/playwright install chromium` | Install the pinned browser before browser journeys |
| `node web/scripts/smoke-live.mjs` | Opt-in real Keycloak/API/SPA smoke journey |

Never hand-edit generated output. The authoritative API inputs are the
[platform OpenAPI](../../internal/adapters/http/server/openapi.yaml) and
[worker OpenAPI](../../internal/adapters/http/agentctl/openapi.yaml); the frontend
client is generated from the platform contract. Go checks name project trees to
exclude npm-shipped Go files.

Live smoke creates, exercises and removes a test organization in a disposable
local database. It never starts agents or clones remotes. Settings are
`ORKEST_LIVE_BASE_URL` and optional `ORKEST_LIVE_LOGIN_URL`, `ORKEST_LIVE_USERNAME`,
`ORKEST_LIVE_PASSWORD`; defaults are in the [local setup](../../README.md#run-locally).
Screenshots use the OS temporary directory. See [webapp development](webapp.md).

## Architecture checks

Parsed `go list` imports enforce server/worker, adapter, contract, platform and
command boundaries. The checker rejects superseded packages, stale exceptions
and generic exported `Service`/`Repository` declarations in server domains.
Concrete runtime checks cover all five profiles, including Kiro. The migration
exception list is empty; explicit constructors and domain-owned ports express
current dependencies.

The PostgreSQL suite covers two-worker claims, fencing before run reservation,
stale acknowledgements, reservation/acceptance loss, cancellation, acknowledgement
failure, unavailable acceptance storage/agentctl, ordinary failure without retry,
atomic outbox rollback, stale events/runs, repeated imports, bounded scoped task
listing queries and migration rollback guards. External dispatch is asserted to
hold no database connection or transaction.

Runtime/recovery targets include concrete runtime, HTTP and client adapters as
well as worker domains and orchestration. Deterministic legacy-manifest tests
prove adapter/policy behavior; configured integration tests exercise actual next
model responses. Keep those evidence categories separate.

## Record evidence

Record the command, result, date, commit or working-tree base, environment and
missing prerequisites in task/PR evidence. Distinguish fixtures, real local
infrastructure and live providers. The [verification record](../status.md#verification-record)
contains historical refactor results and remaining limits. Model catalogue
availability and prior passing results are not unconditional guarantees.
