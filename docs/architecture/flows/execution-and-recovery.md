# Execution and recovery

Workflow commands use durable, fenced delivery. The platform reserves a stable
run identity before contacting an executor, and an expired claim can reconcile
an accepted run but cannot authorize a replacement launch.

```mermaid
sequenceDiagram
    participant W as Delivery worker
    participant DB as PostgreSQL
    participant O as Orchestration service
    participant E as Executor
    participant A as agentctl

    W->>DB: Claim pending command (token + deadline)
    DB-->>W: Stable command and claim token
    W->>DB: Recheck acceptance / reserve run
    DB-->>W: Run, session, and prompt identity
    W->>O: Dispatch outside transaction
    O->>E: Create environment and start agentctl
    E->>A: Start independent worker
    A-->>O: HTTP readiness and activity stream
    O-->>W: External result / connection status
    W->>DB: Fenced acknowledgement transaction
    DB->>DB: Lock task/run, apply work state and effects
    DB-->>W: Acknowledge command
```

```mermaid
flowchart TD
    Claim[Claim expires or worker crashes] --> Reconcile{Was a run accepted?}
    Reconcile -->|yes| Existing[Reconcile original run/session/prompt]
    Existing --> Ack[Retry fenced acknowledgement]
    Reconcile -->|no| Uncertain[Mark delivery_uncertain / failed work]
    Uncertain --> User[User reviews and explicitly retries]
    Ack --> Terminal{Terminal outcome known?}
    Terminal -->|yes| Effects[Apply conditional stage-work effects]
    Terminal -->|no| Wait[Keep ownership; continue recovery]
    Shutdown[Platform shutdown] --> Join[Cancel and join platform workers]
    Shutdown --> Keep[Leave independent agentctl processes running]
```

- Claims grant permission to reconcile or deliver one command, not ownership of
  the task or an already-running process.
- Claim, reservation, launch, and acknowledgement are deliberately separate:
  database transactions are short and never contain external process or HTTP
  calls.
- Claim tokens fence stale workers. Run IDs fence obsolete same-stage terminal
  outcomes, and stage-entry versions reject stale entries.
- Missing acceptance is not proof that an external call did not happen. The
  platform records uncertainty rather than blindly launching a second run.
- Executor selection is an adapter concern: Docker, Local, and Kubernetes
  create the environment while orchestration owns run/session identity.
