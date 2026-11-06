# Server and agentctl simplification

Status: implementation and available verification completed, 2026-11-04.
Changes remain uncommitted for local review; live verification limits are below.
The pre-existing deletion of `architecture-improvements-plan.md` is preserved.

## Scope

- Phase 7: cohesive files and leaf domain model packages; preserve private state,
  consumer-owned contracts, and independent server/worker/wire models.
- Phase 8: shared checkout preparation and validation primitives, string and slice
  helpers, HTTP helpers, and consistent event cursor limits.
- Phase 9: simpler workflow evaluation, session preparation, runtime startup, and
  queue control flow without changing ownership or recovery ordering.
- Phase 10: architecture documentation, focused and full verification, diff review.

Related types share files. No one-declaration-per-file rule or artificial types.
Model packages cannot import their parent or concrete adapters. No database schema,
wire field, journal, manifest, runtime-pin, or durable-path changes are introduced.

## Progress

- Read requirements, stack, architecture boundaries, and database-change skill.
- Confirmed plan-document deletion existed before implementation.
- Migrated public server models, worker configuration, and worker status into leaf
  model packages, including adapter and test callers. Kept related values grouped,
  private state with its owner, and transaction policies in their domain package.
- Grouped wire DTOs by concern; separated HTTP requests/views and package-wide
  response/decoding helpers. Split task/direct-chat session preparation and worker
  recovery into focused behavior files.
- Consolidated Local/Docker checkout preparation and credential contract, scalar
  configuration checks, string normalization, slice mapping, and run views.
- Simplified workflow evaluation, direct-chat run lookup/reservation, runtime-host
  diagnostics/secret registration, controller startup, and post-prompt scheduling.
- Added architecture enforcement for model leaves and focused regression coverage.

Intentional behavior corrections: direct-chat event cursors now respect the same
signed-bigint range as task events. Direct continuation with only built-in tools
does not require an unrelated external MCP resolver; built-in tool refresh uses
the task dispatcher's existing deduplication rule. Saved external MCP URLs/settings
remain pinned, and refreshed headers are cloned before reservation.
The platform OpenAPI now documents the cursor maximum for both endpoints, with
regenerated client validators. Existing client parameter types remain unchanged.

Completed follow-up:

- [x] Phase 7: cohesive organization and leaf model packages.
- [x] Phase 8: shared mechanics and cursor correction.
- [x] Phase 9: orchestration and control-flow simplification.
- [x] Phase 10: documentation, available verification, and final diff review.

## Verification

Final results on 2026-11-04:

| Check | Result |
| --- | --- |
| `make check` | Passed: architecture checks, lint (zero issues), Go tests, generated API consistency, frontend lint/tests/types, and Go/frontend builds |
| `make test-runtime-conformance` | Passed deterministic profiles/runtime/worker/transport/orchestration suites with race detection, including current/v1 manifest restore and fixture follow-up |
| Additional `go test -race` | Passed executor, platform HTTP, agent, task, workflow, automation, configuration validation, and shared utility packages |
| Focused regressions | Passed checkout identity/path safety/error wrapping, credential validation, cursor boundaries, and direct continuation preserving pinned settings |
| Docker/integration-tagged compilation | Passed with both tags enabled for commands, internal packages, and integration tests |
| `make test-postgres` | Passed using local settings in an isolated test database |
| Full PostgreSQL package with `-race -count=1` | Passed against the same isolated database; database removed after verification |
| `make docker-agentctl-opencode` and Docker launch smoke | Rebuilt the base and pinned OpenCode image; real worker launch/health check passed, without model prompts |
| Real `TestOpenCodeHandshake` | Pinned OpenCode v2.1.4 created and reloaded the same session; no model prompt sent |
| `npm --prefix web run generate:api` | Regenerated cursor validators from the platform OpenAPI; final generated consistency check passed |
| `git diff --check` | Passed |

The new leaf check initially identified membership models referencing the identity
service package; moving User to identity/model resolved that dependency without
weakening the rule. Final review also migrated Docker-tagged test callers and
removed unrelated formatting-only changes.

No SQL, migrations, generated sqlc files, dependencies/lockfiles, worker OpenAPI,
runtime inventory, compatibility fixtures, or durable storage formats changed.
Docker testing used the rebuilt code before final formatting-only constructor
expansion; final source builds and checks passed afterward. No images were published.

## Verification limits

- Live restore followed by a model response was not run: no explicit
  `ORKEST_TEST_MODEL_PROVIDER`/`ORKEST_TEST_MODEL_ID` was configured. The installed
  pinned executable was available, but no paid/default model was selected.
- Full Docker session restart/replacement checks were not run: their explicit
  Docker model settings were absent. Docker launch smoke passed separately.
- Other providers' live calls, approvals/MCP, deployed-platform recovery, and
  uncertain external effects remain unqualified. Deterministic restore/follow-up
  and real session reload do not establish those live journeys.

## Review follow-up — 2026-11-06

- Reviewed the structural changes with particular attention to validation,
  checkout preparation, transport mappings, and session continuation.
- Simplified built-in MCP replacement while retaining the first saved position,
  removing duplicates, and preserving external server settings. Added coverage
  for credential scope, ordering, saved-state preservation, and resolver failure.
- Reused shared cancellation/permission request DTOs in direct chat and removed
  its redundant conversation lookup before `DirectChatService.Runs`, which
  already checks the scoped conversation. Workspace authorization stays in HTTP.
- Verification: `make check`, `make test-runtime-conformance`, focused HTTP and
  orchestration tests with `-race -count=1`, and `git diff --check` passed.
- PostgreSQL, Docker, and live model integration checks were not repeated in this
  review; their earlier evidence and qualification limits above still apply.
