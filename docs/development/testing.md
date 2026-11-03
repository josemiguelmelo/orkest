# Testing and qualification

- Run from repository root after [setup](../../README.md).
- Available checks, not current passing results; report skipped/unavailable/failed checks.

| Command | Covers | Prerequisites |
| --- | --- | --- |
| `make check` | Go lint/tests, boundaries, frontend generated-code/lint/tests/types, builds | Installed project dependencies |
| `make lint` / `make fmt` | Pinned Go lint and formatting | Go tooling |
| `make check-runtime-boundaries` | Runtime import directions | Python 3 |
| `make test-postgres` | Real repositories, reservations, outbox, migrations | Dedicated PostgreSQL database |
| `make test-runtime-conformance` | Deterministic provider/recovery contracts with race detection | Go, Python |
| `make test-recovery-unit` | Restore, recovery, queue, and session behavior under race detection | Go |
| `make test-integration` | Real OpenCode handshake/loading; optional continuation | Pinned executable via `ORKEST_OPENCODE_BINARY` |
| `make test-recovery` | Deterministic suite plus real OpenCode continuation | Executable, explicit model, credentials |
| `make test-docker` | Daemon/container restart, fresh sessions, retained files | Docker; builds runtime images |
| `make test-image-composition` | BuildKit and Docker composition, publishing, digest recovery | Docker, registry, foundation images, pinned buildctl/BuildKit |
| `npm --prefix web run test:e2e` | Desktop/mobile journeys against API mocks | Installed Playwright Chromium |

- Use an isolated PostgreSQL test database: suites alter data.
- Database/socket settings: [configuration](../operations/configuration.md); export `DOCKER_HOST` for non-default sockets.

For Docker-only image composition without buildctl:

```sh
go test -tags=docker ./internal/imagebuild/docker -run '^TestDockerImageComposition/docker$' -count=1 -v
```

## Real continuation

1. Set `ORKEST_OPENCODE_BINARY`, `ORKEST_TEST_MODEL_PROVIDER`, `ORKEST_TEST_MODEL_ID`.
2. Grant credential names through comma-separated `ORKEST_TEST_PROVIDER_ENV`.
3. Run `make test-recovery`. No implicit paid model; handshake alone does not qualify continuation.

## Runtime qualification

- One profile: `make qualify-runtime QUALIFY_RUNTIME=<runtime>`; all: `make qualify-runtime`.
- Local settings: pinned `ORKEST_<RUNTIME>_BINARY`, explicit `ORKEST_TEST_MODEL_PROVIDER` /
  `ORKEST_TEST_MODEL_ID`, granted `ORKEST_TEST_PROVIDER_ENV`.
- Suite inputs: `ORKEST_TEST_APPROVAL_PROMPT`, `ORKEST_TEST_RECOVERY_PROMPT`,
  `ORKEST_TEST_CANCELLATION_PROMPT`, `ORKEST_TEST_MCP_URL`, `ORKEST_TEST_MCP_PROMPT`,
  `ORKEST_TEST_MCP_EXPECTED`. Exact contract: [tests](../../internal/agentctl/runtime/).
  Missing prerequisites must fail, never substitute fixtures.
- CI: `runtime-qualification` environment, `ORKEST_RUN_LIVE_QUALIFICATION=true`.
  Credential/endpoint mapping: [workflow](../../.github/workflows/runtime-conformance.yml).

| CI runtime | Model variable |
| --- | --- |
| OpenCode | `ORKEST_TEST_MODEL_ID` |
| Claude Code | `ORKEST_CLAUDE_CODE_TEST_MODEL_ID` |
| Codex | `ORKEST_CODEX_TEST_MODEL_ID` |
| Copilot | `ORKEST_GITHUB_COPILOT_TEST_MODEL_ID` |
| Kiro | `ORKEST_KIRO_TEST_MODEL_ID` |

Verify real response/model, instructions/skills, MCP headers, approval round trip,
cancellation, restore plus follow-up. Interrupted replay needs command/side-effect
evidence; loading alone proves no exact recovery. See [status](../status.md).

## Frontend and generated artifacts

| Command | When to use |
| --- | --- |
| `npm --prefix web run generate:api` | After platform OpenAPI changes; regenerate SDK JavaScript/declarations and validators, then run `make check` |
| `npm --prefix web run check:generated` | Generate separately and compare without changing committed output |
| `make sqlc` | After SQL query changes; regenerate persistence code |
| `npm --prefix web run format` / `format:check` | Apply/check frontend formatting |
| `web/node_modules/.bin/playwright install chromium` | Install the pinned browser before browser journeys |
| `node web/scripts/smoke-live.mjs` | Opt-in real Keycloak/API/SPA smoke journey |

- Never hand-edit generated output. Go checks name project trees to exclude npm-shipped Go files.

Live smoke:

- Disposable local database only: creates/exercises/removes a test organization;
  never starts agents/clones remotes. Screenshots: OS temporary directory.
- `ORKEST_LIVE_BASE_URL`; optional `ORKEST_LIVE_LOGIN_URL`, `ORKEST_LIVE_USERNAME`,
  `ORKEST_LIVE_PASSWORD`. Defaults: [local setup](../../README.md#run-locally).
- Architecture/setup: [webapp](webapp.md).

## Record evidence

- Task/PR evidence: command, result, date, commit, environment, missing prerequisites.
- Report mocks, deterministic runtime tests, real infrastructure, and live qualification separately.
- Credentialed suites may incur model costs.
