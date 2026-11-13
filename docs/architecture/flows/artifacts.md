# Artifacts

Artifacts are task-scoped immutable revisions. Agents publish through a
run-bound capability; users and browsers can read revisions but cannot claim a
producing run.

```mermaid
sequenceDiagram
    participant Agent as Agent/MCP tool
    participant API as Artifact service
    participant DB as PostgreSQL
    participant UI as User/API reader

    Agent->>API: Publish(kind, key, expected revision, content)
    API->>API: Validate task/workspace, membership, run and stage binding
    API->>DB: Lock task and check idempotency
    alt New or valid update
        DB->>DB: Append immutable revision and advance current revision
        DB-->>API: New revision
    else Identical retry
        DB-->>API: Original revision
    else Stale or changed retry
        DB-->>API: Conflict
    end
    API-->>Agent: Revision result
    UI->>API: List current or revision history
    API-->>UI: Task-scoped artifact data
```

```mermaid
flowchart LR
    Producer[Producer stage output] --> Revisions[(Artifact revisions)]
    Revisions --> Select[Dispatch selects newest matching source entry]
    Select --> Snapshot[Pin input revision IDs in prompt]
    Snapshot --> Consumer[Consumer stage run]
    Consumer --> Expected[Publish expected outputs]
    Expected --> Revisions
```

- Every retry uses the same task-unique idempotency key. The same actor/run and
  input returns the original revision; a changed input or stale revision is a
  conflict.
- Input selection happens before reservation and the exact prompt/outbox stores
  the selected revision IDs. Later artifact edits cannot change an accepted run.
- Required inputs reject a consumer start and expose an automation failure;
  optional inputs resolve to an empty list. Artifact outputs do not gate producer
  completion.
- External URLs are references, not fetched snapshots. Provider IDs belong in
  metadata when reproducibility matters.
