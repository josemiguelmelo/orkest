# Scheduled jobs

Scheduled jobs are workspace-scoped and independent of task stages. Each
occurrence gets separate durable execution identity and retains its original
configuration during recovery.

```mermaid
flowchart TD
    Edit[Create or edit job] --> Validate[Validate schedule, agent, repositories, access]
    Validate --> Save[(Persist immutable job revision)]
    Save --> Enabled{Enabled?}
    Enabled -->|no| Paused[No future occurrence is created]
    Enabled -->|yes| Calendar[Calendar worker finds due occurrence]
    RunNow[Run now] --> Calendar
    Calendar --> Grace{Within lateness grace?}
    Grace -->|no| Skip[Record skipped occurrence]
    Grace -->|yes| Reserve[Reserve job occurrence]
    Reserve --> Capacity{Workspace/job capacity available?}
    Capacity -->|no| Wait[Remain queued until deadline]
    Capacity -->|yes| Launch[Create session/run with pinned config]
    Launch --> Execute[agentctl execution]
    Execute --> Complete[Persist outcome and cleanup]
    Complete --> History[(Scheduled job run history)]
```

```mermaid
flowchart LR
    Crash[Platform or worker interruption] --> Existing{Existing occurrence accepted?}
    Existing -->|yes| Reconnect[Reconnect/recover original run]
    Existing -->|no| Failed[Visible uncertain/failed occurrence]
    Reconnect --> Complete[Complete original occurrence]
    Failed --> User[Explicit user retry]
```

- Daily, weekly, and five-field cron schedules are evaluated in the configured
  IANA time zone. Repeated DST times run once; nonexistent local times are
  skipped.
- A job does not overlap itself, and the workspace allows one active scheduled
  execution. Approval waits and uncertain ownership retain capacity.
- Pausing does not cancel queued or active occurrences. Failed runs are not
  automatically retried; the next calendar occurrence is independent.
- Repository IDs and the agent/runtime/model configuration are snapshotted for
  the occurrence and revalidated at launch. External effects remain
  agent-reported and require reconciliation before a user retry.
