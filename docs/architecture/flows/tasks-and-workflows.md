# Tasks and workflows

Task content and workflow position are separate concerns. User actions and
imports update task state under the workflow lock; automation reacts to durable
events and never silently undoes a committed movement.

```mermaid
flowchart TD
    User[User or task-source import] --> TaskAPI[Task API / import policy]
    TaskAPI --> Txn{Task/workflow transaction}
    Txn --> TaskState[(Task content, stage, assignment)]
    Txn --> Entry[stage_entered event]
    Entry --> Worker[Workflow automation worker]
    Worker --> Evaluate[Load latest workflow and evaluate rule]
    Evaluate --> Plan[Build stage plan and execution command]
    Plan --> Apply{Apply plan transaction}
    Apply --> History[(Task history)]
    Apply --> Stage[(Current stage and work state)]
    Apply --> Command[(workflow command / outbox)]
    Command --> Delivery[Execution delivery]
    Delivery --> Run[Run terminal outcome]
    Run --> Outcome[run_succeeded or run_failed event]
    Outcome --> Worker
    User -->|move, start, complete| Transition[Transition API]
    Transition --> Apply
    Apply -->|success/manual completion rule| Outcome
```

## Important branches

```mermaid
flowchart LR
    Entry[Stage entry] --> Rule{Enabled start_work rule?}
    Rule -->|no| Idle[Remain in stage]
    Rule -->|yes| Start[Queue new work]
    Run[Run outcome] --> Success{Success or manual completion?}
    Success -->|no: failure/cancel/wait| Paused[Visible failed or paused work]
    Success -->|yes| Move{Move rule?}
    Move -->|no| Review[Remain in stage]
    Move -->|yes| NewStage[Commit movement and emit new stage entry]
    NewStage --> Entry
```

- Entry rules can start work but cannot move tasks, which prevents movement
  loops.
- Assignment, reopening, and movement do not start execution unless a rule or
  explicit start creates a command.
- A committed movement survives a delivery failure. An uncertain delivery is
  visible and requires explicit retry after reconciliation.
- The seeded board uses `In progress` entry to start work and success/manual
  completion to move to `In review`; custom workflows have no implicit rules.
- Workflow configuration saves validate against locked references and do not
  emit retroactive events.
