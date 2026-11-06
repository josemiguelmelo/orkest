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
| Go architecture and delivery separation | Implemented | Parsed dependency boundaries, independent wire/domain models, legacy continuation fixtures, PostgreSQL claim/fencing/concurrency tests; dated results and live limits recorded below |
| OpenCode | Pinned profile and shared host | Limited live current/v1-session memory continuation and Docker storage checks were reported; approvals/MCP/interrupted tool effects still need qualification |
| Claude Code, Codex, Copilot, Kiro | Pinned profiles and shared host | Deterministic conformance; live calls, approvals, MCP, and continuation need qualification |
| Docker and Local executors | Implemented | Fake-client/unit coverage; Docker suite exercises real containers |
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

## Verification record

The refactor session reported these results on **2026-11-03/04**, in an uncommitted
working tree based on `115fc6311dc31270c721f8be5fcc7e2ea8f0dae8`. These are historical
results for the tested tree, not a new test run during this documentation review.
Later code changes require the relevant checks again.

| Check | Reported evidence |
| --- | --- |
| `make check` | Go lint/tests, architecture checks, frontend generated API/lint/tests/types and both binary/frontend builds passed |
| Architecture | Parsed import and concrete runtime checks passed with zero temporary exceptions |
| Real PostgreSQL | Repository/migration and claim/fencing/concurrency tests passed with race detection; an isolated test database also passed three consecutive runs |
| Deterministic runtime/recovery | All five profiles, worker and runtime/HTTP/client adapter suites passed with race detection |
| `make test-recovery` | OpenCode v2.1.4 handshake and live restore plus a successful next model response passed for current manifests and the historical v1 manifest fixture |
| Docker | Pinned OpenCode image build and launch, daemon restart, container replacement, session identity, fresh-session and untracked-file preservation checks passed |
| Compatibility | Both OpenAPI specifications and the runtime-version inventory were byte-identical to the pre-refactor baseline; command/configuration/status/approval, checkpoint, manifest and fingerprint fixtures passed |

Live memory tests used explicit `opencode/beam-free` with no granted credentials.
Docker storage tests selected that model but sent no model prompts. Catalogue
availability can change; those results are not a current model recommendation.
The temporary PostgreSQL test database was removed after verification.

Remaining qualification limits:

- Claude Code, Codex, Copilot and Kiro live model confirmation and continuation
  have not been qualified. Their profiles now require ACP confirmation and fail
  visibly, preserving state, if a real provider cannot confirm the configured model.
- Real provider approvals/MCP, full deployed-platform restart during active work
  or an approval, and deliberately interrupted remote tool effects remain unverified.
  Deterministic binding/replay/uncertainty tests do not establish those live journeys.
- Newly written worker checkpoints omit structured Git credentials. A required
  missing credential set prevents restore until matching configuration replenishes
  it. This worker guard does not establish automatic platform credential reinjection
  after reconnect; that path needs integration verification.
- Team-member capacity, worker transport authentication and host/disk-loss guarantees
  remain subject to the implementation/scope limits above.

Migration 0038 requires stopping old platform workflow workers before upgrading;
mixed delivery versions are unsupported. See [delivery ownership](development/workflow-delivery.md)
and [migration operations](operations/migrations.md#workflow-delivery-claims).
