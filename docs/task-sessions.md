# Durable task sessions

A task has one current conversation, identified by `taskSessionId`, and may retain
older conversations. A run is one explicit work step; several runs can load the
same runtime session. Infrastructure recovery retains the run ID. Assignment
and status changes outside an explicit work start do not launch anything.

## Starting and iterating

The task start controls offer **Continue session** (default) and **New session**
when history exists. The chat composer remains visible after each run ends.
Sending a follow-up starts another run in the same saved session when needed,
including before the task-status refresh catches up. The composer’s session
selector makes starting a fresh conversation an explicit choice; ordinary
follow-ups do not require a start dialog. Done tasks and viewers remain read-only.
A new session snapshots the currently assigned agent; continuing retains the
original agent, model, instructions, skills, and MCP endpoint configuration.
MCP credentials are resolved again by server ID. New sessions preserve previous
chat, working trees, untracked files, and task branches.

The task update API accepts `sessionMode: continue|new`, optional `promptText`,
and a stable `commandId`. `startWork: true` explicitly requests another step
when the task is already in progress after a failed run. Repeated start commands
are reconciled against the task's durable command history. Historical runs without
a session binding are readable and are not automatically adopted as restorable.

The platform stores session snapshots without MCP credential headers. Session
selection and run reservation share a PostgreSQL transaction and task row lock;
a partial unique index rejects overlapping active runs for the task. This does
not implement team-member slot allocation, which remains separate work.

## Queue and completion

Durable sessions accept follow-up prompts while another prompt or approval is
active. agentctl syncs each acceptance before returning `202`, then executes FIFO
with one active prompt. Commands remain idempotent by ID and text across restart.
The platform also records delivery intent in a prompt outbox and reconciles
unacknowledged delivery using those same identities.

A completed prompt is not a completed run. agentctl serializes acceptance and
queue drain; `run_completed` fences further new commands. The platform stops the
container only after that event and moves an in-progress task to review after
confirmed cleanup. Failure or cancellation pauses pending prompts; **Resume
queue** explicitly continues the remaining queue, and **Discard queued prompts**
ends a failed step without moving the task to review. When restoration is blocked,
explicit discard confirms environment termination before allowing another run. Interrupted prompts are
not silently resent by the resume-queue action.

## Restore and ownership

Configure agentctl with a stable `taskSessionId`, then call `POST /agent/launch`
for creation or `POST /agent/restore` for loading. The restore operation uses the
saved runtime identity and does not create a replacement on failure. `GET /info`
includes session identity, active command, queue depth, paused/ended state, and
`recoveryBlocked`. Command lookup is `GET /agent/commands/{commandID}`. Queue
controls are `POST /agent/queue/resume` and `/agent/queue/discard`.

Task working paths and the entire agentctl state directory must survive.
OpenCode 2.1.4, Claude Code's `claude-agent-acp` 0.99.0 adapter, Codex's
`codex-acp` 2.3.0 adapter, GitHub Copilot CLI 1.0.105, and Kiro CLI 2.37.0 use
ACP v1. Kiro uses the V3 harness and newline-delimited JSON-RPC framing. Copilot's
ACP server is in public preview; its interrupted-turn replay remains
unqualified and pauses when the journal cannot establish the outcome.
Session manifests verify the runtime version, working path, and configuration
identity before calling `session/load`. Claude Code's real interrupted-work
continuation remains unqualified and pauses when replay evidence is incomplete.
Runtime history replay is kept separate from live conversation events.

An OS file lock prevents concurrent task runtimes. Its descriptor is inherited
by the runtime process, so losing the parent does not itself authorize a new
owner. Journals also hold exclusive locks. Retained creation markers prevent
silently retrying session creation after uncertain startup. Never delete markers
to force a retry; reconcile the previous execution first.

A private, versioned `current-run.json` checkpoint allows daemon restart to load
the last run. It can contain MCP credentials and must be protected as secret
runtime state, excluded from logs, artifacts, and commits. Docker supplies
`-run-id` so a new work step does not recover the preceding run accidentally.
Session-enabled containers use Docker's `unless-stopped` policy; intentional
platform cleanup stops/removes them. agentctl also attempts restoration when
its runtime subprocess stops. Failed restoration is exposed as interrupted and
blocked rather than retried indefinitely.

Journal recovery validates committed records and sequence/scope, discards an
incomplete trailing write, restores command/queue state, and retains the last
4096 events in memory. Durable sessions replay older cursors from disk, so a long
platform disconnection does not lose chat activity. The platform's persisted
history serves task chat. Corrupt committed history prevents recovery.

Known queued commands continue automatically after restoration. An interrupted
turn can automatically receive a recorded continuation instruction only when
runtime replay matches its stable command marker and user message, and contains no tool calls for that
turn. Missing evidence, tool activity, pending approvals, and uncertain external
effects pause execution for user reconciliation. Intentional cancellation stays
paused. Session loading alone is not proof of exact interrupted-work continuation.
Old approval response channels are never recreated or automatically answered;
responses must match a fresh live task/run/session/request binding.

## Verification and operations

- `make check`: lint, backend/frontend tests, generated API checks, and builds.
- `make test-recovery-unit`: deterministic queue/replay/restore tests under race detection.
- `ORKEST_OPENCODE_BINARY=/absolute/path/opencode make test-integration`: real
  handshake and session loading without model calls. Model continuation skips
  explicitly unless model settings are supplied.
- `make test-recovery`: deterministic tests plus real model continuation. Requires
  `ORKEST_OPENCODE_BINARY`, `ORKEST_TEST_MODEL_PROVIDER`, `ORKEST_TEST_MODEL_ID`,
  and any required credential variable names in comma-separated
  `ORKEST_TEST_PROVIDER_ENV`. No paid model is selected implicitly.
- `make test-docker`: real daemon restart, container replacement, fresh-session
  identity, and working-file preservation without model calls. The Go Docker
  client needs `DOCKER_HOST` if the daemon uses a non-default socket.
- `make test-postgres`: real session/run reservation and outbox tests using the
  documented PostgreSQL connection environment.

Migration 0018 preserves existing runs without inventing session identities.
Its active-task unique index intentionally fails if older data already contains
overlapping active runs: reconcile those executions before applying it. Rolling
back drops the new session metadata and prompt outbox; it does not delete task
working files. Complete host/disk loss remains outside the recovery guarantee.
