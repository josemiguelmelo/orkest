# Scheduled jobs

Scheduled jobs run an agent instruction on a workspace calendar. They are
independent of tasks, workflow stages, and board transitions. Each occurrence
gets its own durable run and session, so a job can keep a history without
sharing task state.

## Create a job

Workspace members who can write to the workspace can create and edit jobs from
**Scheduled jobs** in the workspace navigation. Viewers can inspect jobs and
run history but cannot create, edit, run, cancel, or answer a job run.

Configure these fields:

- **Name** and **Instructions** — the instruction sent to the selected agent.
  Names are limited to 200 characters; instructions are limited to 60,000
  characters.
- **Agent** — an existing workspace agent, including its configured runtime,
  skills, tools, and account permissions.
- **Repository scope** — choose no repositories, selected repositories, or all
  workspace repositories. The `all` option includes repositories added later.
  Selected repositories are checked when the job is saved and snapshotted for
  each occurrence.
- **Schedule** — daily, weekly, or custom crontab, plus an IANA time zone.
- **Maximum duration** — from 1 to 1,440 minutes. The default is 60 minutes.
- **Enabled** — explicitly authorizes future calendar occurrences. Saving a job
  disabled does not authorize future scheduled runs.

The editor can preview the next five occurrences in the configured time zone
and UTC. Save a job with `Enabled` off while preparing it; enable it when the
instruction, agent, repository scope, and external permissions are ready.

## Schedule syntax

Daily and weekly schedules use the selected local time. Weekly weekdays use
Sunday = 0 through Saturday = 6. Custom schedules use the standard five-field
crontab order:

```text
minute hour day-of-month month weekday
```

Lists, ranges, steps, month names, and weekday names are supported. Sunday may
be written as `0` or `7`. For example, `0 9 * * 1-5` runs at 09:00 on weekdays.
If both day-of-month and weekday are restricted, either field may match,
following standard cron semantics.

Times are interpreted in the configured IANA time zone. A repeated local time
during the autumn DST transition runs once; a nonexistent local time during the
spring transition is skipped.

## Run and review a job

Use **Run now** to enqueue a manual occurrence. The same command can be safely
reconciled if the request result is uncertain; do not submit a second manual run
until the existing occurrence is checked in history.

Each occurrence records the job revision and the exact repository IDs used at
dispatch time. The history view shows its state, activity, questions,
permissions, and result. A pending or running occurrence can be cancelled. If an
agent asks a question or requests permission, answer it from the occurrence
details; approval waits continue to hold the job's execution capacity.

Scheduled work is not automatically retried after failure. A missed occurrence
and the next calendar occurrence are separate runs. There is no same-job
overlap, and a workspace permits only one active scheduled execution at a time.

## External effects and security

The agent operates with the selected agent's configured tools and the account
permissions available to the worker. Orkest records agent events and summaries;
it does not independently prove that an external email, API request, commit, or
merge request was accepted. For an uncertain external effect, inspect the
target service and reconcile the existing effect before retrying.

Repository scope does not configure credentials. Configure the required coding
integration or MCP server separately, and avoid putting credentials in job
instructions, logs, commits, or examples. Mailbox work requires an authorized
external connector; scheduled jobs do not add native mailbox OAuth.

## API

The same operations are available under the authenticated workspace API:

```text
GET  /api/organizations/{orgID}/workspaces/{workspaceID}/scheduled-jobs
POST /api/organizations/{orgID}/workspaces/{workspaceID}/scheduled-jobs
POST /api/organizations/{orgID}/workspaces/{workspaceID}/scheduled-jobs/preview
GET  /api/organizations/{orgID}/workspaces/{workspaceID}/scheduled-jobs/{jobID}/runs
POST /api/organizations/{orgID}/workspaces/{workspaceID}/scheduled-jobs/{jobID}/runs
```

Use the interactive contract at `/docs/` or `/openapi.yaml` for request and
response schemas, run events, cancellation, permission responses, and question
responses. Create and update requests include a `commandId`; updates also
include the last-read `revision` to prevent overwriting another edit.

## Operations

Scheduled jobs are workspace-scoped and persist in PostgreSQL. Before upgrading
from the older workflow-delivery implementation, stop old platform instances
before activating jobs; old completion code cannot interpret job ownership.
For migration, rollback, recovery, and verification boundaries, see the
[development reference](../development/scheduled-jobs.md) and
[migration guidance](../operations/migrations.md).
