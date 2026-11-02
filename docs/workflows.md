# Workspace workflows

Issue #52 replaces the fixed stage catalog with ordered, versioned workflows
per workspace. Every task is bound to one workflow. Manual task creation selects
the workflow, and each source integration selects the workflow for imported
tasks. Stage IDs are stable keys within a workflow; task `status` contains the
stage key for API compatibility. Names, descriptions and order are configurable.
Stages are ordered left to right. Tasks start in the first stage, and source-
closed tasks move to the final stage; these positions are derived from the stage
order rather than configured separately. There may be 1–50 stages per workflow.

## Configuration and behavior

Workspace admins and inherited organization owners/admins manage configuration.
Members can transition tasks; viewers cannot mutate configuration, move tasks,
start work, complete stages, or answer runtime approvals.

Entering a stage assigns its responsible agent when configured; otherwise the
current assignment is retained. Auto-start enqueues a durable command. An assigned
agent is required to start. A missing assignment produces a conflict for explicit
entry, or a visible failed work indicator on an imported task. Stage names and
descriptions are included in the task prompt to describe the current work scope.

Successful agent execution completes its stage after process ownership is
resolved. Auto-advance moves to the next ordered stage; the destination's entry
policy then applies, including assignment and optional start. Auto-advance is off
for new stages. Without auto-advance, the task remains in place with completed
stage work. Users move it explicitly. Users can also complete manual stages;
they follow the same auto-advance setting. Failure, cancellation, input and
approval waits never advance a task. Failed starts require an explicit user retry.
Starting again in the same stage creates another work step. It does not implicitly
move the task to a stage named `in_progress`.

The final stage cannot auto-start or auto-advance. Tasks that entered the final
stage cannot reopen.
Stage movement and reassignment are rejected while a start is queued or execution
is active. Ordinary content edits remain possible.

Stage entry snapshots the workflow. Its completion and next-stage choice use that
snapshot. Destination entry uses the current destination configuration. Editing
configuration does not launch existing tasks or alter an active run. If an
unoccupied future stage is removed while a preceding run is active, completion
stays in the current stage, marked complete, for a user to select a destination.
Deleting a stage occupied by one or more tasks is rejected by the task stage
foreign key. Deleting a workflow referenced by tasks or source integrations is
rejected; a workspace must retain at least one workflow. An agent referenced by
a workflow stage must be removed from that configuration before its agent
definition is deleted.

Manual task creation and new open imports enter the first stage. Source
polling applies entry effects only for a new task or source-driven stage change;
repeated polling never repeats a start. Import automation runs on behalf of the
admin who last saved the workflow; their current mutation access is checked before
execution. A source closure moves directly to the final stage and
invalidates the prior stage entry. Existing external work is still reconciled
independently; a source change does not prove process termination.

## Architecture

`internal/workflow` owns configuration validation and the pure transition planner.
The task service owns task access and user commands. Consumer-owned repository
contracts and constructor injection connect these to PostgreSQL and orchestration.
Stage variants are data. Additional completion strategies can be introduced when
there is a concrete second completion mechanism.

PostgreSQL transactions serialize workflow configuration and task transitions,
check versions, change assignment/stage, record history and persist commands
atomically. Settings saves use the workflow version; transition commands require
the task's stage version and a command ID. Reusing a command ID with a different
transition payload is rejected. Task reads include stage identity, work state and
version from the same database row snapshot.

A bounded worker holds an exclusive outbox row lock through dispatch. Workers use
`SKIP LOCKED`; platform shutdown cancels and joins the worker. Delivery reconciles
the stable command identity against the existing prompt outbox before any start.
Reservation persists a run before external launch and rejects duplicate command
identities. Accepted but uncertain starts belong to existing launch recovery and
are never blindly replaced. Delivery is at least once, with idempotent admission;
external effects are not assumed to be exactly once.

Stage versions and the currently bound run ID fence old completion events,
including retries within the same stage. Terminal run status, stage changes and
any next-stage command commit together after process cleanup. The platform worker
does not own the agentctl lifecycle and never auto-approves runtime permissions.

The worker currently processes commands sequentially per platform process and
holds a database connection while dispatching. This bounds concurrent startup
work; a lease-based claim can replace that transaction boundary if dispatch
throughput requires it. Configuration changes and stage moves serialize per
workspace to keep configuration reads and entry snapshots consistent.

## API and UI

- `GET/POST .../workspaces/{workspaceID}/workflows`: list or create workflows.
- `GET/PUT/DELETE .../workspaces/{workspaceID}/workflows/{workflowID}`: read,
  replace ordered stages, or delete a workflow. PUT requires the last-read
  configuration version; DELETE conflicts while tasks or integrations use it.
- `POST .../tasks/{taskID}/transitions`: `move`, `start`, or `complete`, with
  `stageVersion`, `commandId`, and optional session choice/prompt.
- Existing task PUT remains compatible and uses the same transaction planner.
- The Workflow navigation page edits stage metadata, responsible agents, ordering
  and automation. The board, selectors and filters load this definition.
- Task responses expose `stageVersion`, `stageWorkState`, and `closed` separately
  from runtime execution health. No runtime transport addresses are exposed.

Automation defaults to a fresh runtime session so a stage's newly assigned agent
configuration takes effect. Explicit user starts may choose `continue` or `new`.
Continuing intentionally retains the saved runtime session's effective settings.

## Migrations and operations

0035 seeds existing and future workspaces with Todo, In progress, In review, Done;
existing tasks retain their stage keys. The seeded In progress stage explicitly
enables start and advance to preserve entering-stage execution/review behavior.
Custom stages default both automation switches off.

0036 binds runs to their stage entry and constrains responsible agents to their
workspace. 0037 defers reference checks for repository/stage links until commit so
workspace deletion can cascade in any order while individual referenced resource
deletion remains prohibited. No migration deletes task working files or stops
external processes. 0038 promotes the seeded workspace workflow to a named
workflow, associates existing tasks and integrations with it, and adds
workflow-scoped stage identities. Its rollback refuses to proceed while extra
workflows or resources reference them.

Rollback of 0035 deletes workflow configuration, stage history and workflow
commands, and maps custom stage keys to `todo`. Back up before rollback. Rolling
back during active automation requires stopping platform command delivery and
reconciling existing runs first.

Verification: `make check`; `make test-postgres` with documented PostgreSQL
settings; race detection for workflow, task, orchestration and PostgreSQL tests.
Runtime model execution and multi-stage live provider behavior require configured
runtimes/credentials and are separate from deterministic delivery verification.
