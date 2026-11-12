# Scheduled jobs

Scheduled jobs run repeatable agent instructions independently of workflow tasks
and board stages. Each occurrence gets its own durable session, working files,
activity, and result history.

## Create a job

1. Open **Scheduled jobs** in a workspace and choose **New scheduled job**.
2. Enter a name and the instructions the agent should follow.
3. Select an agent. The job uses that agent's configured runtime, model, skills,
   MCP servers, permissions, and account environment.
4. Choose the repository scope:
   - **None**: the agent uses its configured tools without a repository snapshot.
   - **Selected repositories**: choose one or more registered repositories.
   - **All workspace repositories**: include current repositories and future additions.
5. Choose a daily or weekly schedule, or enter a five-field crontab expression.
   Set the IANA time zone and maximum duration, then use **Preview next runs** to
   check the local and UTC times.
6. Enable the job only when its instructions, repositories, tools, and account
   permissions are correct. Saving an enabled job authorizes future runs.

The workspace must have an agent available. Workspace members with permission to
edit the workspace can create, edit, enable, pause, and run jobs; viewers cannot
change them.

## Schedules and runs

Daily and weekly schedules use the configured local time. Custom schedules use
standard five-field crontab order: `minute hour day-of-month month weekday`.
Lists, ranges, steps, month and weekday names, and Sunday as `0` or `7` are
supported. If both day-of-month and weekday are restricted, either field may match.
Repeated local times during daylight-saving changes run once; nonexistent local
times are skipped.

Use **Run now** for an immediate occurrence or open a job's history to inspect
activity and results. Runs do not overlap for the same job, and the workspace
allows one active scheduled execution at a time. Late schedules and capacity waits
have a one-hour grace period. Pausing a job does not cancel queued or active runs;
failed runs are not automatically retried.

## External effects and recovery

Scheduled jobs can use the selected agent's configured MCP tools and credentials,
including tools for mail or pull requests. Orkest records agent-reported activity;
it does not independently prove that an external service accepted an effect. Before
retrying an uncertain run, reconcile the existing external effect.

Each occurrence preserves its original agent and runtime/model configuration during
recovery. See the [scheduled-job implementation notes](../development/scheduled-jobs.md)
for ownership, migration, rollback, and verification details.
