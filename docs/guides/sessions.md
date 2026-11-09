# Durable task sessions

- One current conversation per task; older sessions may remain.
- One work step per run; sessions span runs. Infrastructure recovery retains run ID.
- Assignment never starts work; configured [entry rules](workflows.md) can.
- Direct workspace/agent conversations share task-chat history, replay, approvals,
  cancellation, and queues. Creation starts the runtime; first user message is its
  first prompt. No task, task-context prompt, or workflow change.

## Interaction flow

```mermaid
flowchart TD
    Start[Explicit start or configured stage entry] -->|Choose session| Session[Create or restore session]
    Session -->|Start work| Prompt[Execute next queued prompt]
    Prompt -->|approval requested| Wait[Wait for bound user response]
    Wait -->|response delivered| Prompt
    Prompt -->|prompt succeeds| Queue{More accepted prompts?}
    Queue -->|yes| Prompt
    Queue -->|no| End[Complete run and release confirmed ownership]
    Prompt -->|failure or cancellation| Pause[Pause queue for user action]
    End -->|Record outcome| Outcome[Workflow evaluates run outcome]
```

## Start and iterate

- History enables **Continue session** (default) / **New session**.
- Composer stays visible after completion. Follow-ups start runs as needed, even
  before status refresh; no start dialog. Composer can explicitly choose a fresh session.
- Done tasks/Viewers are read-only.

Agents can request structured input through a question card in task or direct chat.
Choose/type an answer, decline, or cancel that question; cancelling the entire run
remains a separate action. Questions retain the execution reservation. Chat messages
do not answer pending forms. Recorded answers and closed questions remain in history;
an interrupted delivery is shown as unresolved. Supported paths and live qualification
limits are listed in [runtime setup](../reference/runtimes/README.md).

| Choice | Configuration | Retained work |
| --- | --- | --- |
| Continue session | Original agent, runtime, provider/model, instructions, skills, permissions, and MCP endpoints | Saved conversation, working trees, untracked files, task branches |
| New session | Current assigned agent configuration | Previous chat history, working trees, untracked files, task branches |

- Re-resolve MCP headers by server ID; apply [environment continuation rules](agent-configuration.md#environment-and-session-changes).
- Incompatibility fails visibly, retaining state; no silent model/conversation substitution.
- Task context is included in the runtime instructions when a session starts, so
  it does not appear as a user chat message. Follow-ups send only the submitted
  message while that context is unchanged. Changed task, stage, or artifact context
  is sent as a focused update; older sessions without a context baseline receive
  one full refresh before follow-ups are minimized. Starting a new session always
  receives the current full context.

## Queue and completion

- Follow-ups queue durably FIFO during work/approvals. Acceptance means stored, not completed.
- Queue drain ends the run; success rules move stages (seeded board: review).
- Failure/cancellation pauses pending prompts.
- **Resume queue** continues remaining prompts without silently resending interrupted work.
- **Discard queued prompts** ends failed work without review movement. Blocked restore
  requires confirmed environment termination before another run.

## Recovery

- Platform shutdown leaves workers running. Restore compatible state/known queues where safe.
- Missing replay evidence, tool activity, pending approvals, or uncertain effects
  pause interrupted work for reconciliation. Cancellation stays paused; failed restore is interrupted/blocked.
- Workflow delivery claims can expire without ending a reserved run. Accepted
  commands reconcile against their existing identities. An uncertain unaccepted
  delivery becomes failed work requiring explicit user retry; it does not
  automatically start again. [Delivery details](../development/workflow-delivery.md).
- Worker restart may require fresh Git credentials before restore; reconnect
  alone does not establish continuation. [Recovery requirements](../development/sessions.md#restore-and-ownership).
- Loading alone proves no interrupted-work continuation. Retain files/state; never
  delete locks/creation markers to force replacement.

References: [storage/recovery](../operations/storage-and-secrets.md), [runtime limits](../reference/runtimes/README.md),
[status](../status.md), [worker API](../reference/agentctl-api.md),
[platform API](../../internal/adapters/http/server/openapi.yaml), [session internals](../development/sessions.md),
[testing](../development/testing.md).
