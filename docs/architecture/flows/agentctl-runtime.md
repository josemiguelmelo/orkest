# Agentctl runtime

`agentctl` is an independent worker. It owns runtime processes, ACP protocol
translation, durable journals/checkpoints, prompt queues, and interaction
state; the platform observes it through HTTP and activity streams.

```mermaid
flowchart TD
    Platform[Task, direct-chat, or scheduled-job dispatch] --> API[agentctl HTTP API]
    API --> Controller[Run controller]
    Controller --> Journal[(Durable journal and checkpoint)]
    Controller --> Queue[FIFO command queue]
    Controller --> Host[Runtime host]
    Host --> Profile[Runtime profile]
    Host --> ACP[ACP adapter]
    ACP <-->|stdio| Runtime[Coding runtime process]
    Runtime --> Prompt[Prompt result / tool event]
    Prompt --> Journal
    Journal --> Events[Replayable activity events]
    Events --> Stream[HTTP/WebSocket stream]
    Stream --> Platform[Platform orchestration]
```

```mermaid
sequenceDiagram
    participant R as Runtime
    participant C as Conversation
    participant J as Journal
    participant U as User/API

    R->>C: Request question or permission
    C->>J: Persist request and pending identity
    J-->>C: Commit succeeds
    C-->>U: Publish question event
    U->>C: Response bound to task/run/session/request
    C->>J: Persist answer and closed state
    J-->>C: Commit succeeds
    C->>R: Deliver answer
    R-->>C: Continue prompt
    C->>J: Persist prompt completion
```

- A prompt is accepted synchronously, then executes FIFO with one active prompt.
  Follow-ups accepted during work or approval remain queued.
- Question responses are delivered only after their journal record commits.
  Task, run, session, and request identities prevent cross-run responses.
- For task-session runs, prompt completion does not complete the run. The queue
  drains first; terminal run completion then persists terminal intent, stops the
  runtime, and releases confirmed ownership. Scheduled-job runs are the
  exception: `LaunchCompletionService` checks job ownership and treats
  `prompt_completed` or `prompt_failed` as terminal even when a task-session ID
  is present. In both cases the platform persists terminal intent before
  process termination, then writes the final launch status; restart recovery
  retries that cleanup if termination was interrupted.
- Cancellation pauses remaining work. It does not authorize automatic retry or
  auto-answering an old interaction.
- Restore checks runtime/provider/model, configuration fingerprints, working
  paths, locks, and replay evidence. Missing evidence or credentials blocks
  continuation visibly while retaining files and state.
