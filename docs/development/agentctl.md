# Agentctl domains and runtime providers

Agentctl is organized by ownership under `internal/agentctl`. Start with
`cmd/agentctl/main.go` for executable wiring, then `run/controller.go` for the
lifecycle of one execution run.

| Package | Owns |
| --- | --- |
| `config` | Resolved run/runtime settings, validation, cloning, executor checkout inputs |
| `interaction` | Commands, command status, approvals, updates, durable event records, command marker encoding |
| `run` | Controller, private conversation engine/reducer, queue, journal, subscriptions, checkpoints, restoration policy |
| `runtime` | [Normalized host/session contracts](../../internal/agentctl/runtime/contracts.go), prompt observers, stop reasons, provisioning profiles, protocol contracts, sanitized runtime errors |
| `runtimehost` | Subprocess and stdio ownership, provisioning application, native session manifests, task leases, replay evidence |
| `runtime/acp` | ACP SDK communication and translation into normalized values |
| `runtime/opencode` | Pure OpenCode native configuration rendering |
| `runtime/claudecode` | Claude Code ACP session options and isolated configuration paths |
| `runtime/githubcopilot` | GitHub Copilot CLI ACP startup, model, permissions, instructions, and skills |
| `runtime/kiro` | Kiro CLI ACP V3 startup, model, permissions, instructions, and skills |
| `httpapi` | One REST/WebSocket server, transport-owned controller interface, wire representations |
| `daemon` | Standalone server lifecycle and periodic restoration supervision |

## Dependencies

- Import the owning domain directly; no root forwarding package.
- `config`/`interaction` do not depend on execution. Runtime contracts consume
  their values; controllers, hosts, protocol clients, and profiles consume those contracts.
- Concrete runtimes never import run, HTTP, or platform services.
- `make check-runtime-boundaries` enforces these directions.

## Run and conversation ownership

| `run.RunController` operation | Effect |
| --- | --- |
| Configure | Bind settings without starting work |
| `Launch` | Create native session |
| `RestoreSession` | Replace a terminated runtime from its durable session |
| `RecoverFromCheckpoint` | Reload configured run after daemon restart |

The daemon schedules supervision; the controller decides eligibility.

- Keep the private conversation engine with `run`: binding, shutdown, admission,
  journal commits, and recovery share lifecycle/locks. Focused files separate commands,
  permissions, queues, replay, event commits, and subscriptions without forwarding interfaces.
- `conversation.state` is durable reducer state. Prompt contexts, permission
  channels, workers, and observed progress are transient. Live commits/replay use one reducer.
- Commands retain original prompt hash and typed state; continuation instructions
  never replace accepted identity. Prompts return typed `runtime.StopReason`, including cancellation.
- Lock order: `turnMu` → `eventMu` → `mu`. `UnderEventLock`/`UnderTurnAndEventLocks`
  require caller-held named locks and acquire state locks internally.
- Journal/runtime I/O excludes the state lock, keeping status/shutdown responsive.
  Cancellation holds `turnMu` through delivery to protect later queued commands.
  Replay/live handoff preserves gap-free ordering.
- Private `run` checkpoint storage owns reads/writes. Checkpoint/reconciliation
  failures share startup cleanup. Shutdown fences publication under the controller
  lock; late unpublished sessions are closed.
- Only journaled infrastructure interruptions permit automatic recovery; ordinary
  failures, cancellation, and terminal runs never auto-retry. Late activity/permission
  events retain recorded eligibility; legacy non-task behavior is unchanged.

## Host, profiles, and protocol

`runtimehost.Host` receives `runtime.RuntimeProfile`,
`runtime.ProtocolClientFactory`, and granted environment values through constructors.
Startup:

1. Validate configuration/paths; acquire exclusive task ownership; load/reserve the v1 native session.
2. Apply provisioning; probe pinned executable; start child/stdio.
3. Initialize ACP; verify protocol version and mandatory loading support.
4. Explicitly create/load session, apply native mode, persist manifest.
5. On failure, close partial resources before releasing ownership.

Ownership and provisioning:

- `runtimehost.Session` owns child, protocol connection, callbacks, and task lease.
  Children inherit leases to prevent duplicates after parent loss. Shutdown joins
  concurrent callers and confirms termination before releasing ownership.
- Manifests/checkpoints use `internal/core/durablefile`: atomic writes with file
  and directory sync, preserving storage identity/paths.
- Pure profiles receive resolved runtime settings/paths, excluding organization,
  workspace, task, run, process, and storage state; no operational I/O.
- The host checks relative paths, root escapes, immutable-file conflicts, and reserved
  environment names; creates private files/directories; merges granted variables.
  `ProvisioningPaths.ConfigSubdir` is relative to its absolute runtime root.
- `runtime/acp` binds owned streams to the SDK; no process, filesystem, persistence,
  or platform operations. SDK types stay internal; callbacks carry
  `interaction.Update`/`interaction.Permission`. Create/load receives its working directory directly.
- `runtime/opencode/profile.go` owns agent Markdown, native names, model/mode,
  skills, HTTP MCP JSON, XDG paths, and `OPENCODE_*` values.
- Profiles translate permission policies; interactive approvals use shared callbacks.

Replay evidence:

- `interaction` owns marker encoding; run/host own reconciliation/evidence.
- `LoadSession` collects normalized observations, sealed after load and preceding
  callbacks complete. Replay is never live output.
- Missing/incomplete/ambiguous/unsupported history or tool effects pause recovery.
  Require command identity; equal prompt text is insufficient.
- Absence of tool activity supports continuation only with `ReplayCoverageVerified`
  attesting complete coverage. OpenCode 2.1.4 has no attestation.
- Recovery never silently creates a new session.

## Transport, compatibility, and verification

- One `httpapi.Server` shares controller state, timestamp, router, logs, and contract
  across REST/receive-only WebSocket. Server cancellation closes streams independently
  of accepted execution.
- `httpapi/protocol` maps wire/domain values without exposing resolved secrets.
- Preserve payloads, event names, markers, checkpoints, manifests, fingerprints,
  ownership locks, and working paths when moving packages.

| Tests | Owner |
| --- | --- |
| Command encoding | `interaction` |
| Execution, recovery, configuration acceptance | `run` |
| ACP translation | `runtime/acp` |
| Host/storage | `runtimehost` |
| Pure provisioning | `runtime/opencode`, `runtime/claudecode` |
| Combined runtime wiring | `runtime/*_test.go` |

Fixtures share `opencodetest.NewHost`. See [testing](testing.md), [status](../status.md),
and [runtime setup](../reference/runtimes/README.md).

## Model selection

OpenCode sets the native model default and confirms it through ACP
`session/set_config_option` before prompts. Unavailable/unconfirmed models fail
startup; restored sessions retain their model. Pins: [version inventory](../../internal/core/runtime/versions.json).
