# Direct conversations

Direct conversations use the same durable launch and agentctl boundaries as
task execution, but the conversation ID is the task-shaped identity. Creating a
conversation creates its first run; subsequent prompts either reuse a ready run
or create a new run that restores the existing direct session.

```mermaid
sequenceDiagram
    participant U as User/API
    participant C as DirectChatService
    participant DB as Conversation/session store
    participant D as TaskDispatcher
    participant L as Launch service
    participant A as Executor/agentctl

    U->>C: Create(agent)
    C->>DB: Persist conversation
    C->>D: StartDirectChat(empty prompt)
    D->>DB: Load current direct session
    alt First run
        D->>DB: Create session configuration
        D->>D: Resolve environment, mounts, capabilities, image
    else Existing session
        D->>D: Refresh MCP servers and restore session
    end
    D->>DB: Reserve direct-session run
    D->>L: Start run
    L->>A: Launch agentctl
    A-->>U: Ready/activity events
```

```mermaid
flowchart TD
    Prompt[Prompt request] --> Check[Load conversation and runs]
    Check --> Valid{Prompt non-empty and run exists?}
    Valid -->|no| Reject[Return validation/not-found error]
    Valid -->|yes| Ready{Selected run ready?}
    Ready -->|yes| Send[Send prompt to existing launch]
    Ready -->|no| Restore[StartDirectChat with prompt]
    Restore --> Session{Current direct session exists?}
    Session -->|yes| Refresh[Refresh session and restore]
    Session -->|no| Create[Create session configuration]
    Refresh --> Reserve[Reserve new direct-session run]
    Create --> Reserve
    Reserve --> Launch[Start or StartWithPrompt]
    Send --> Events[Durable run events]
    Launch --> Events
    Reserve -->|conflict or persistence failure| Retry[Reconcile original run before retry]
    Launch -->|executor failure| Recovery[Reconnect/recover the reserved run]
    Recovery --> Events
```

- Conversation persistence happens before its initial launch. If dispatch fails,
  the conversation remains visible and the failed start must be reconciled;
  creation does not silently roll back the conversation row.
- A follow-up carries `RestoreSession=true` when a current session exists. The
  run reservation is durable before any executor call, and an auto-generated
  command ID is used when a prompted start did not supply one.
- A direct prompt is rejected when its conversation or run is unavailable, and
  blank prompts are rejected. A ready run receives the prompt directly; any
  other run goes through the new reservation/launch path.
- Launch completion uses persisted terminal intent, process termination, and
  final launch status as documented in [execution and recovery](execution-and-recovery.md).
