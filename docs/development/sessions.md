# Session persistence and continuation

Mechanics behind the [session guide](../guides/sessions.md).

## Reservation and compatibility

- Compatibility includes runtime, provider/model, configuration, and working paths.
  Runtime/format changes must preserve restore plus successful follow-up for existing
  sessions; otherwise preserve state and report incompatibility.
- Unbound historical runs stay readable; never adopt them automatically as restorable.
- Platform snapshots omit MCP headers. Session selection/reservation share a PostgreSQL
  transaction/task lock; a partial unique index prevents overlapping active task runs.
  This does not enforce team-member capacity.

| Manifest | Configuration fingerprints | Behavior |
| --- | --- | --- |
| v1 | Legacy whole-configuration fingerprint | Retains legacy matching; no field-level diagnostics or environment-only exception |
| v2 | Legacy whole fingerprint plus sections | An environment-only section difference may restore |
| v3 (current) | Whole and section fingerprints exclude environment | Environment refresh does not invalidate session identity |

Current fingerprints also exclude the run ID, resolved MCP headers, and structured
Git credentials. Runtime version, working path, provider/model, instructions,
skills, permissions, and MCP endpoints remain compatible-session requirements.
Where available, failure diagnostics name differing sections, never values.

## Queue and run completion

- Sync prompt acceptance before `202`; execute FIFO, one active prompt, including
  follow-ups accepted during work/approvals. ID/text idempotency survives restart.
- Platform outbox records intent; reconcile unacknowledged delivery with the same IDs.
- Serialize acceptance/queue drain; `run_completed` rejects new commands. Prompt
  completion alone does not end the run.
- Stop the container after `run_completed`; persist terminal outcome after confirmed
  cleanup. See [queue controls](../guides/sessions.md#queue-and-completion).

## Restore and ownership

- Before `session/load`, check pinned version, working path, and configuration identity.
  Keep runtime replay separate from live events. All profiles use ACP v1; see
  [native behavior/limits](../reference/runtimes/README.md).
- OS task locks prevent concurrent runtimes. Children inherit descriptors; parent
  loss does not authorize replacement. Journals also require exclusive locks.
- Creation markers prevent retry after uncertain startup. Reconcile first; never
  delete markers to force retry.
- Private, versioned `current-run.json` reloads the last run after daemon restart.
  It may contain MCP credentials: exclude from logs, artifacts, and commits.
- Docker passes `-run-id` to avoid recovering a preceding step. Session containers use
  `unless-stopped`; intentional cleanup stops/removes them.
- agentctl attempts eligible restore after runtime exit; failure becomes
  interrupted/blocked, not an indefinite retry.
- Newly written worker checkpoints omit structured `gitCredentials`. If
  `gitCredentialsRequired` is set, a restarted worker retains its binding but
  cannot restore until the missing credentials are supplied. It accepts matching
  configuration with that missing list replenished; other binding changes still
  conflict. Platform reconnect alone does not establish automatic credential
  reinjection or successful continuation.
- Journal recovery checks committed records, sequence/scope; drops incomplete trailing
  writes; restores command/queue state. Committed corruption blocks recovery.
  Platform history serves chat; see [replay limits](../reference/agentctl-api.md#activity-stream).

Continuation:

- Known queued commands continue after restore.
- Interrupted turns need verified complete replay matching both command marker and
  user message, with no tool calls, before a recorded continuation instruction.
  Missing tool events alone prove nothing without verified replay coverage.
- Missing evidence, tool activity, pending approvals, or uncertain external effects
  pause for user reconciliation. Cancellation stays paused; loading alone proves no exact continuation.
- Never recreate/auto-answer old approval channels. Responses need fresh live
  task/run/session/request bindings.

See [agentctl locks/reducers](agentctl.md), [storage and keys](../operations/storage-and-secrets.md),
and [testing](testing.md).

## Platform delivery fencing

Workflow starts carry a transient delivery token to the reservation transaction.
The token is neither a worker wire field nor part of persisted session
configuration/fingerprints. Claim reassignment fences new reservations from the
old claimant; any already reserved run retains its original execution ownership.
Acknowledgement failure reconciles that reservation/outbox rather than starting a
replacement. See [workflow delivery](workflow-delivery.md).
