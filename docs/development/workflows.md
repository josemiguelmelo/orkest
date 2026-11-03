# Workflow implementation

User behavior: [workflows](../guides/workflows.md).

## Event propagation and recovery

```mermaid
flowchart TD
    User[User command] --> Tasks[Task service / transition transaction]
    Tasks --> State[Task state and history]
    Tasks --> Events[PostgreSQL workflow_events]
    Runtime[Persisted terminal run outcome] --> Events
    Events --> Worker[Workflow worker]
    Worker --> Evaluate[Pure automation evaluator]
    Evaluate --> Apply[Validate and apply plan in transaction]
    Apply --> State
    Apply --> Events
    Apply --> Commands[PostgreSQL workflow_commands]
    Commands --> Delivery[Existing start-command delivery]
    Delivery --> Runtime
```

Event transaction:

- Creation, source/user/automation movement persist stage-entry events with task changes.
  Terminal runs atomically record normalized success/failure; manual completion has its own event.
- Events are internal: no publishing API or broker. Idle polling: one second, then drain.
- `FOR UPDATE SKIP LOCKED` claims, latest-configuration reads, task locks, evaluation,
  movement/start enqueueing, and acknowledgement share one transaction; no external execution.
- Movement records another entry event; entry automation is asynchronous. API/UI expose
  `automationPending` separately from execution health.

Movement and delivery:

- User movement returns after commit. Missing assignment/prerequisites produce visible
  failed work when automation runs, without undoing movement.
- Queued/active work conflicts with movement/reassignment. An explicit start accepted
  before entry-event handling supersedes automatic start. No movement rule means stay put.
- At-least-once delivery uses stable event IDs; effects/acknowledgement commit together.
  Stage-entry versions reject stale entries; run IDs reject obsolete same-stage completions.
  Acknowledge obsolete events without effects.
- Record configuration version, matched rule (if any), occurrence time, causation,
  and correlation. Configuration saves emit no events or retroactive starts.
- Exclusive start-delivery locks reconcile stable prompt IDs. Reserve runs before
  external launch; uncertain accepted starts enter recovery, never blind replacement.
- Recheck actor mutation access at dispatch. Imports use the administrator who last
  saved configuration. Ordinary failures require user retry.
- Shutdown cancels/joins platform workers, preserving independent agentctl processes
  and pending approvals. Dispatch holds a database connection; lease-based claims remain proposed.

## Architecture and extension boundaries

| Owner | Responsibility |
| --- | --- |
| `internal/workflow` / `WorkflowConfigurationService` | Definitions, stages, rules, structural validation |
| `internal/task` / `TaskService` | CRUD, comments, repositories, imported-content protection |
| `internal/workflowautomation` / `WorkflowAutomationService` | Movement/evaluation; implements task-owned `TaskWorkflowPlanner` for combined updates |
| `WorkflowExecutionService` | Access, stage-version, accepted-command checks; orchestration dispatch; delivery/work-state effects |
| `WorkflowAutomationWorker` | Scheduling only |

- Task/workflow never import automation; app wires them. Workflow also never imports task.
- Configuration save/delete validates service policy against locked versions, agents,
  occupied stages, and task/integration references; database constraints remain final guards.
- Task creation selects initial stage/assignment under workflow lock, committing task/entry
  event together. Import policy retains local state or supplies explicit source-closure destination to SQL.
- Execution-health reads retain task/run locks until returned effects commit.
- Creation, updates, transitions, events, and execution commands have separate repository
  contracts/adapters. Updates/transitions share latest-workflow/task locking and an I/O-free service callback.
- Compare replays before effects; commit task, history, events, command intent atomically.
  Imported-content/execution-conflict policy stays in services. Package layout does not
  change persisted command JSON, API shapes, or schema.
- [Artifact snapshots](artifacts.md) commit with reservation; [stage bindings](../guides/artifacts.md) define inputs.
- A future PR adapter should verify/deduplicate notifications, resolve artifact→workspace
  task, and persist normalized events. Provider conditions/matchers belong in that adapter.
  PR ingestion, output gates, and scripts remain deferred.
- Returning to In progress and starting revisions are separate; entry rules decide execution.

## Verification

See [testing](testing.md) and [migrations](../operations/migrations.md).
