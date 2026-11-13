# Execution profiles

Saving an execution profile snapshots its build inputs and enqueues a build in
one database transaction. Workers lease builds so queued work, expired workers,
and uncertain registry outcomes can be handled without concurrent ownership.

```mermaid
flowchart TD
    Save[Admin saves profile] --> Validate[Validate access, runtime, images, registry]
    Validate --> Txn{Profile + build transaction}
    Txn --> Profile[(Profile generation)]
    Txn --> Queue[(Queued build with destination)]
    Queue --> Claim[Worker claims queued build]
    Expired[Building/reconciling lease expires] --> Claim
    Claim --> Owned[Promote profile state and start 90s lease]
    Owned --> Heartbeat[Heartbeat every 20s]
    Owned --> State{Claimed state}
    State -->|building| Resolve[Resolve image digests/inputs]
    Resolve --> Pin[Persist resolved inputs]
    Pin --> Build[Build, push, and validate image]
    State -->|reconciling| Reconcile[Reconcile registry artifact]
    Build --> Finish[Persist build result and promote profile]
    Reconcile --> Finish
    Heartbeat -->|lost ownership| Cancel[Cancel build context]
    Resolve --> Finish
    Pin --> Finish
    Build --> Finish
    Finish --> Ready[ready + execution image]
    Finish --> Failed[failed + safe diagnostic]
    Finish --> Interrupted[interrupted + reconcile-before-retry]
    Cancel --> Interrupted
```

```mermaid
sequenceDiagram
    participant W as Build worker
    participant DB as PostgreSQL
    participant B as Image builder
    participant R as Registry

    W->>DB: Claim queued or expired build (owner + lease)
    DB-->>W: building or reconciling build
    loop Until build ends
        W->>DB: Heartbeat owner lease
    end
    alt New build
        W->>B: ResolveInputs
        B-->>W: Pinned base/toolchain inputs
        W->>DB: Pin resolved inputs
        W->>B: BuildAndPush
        B->>R: Push and validate destination image
    else Expired in-flight build
        W->>B: Reconcile registry artifact
        B->>R: Inspect existing destination
    end
    W->>DB: Finish only if owner still holds build
    DB-->>W: Promote profile and clear lease
```

- A queued build is claimed as `building`; an expired `building` or
  `reconciling` lease is claimed as `reconciling`. Reconciliation inspects the
  registry and does not start another build.
- Input resolution and pinning occur before external build effects. A lost
  heartbeat, timeout, or uncertain reconciliation outcome is persisted as
  `interrupted`, requiring registry reconciliation before an explicit retry.
- Ordinary input, registry, or compatibility errors become `failed` with a
  safe diagnostic. Successful builds become `ready` only after the build image
  and profile promotion are committed under the owner fence.
- Finish is owner-conditional. A stale worker cannot overwrite a newer claim,
  and profile generation prevents an older build from replacing newer inputs.
