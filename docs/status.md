# Implementation and verification status

The table describes implementation and suite coverage. Dated passing results are
recorded [below](#verification-record); they do not qualify later source edits.
[Testing](development/testing.md) lists prerequisites; [requirements](requirements.md)
defines intended scope.

| Area | Implementation | Verification boundary |
| --- | --- | --- |
| Identity, memberships, workspace isolation | Implemented | Unit, PostgreSQL, and browser suites; backend remains authoritative |
| Task CRUD, workflow rules, artifacts | Implemented | Deterministic and PostgreSQL suites; live multi-stage agents need credentials |
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
| Task trigger integrations | GitHub signed review/merge webhooks, automatic branch/commit PR association, reconciliation, explicit links, stage movement or same-stage assigned-agent starts with custom prompts, and configuration UI | Unit/UI and PostgreSQL transaction coverage; live GitHub delivery and credentialed agent execution not qualified |
| Browser SSE, binary artifacts | Proposed or deferred | No implementation claim |
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

### 2026-11-08: webhook task trigger integrations

- `make check` passed, including lint, Go tests, generated API consistency,
  frontend lint/type checks and production builds.
- PostgreSQL migration/repository suite passed with local database settings.
  Trigger coverage verifies semantic deduplication, acknowledgement rollback and
  concurrent worker claims; the focused suite also passed with race detection.
- Trigger provider/policy, workflow startup context, HTTP and command wiring tests
  passed with race detection. Frontend suite passed: 62 tests, including trigger
  viewer restrictions and versioned edits that preserve write-only secrets.
- Live GitHub delivery and credentialed review-response/merge workflows were not
  run. Automated entry starts retain the existing new-session behavior; this does
  not establish credentialed session restoration or provider continuation.
- Migration and mixed-version precautions are documented under
  [trigger migration operations](operations/migrations.md#task-trigger-integrations-and-event-context-00400041).


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

### Automatic PR association

agentctl journals Git branch evidence from task checkouts/worktrees without
controlling names. The platform associates same-repository GitHub PRs and retains
unmatched deliveries while evidence arrives. Reconciliation uses coding credentials;
existing trigger/workflow services retain execution ownership. See
[behavior and limitations](guides/task-trigger-integrations.md#automatic-association-and-recovery).
Live GitHub delivery and credentialed runtime continuation remain unqualified.

### Trigger integration simplification

Manual PR association creation is removed. Webhooks persist verified events and
resolve task association before acknowledgement; delayed evidence remains
recoverable. One integration supports multiple repository IDs. This feature has
no deployed data: there is no manual-owner fallback or migration for older
configuration formats. PR resolution exclusively owns task binding. Ingestion
uses one transaction guarded by the expected configuration version. Observer
comparison is pure; the run coordinator explicitly journals and persists changes
before advancing state. Polling start/stop and the final scan are explicit.
Live GitHub delivery qualification remains unverified.
