# Implementation and verification status

The table describes implementation and suite coverage. Dated passing results are
recorded [below](#verification-record); they do not qualify later source edits.
[Testing](development/testing.md) lists prerequisites; [requirements](requirements.md)
defines intended scope.

| Area | Implementation | Verification boundary |
| --- | --- | --- |
| Identity, memberships, workspace isolation | Implemented | Unit, PostgreSQL, and browser suites; backend remains authoritative |
| Task CRUD, workflow rules, artifacts | Implemented | Deterministic and PostgreSQL suites; live multi-stage agents need credentials |
| Workflow-independent scheduled jobs | Implemented | Daily/weekly calendars, isolated occurrences, fenced delivery, workspace quota, scoped UI/API and durable activity; live model/email/PR effects require separate qualification |
| Direct conversations and durable sessions | Implemented | Queue, replay, reservation, outbox and ownership coverage; credential-dependent worker restart requires fresh Git credentials before restore |
| Structured user questions | Shared ACP forms for OpenCode/Codex/Claude; private MCP fallback for new Copilot/Kiro sessions | Deterministic schema, journal, HTTP/MCP, UI, and restore/follow-up coverage; live question/timeout/container qualification remains unavailable without explicit binaries/models/credentials; [details](development/user-questions.md) |
| Go architecture and delivery separation | Implemented | Parsed dependency boundaries, independent wire/domain models, legacy continuation fixtures, PostgreSQL claim/fencing/concurrency tests; dated results and live limits recorded below |
| OpenCode | Pinned profile and shared host | Limited live current/v1-session memory continuation and Docker storage checks were reported; approvals/MCP/interrupted tool effects still need qualification |
| Claude Code, Codex, Copilot, Kiro | Pinned profiles and shared host | Deterministic conformance; live calls, approvals, MCP, and continuation need qualification |
| Docker and Local executors | Implemented | Fake-client/unit coverage; Docker suite exercises real containers |
| Workspace Docker host volumes | Direct admin UI settings without environment configuration; session-pinned bind mounts under `/mnt/host` or private-home `~/`; trusted deployments only | Unit/API/UI/PostgreSQL and real Engine file/directory access, missing-source failure, and container replacement; synthetic AWS profile/Gradle property discovery and deterministic restored-runtime home lookup; not credentialed ACP/model continuation |
| GitHub/GitLab coding integrations | Implemented | Provider and credential tests; live private-repository access needs credentials |
| GitHub Projects v2 / Jira Server imports | Implemented | Provider and synchronization tests; real source access needs credentials |
| Execution profile builds | BuildKit and Docker backends | Real composition suite needs daemon, registry, and foundation images |
| Team-member slot allocation | Unfinished | Task-level run reservation does not establish member capacity enforcement |
| Platform-to-agentctl authentication | Unfinished | Worker API must stay within a trusted boundary |
| Browser SSE, PR webhook rules, binary artifacts | Proposed or deferred | No implementation claim |
| Complete host/disk loss recovery | Outside MVP guarantee | Durable storage must survive |

- Loading alone proves no safe interrupted continuation. Replay must establish
  command identity/no uncertain effects; missing evidence pauses recovery.
- Historical Docker replacement issue: intermittent HTTP 502. Recheck with the real
  suite; record recurrence/environment/commit. This is not a current result.

## Product decisions requiring reconciliation

| Policy | Repository instruction / earlier proposal | Current specification/code / decision needed |
| --- | --- | --- |
| Starts | `AGENTS.md`: manual only | Configured entry starts/same-task advancement; reconcile |
| Permissions | `AGENTS.md`: default OpenCode permissions | Explicit `ask`/`autonomous`; reconcile |
| Reopening | `AGENTS.md`: users reopen | Closed tasks cannot reopen; destination/authorization open |
| Repository selection | Requirements §5: agent selects needed workspace repositories | §3.2/code: task-selected registrations; may agents expand selection? |
| Assignment | Requirements §5: team member | Code: agent definition, capacity unfinished; define assignment migration |

Product decisions required. Documentation neither changes repository instructions
nor authorizes new behavior.

Migration 0038 requires stopping old platform workflow workers before upgrading;
mixed delivery versions are unsupported. See [delivery ownership](development/workflow-delivery.md)
and [migration operations](operations/migrations.md#workflow-delivery-claims).

## Verification record

### 2026-11-07: structured user questions

- `make check` passed: dependency boundaries, Go lint/unit tests, generated API
  consistency, frontend lint, 58 Vitest tests, type checks, Go builds, and Vite build.
- Focused race suites passed for the run controller, ACP adapter, private MCP tool,
  runtime host, common runtime conformance, and worker/platform HTTP adapters.
  The runtime-host race suite and Go lint passed again after the final protected
  string/boolean-metadata guard adjustment.
- Common deterministic runtime fixtures exercise question/answer consumption before
  and after restore for all five profiles; Copilot/Kiro fixtures use actual HTTP MCP.
  Legacy and v4 fallback session fixtures restore and accept follow-ups.
- Live question qualification and Docker execution were not run. Explicit runtime
  binary/model/provider-grant/question-prompt settings were absent; Claude/Kiro
  executables were unavailable. Fixture results do not establish real CLI tool
  discovery, human-duration waits, or interrupted invocation continuation.
- Deploy matching platform/UI and worker builds; new fallback sessions use native
  manifest v4. [Compatibility and rollout](development/user-questions.md#persistence-compatibility-and-deployment).

## Stdio MCP support

The workspace catalog, APIs/UI, encrypted environment storage, worker contracts,
ACP mapping and OpenCode native provisioning support stdio alongside HTTP.
Continuation pins command/arguments/environment names, refreshes credential values,
and requires an explicit `http` or `stdio` transport. No empty-transport
fallback or fingerprint normalization remains; saved MCP configurations without
a transport fail visibly and preserve their saved state. Runtime fixture probes
cover configuration before and after restore for all five profiles. On 2026-11-11 the pinned local
OpenCode v2.1.4 passed a real stdio MCP handshake, session restore with credential
rotation, and graceful MCP subprocess shutdown after both sessions. This test
makes no model request and does not establish model-driven tool calls, approval
behavior, interrupted effects, container cleanup, or live behavior of other profiles.
Those need `make qualify-runtime` with provisioned executables and explicit
models/credentials. No runtime pin changed.

Verification: `make check` (lint, Go tests, generated client, frontend lint/tests,
types and builds), `make test-runtime-conformance` (race detection), real OpenCode
HTTP/stdio handshake tests, and PostgreSQL repositories/migrations in a temporary
schema passed. Migration coverage includes HTTP defaults, stdio persistence,
workspace isolation, mixed-field constraints and guarded rollback/reapply. The
local application's migration history is newer than this checkout, so fresh-schema
verification was used rather than changing that schema.

The additive `0041` migration retains HTTP rows. Upgrade platform and workers before
using stdio; older workers cannot execute it. Rollback refuses while stdio catalog
rows exist. Do not downgrade workers owning persisted stdio sessions.

MCP transport uses one shared Go enum (`internal/contracts/mcptransport`) in catalog,
server/worker models, wire DTOs, validation and adapters. Persistence converts only
at the generated SQL boundary. The historical alternative credential-fingerprint
fallback has been removed: sessions requiring it fail visibly without replacing or
rewriting their saved state; exact supported manifest fingerprints still restore.
