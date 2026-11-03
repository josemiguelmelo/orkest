# Implementation and verification status

Code/suite inventory, not current passing results. [Testing](development/testing.md)
lists commands/prerequisites; [requirements](requirements.md) defines intended scope.

| Area | Implementation | Verification boundary |
| --- | --- | --- |
| Identity, memberships, workspace isolation | Implemented | Unit, PostgreSQL, and browser suites; backend remains authoritative |
| Task CRUD, workflow rules, artifacts | Implemented | Deterministic and PostgreSQL suites; live multi-stage agents need credentials |
| Direct conversations and durable sessions | Implemented | Queue, replay, reservation, outbox, and ownership coverage |
| OpenCode | Pinned profile and shared host | Handshake/loading and Docker restore suites exist; live model behavior needs qualification |
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
