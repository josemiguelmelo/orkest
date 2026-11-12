# Scheduled jobs

Scheduled jobs are workspace-scoped repeatable agent instructions, independent of
workflow tasks, stages and board transitions. Enabling explicitly authorizes future
invocations. Each occurrence has its own durable session, run, working files and
result activity; recovery retains that occurrence's original identity/configuration.

Implementation plan and progress:
- [x] Definitions, immutable revisions, timing policy, occurrence identity and claims.
- [x] Job-owned reservation, existing launch/reconnect, bounded completion and cleanup.
- [ ] Scoped HTTP/OpenAPI and workspace UI.
- [ ] Real PostgreSQL races, compatibility, authorization and UI verification.
- [ ] Full checks, diff review, commit/push and MR.

Defaults: daily/weekly IANA calendars; repeated local DST minutes run once and
nonexistent times are skipped; one-hour lateness/capacity grace; no same-job overlap;
one active scheduled execution per workspace. Approval waits and uncertain ownership
retain capacity. Pausing leaves queued/accepted invocations intact. Failed runs are
not automatically retried; the next calendar occurrence is a distinct invocation.

Repositories can be absent, explicitly selected or all registered workspace
repositories (including future additions). Each occurrence snapshots exact IDs and
revalidates current access. Agent-selected MCP tools provide external services;
email jobs require a connector authorized for the intended account. No native
mailbox OAuth integration is added. External effects are agent-reported, not verified
exactly-once outcomes; reconcile existing effects before an explicit retry.

Legacy worker taskId/session fields retain their names and existing paths. For jobs
these contain occurrence execution identities and do not reference board tasks.
Job ownership is stored in scheduled_job_runs. Completion recognizes this owner
without changing existing task/direct-chat prompt semantics or model/runtime pins.

Migration 0041 is additive. Stop old platform instances before activating jobs:
old completion code cannot interpret job ownership safely. Independent agentctl
processes continue. Reconcile pending/uncertain runs before rollback; the down
migration refuses active ownership and removes completed job history/session
snapshots, not durable runtime files.

Verification results and live qualification limits will be recorded before delivery.
