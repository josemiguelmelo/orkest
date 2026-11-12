# Scheduled jobs

Scheduled jobs are workspace-scoped repeatable agent instructions, independent of
workflow tasks, stages and board transitions. Enabling explicitly authorizes future
invocations. Each occurrence has its own durable session, run, working files and
result activity; recovery retains that occurrence's original identity/configuration.

Implementation plan and progress:
- [x] Definitions, immutable revisions, timing policy, occurrence identity and claims.
- [x] Job-owned reservation, existing launch/reconnect, bounded completion and cleanup.
- [x] Scoped HTTP/OpenAPI and workspace UI.
- [x] Real PostgreSQL races, compatibility, authorization and UI verification.
- [x] Full checks and diff review; commit/push and MR are the delivery steps.

Schedules support daily, weekly, and standard five-field crontab expressions
(minute, hour, day of month, month, weekday), including names, lists, ranges, and
steps. Weekday 0 and 7 both mean Sunday. Cron day-of-month and weekday use standard
cron matching semantics. All forms use an IANA time zone; repeated local DST minutes
run once and nonexistent times are skipped. The editor previews the next five
occurrences in that zone and in UTC.

Defaults: one-hour lateness/capacity grace; no same-job overlap; one active scheduled
execution per workspace. Approval waits and uncertain ownership retain capacity.
Pausing leaves queued/accepted invocations intact. Failed runs are not automatically
retried; the next calendar occurrence is a distinct invocation.

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

Migration 0047 is additive. The deployment database is already at version 46;
Goose skips migrations at or below the recorded database version, which is why
the earlier 0042 job migration left `scheduled_jobs` missing. The 0047 migration
runs on that database and on fresh installs. Stop old platform instances before
activating jobs: old completion code cannot interpret job ownership safely.
Independent agentctl processes continue. Reconcile pending/uncertain runs before
rollback; the down migration refuses active ownership and removes completed job
history/session snapshots, not durable runtime files.

Verification:
- The initial standalone-job implementation passed `make check`, PostgreSQL
  integration/race coverage, deterministic recovery fixtures, and scheduled-job
  race tests before this cron extension.
- Migration 0047 passed `make test-postgres` against a fresh isolated database.
- This cron/preview update passes `make lint`, `go build ./cmd/server`, generated
  client consistency, frontend lint, type checks, and production build. Cron
  recurrence behavior has not yet been exercised by a test run.
- Live pinned OpenCode/model continuation remains unqualified because the binary,
  model/provider settings, and credentials are absent. No live model, mailbox, or
  merge-request side effects were qualified.

The one-hour lateness grace and maximum run duration are enforced by the platform
worker. If the platform is down, it cannot enforce a hard stop at the exact
duration; ownership remains held until recovery confirms cleanup. Results/activity
come from the agent's persisted events and summaries, not independent proof that an
external service accepted an effect. Workspaces without a configured external MCP
connector cannot use mailbox jobs; this feature does not add mailbox OAuth.
