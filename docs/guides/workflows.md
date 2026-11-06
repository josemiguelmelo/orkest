# Workspace workflows

- Workspace boards contain 1–50 stages, explicit `initialStageId`/`closedStageId`,
  and up to 200 rules. Every task/source integration selects one.
- Stable stage IDs appear in task `status` for API compatibility. Display order
  controls neither movement, entry, nor closure.

## Configuration and behavior

- Workspace Admins/inherited organization Owners/Admins configure; Members transition.
  Viewers cannot mutate, start, complete stages, configure rules, or answer approvals.
- Stages contain name, description, optional responsible agent. Entry applies that
  assignment or retains the existing one; name/description scope the agent prompt.
- New tasks enter initial stage; source closure goes directly to closed stage.
  Closed tasks cannot reopen. Changing the closed-stage reference changes task closure;
  reordering columns changes no behavior.
- Referenced stages/workflows/agents cannot be deleted; retain at least one workflow.
- Rule fields: stable ID, enabled flag, event, source stage, action, optional target.

| Event | Action | Destination |
| --- | --- | --- |
| `stage_entered` | `start_work` | Current task/stage |
| `run_succeeded` | `move_task` | Explicit different stage |
| `stage_completed` | `move_task` | Explicit different stage |

- One enabled rule per event/source stage; none outgoing from closed stages.
- Entry rules cannot move tasks, preventing movement loops. No expressions/arbitrary actions.
- Success and manual-completion rules are independent. Failure, cancellation, and
  interaction waits never advance/retry automatically.

| Board / start | Behavior |
| --- | --- |
| Seeded | Todo, In progress, In review, Done; In progress entry starts assigned work; success/manual completion there moves to review |
| Custom stages | No automation by default |
| Automated start | Always a new runtime session |
| Explicit start | Independent of movement; new work step with `continue`/`new` and optional prompt |

## API and UI

- Workspace `/workflows`: CRUD, stages, initial/closed references, `rules`.
- `POST .../tasks/{taskID}/transitions`: `move`, `start`, `complete`; requires
  `stageVersion`/`commandId`; execution options apply to starts.
- Legacy task PUT shares locks/planner and preserves content.
- Saves require last-read workflow version; movement/evaluation use latest configuration
  under the same workflow row lock.
- Editor: initial/closed selectors, entry-start toggles, separate success/manual destinations.

## Limits and references

Delivery failure does not undo stage movement. An uncertain delivery requires an
explicit retry after reviewing run state; claim expiry alone never starts another
execution or releases an existing owner. See [delivery ownership](../development/workflow-delivery.md).

PR webhook rules, output completion gates, and scripts remain deferred.
See [implementation](../development/workflows.md), [artifacts](artifacts.md),
[migrations](../operations/migrations.md), and [testing](../development/testing.md).

## Stage custom prompts

Workspace administrators can save an optional custom prompt on each stage (up to
32,768 UTF-8 bytes). Internal Markdown and line breaks are preserved; surrounding
whitespace is trimmed. Empty prompts disable stage-specific instructions.

Every work start includes task details, selected repository paths, stage instructions
when configured, and any user startup message. Startup messages now supplement
task context rather than replacing it. This applies to new conversations, continued
conversations, and review revisions. Ordinary chat does not repeat these instructions.

Changes apply to subsequent starts. Reserved runs retain their composed prompt in
the durable outbox; recovery reconciles that original command without injecting
updated instructions or changing the session's pinned runtime/model configuration.
