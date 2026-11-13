# Architecture flows

These diagrams describe the implemented request, state, and recovery flows. They
are intentionally split by domain so that a change to one boundary can be
reviewed without reading a single system-wide diagram.

| Domain | Flow | Primary owners |
| --- | --- | --- |
| [Tasks and workflows](tasks-and-workflows.md) | Task creation, movement, stage automation, and run outcomes | `task`, `workflow`, `workflowautomation` |
| [Execution and recovery](execution-and-recovery.md) | Command delivery, run reservation, executor launch, and fencing | `orchestration`, executors, PostgreSQL |
| [Agentctl runtime](agentctl-runtime.md) | Runtime sessions, prompts, questions, queues, and restore | `agentctl/run`, `agentctl/interaction`, runtime adapters |
| [Artifacts](artifacts.md) | Publication, immutable revisions, and stage handoffs | `artifact`, MCP adapter, workflow dispatch |
| [Scheduled jobs](scheduled-jobs.md) | Schedule matching, occurrence ownership, and completion | `scheduledjob`, orchestration |
| [Integrations](integrations.md) | Repository setup and source synchronization | `coderepository`, `taskintegration`, provider adapters |

## Reading the diagrams

- Solid arrows represent a call or a persisted state transition.
- Dashed arrows represent an implemented adapter or a recovery/reconciliation
  relationship rather than a synchronous call.
- A PostgreSQL transaction boundary is shown as a subgraph when atomicity is
  important. External calls happen outside those transactions unless noted.
- A failed or uncertain state requires an explicit user action unless the
  diagram explicitly shows a safe reconciliation path.

The diagrams complement the detailed implementation notes in
[workflow implementation](../../development/workflows.md),
[session persistence](../../development/sessions.md), and
[agentctl ownership](../../development/agentctl.md).
