# Implementation status and limitations

The table describes implementation, test coverage and operational limitations.
[Testing](development/testing.md) lists prerequisites; [requirements](requirements.md)
defines intended scope.

| Area | Implementation | Verification boundary |
| --- | --- | --- |
| Identity, memberships, workspace isolation | Implemented | Unit, PostgreSQL, and browser suites; backend remains authoritative |
| Task CRUD, workflow rules, artifacts | Implemented | Deterministic and PostgreSQL suites; live multi-stage agents need credentials |
| Workflow-independent scheduled jobs | Implemented | Daily/weekly/crontab calendars, timezone-aware upcoming-run preview, isolated occurrences, fenced delivery, workspace quota, scoped UI/API and durable activity; live model/email/PR effects require separate qualification |
| Direct conversations and durable sessions | Implemented | Queue, replay, reservation, outbox and ownership coverage; credential-dependent worker restart requires fresh Git credentials before restore |
| Structured user questions | Shared ACP forms for OpenCode/Codex/Claude; private MCP question tool for new sessions of all five runtimes | Deterministic schema, journal, HTTP/MCP, UI, and restore/follow-up coverage; live question/timeout/container qualification remains unavailable without explicit binaries/models/credentials; [details](development/user-questions.md) |
| Go architecture and delivery separation | Implemented | Parsed dependency boundaries, independent wire/domain models, legacy continuation fixtures, PostgreSQL claim/fencing/concurrency tests; live runtime behavior requires separate qualification |
| OpenCode | Pinned profile and shared host | Live continuation, approvals, MCP and interrupted tool effects require scenario-specific qualification |
| Claude Code, Codex, Copilot, Kiro | Pinned profiles and shared host | Deterministic conformance; live calls, approvals, MCP, and continuation need qualification |
| Docker and Local executors | Implemented | Fake-client/unit coverage; Docker suite exercises real containers |
| Workspace Docker host volumes | Direct admin UI settings without environment configuration; session-pinned bind mounts under `/mnt/host` or private-home `~/`; trusted deployments only | Unit/API/UI/PostgreSQL and real Engine file/directory access, missing-source failure, and container replacement; synthetic AWS profile/Gradle property discovery and deterministic restored-runtime home lookup; not credentialed ACP/model continuation |
| GitHub/GitLab coding integrations | Implemented | Provider and credential tests; live private-repository access needs credentials |
| GitHub Projects v2 / Jira Server imports | Implemented | Provider and synchronization tests; real source access needs credentials |
| Execution profile builds | BuildKit and Docker backends | Real composition suite needs daemon, registry, and foundation images |
| Team-member slot allocation | Unfinished | Task-level run reservation does not establish member capacity enforcement |
| Platform-to-agentctl authentication | Unfinished | Worker API must stay within a trusted boundary |
| Browser SSE, binary artifacts | Proposed or deferred | No implementation claim |
| Adapter-based workflow stage integrations | Implemented for GitHub PR review-added and merged events | Deterministic provider, API, workflow, and UI coverage; PostgreSQL and live GitHub behavior require separate qualification |
| Complete host/disk loss recovery | Outside MVP guarantee | Durable storage must survive |

- Loading alone proves no safe interrupted continuation. Replay must establish
  command identity/no uncertain effects; missing evidence pauses recovery.

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

## Stdio MCP support

The workspace catalog, APIs/UI, encrypted environment storage, worker contracts,
ACP mapping and OpenCode native provisioning support stdio alongside HTTP.
Continuation pins command/arguments/environment names, refreshes credential values,
and requires an explicit `http` or `stdio` transport. No empty-transport
fallback or fingerprint normalization remains; saved MCP configurations without
a transport fail visibly and preserve their saved state.

Live model-driven tool calls, approvals, interrupted effects, subprocess cleanup
and continuation require `make qualify-runtime` with provisioned executables and
explicit models/credentials for each profile.

The additive `0041` migration retains HTTP rows. Upgrade platform and workers before
using stdio; older workers cannot execute it. Rollback refuses while stdio catalog
rows exist. Do not downgrade workers owning persisted stdio sessions.

MCP transport uses one shared Go enum (`internal/contracts/mcptransport`) in catalog,
server/worker models, wire DTOs, validation and adapters. Persistence converts only
at the generated SQL boundary. The historical alternative credential-fingerprint
fallback has been removed: sessions requiring it fail visibly without replacing or
rewriting their saved state; exact supported manifest fingerprints still restore.
