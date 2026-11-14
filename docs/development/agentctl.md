# Agentctl domains and runtime providers

Start at `cmd/agentctl/main.go` for flags and environment grants, `wiring.go` for
runtime selection, and `daemon.go` for HTTP lifecycle and restoration supervision.
The worker domains remain under `internal/agentctl`; concrete implementations
live under `internal/adapters`.

## Package ownership

Paths are relative to the repository root.

| Package | Owns |
| --- | --- |
| `internal/agentctl/config/model` | Resolved configuration, domain validation errors, validation and cloning |
| `internal/agentctl/interaction` | Commands, approvals, questions, updates, durable events, command markers |
| `internal/agentctl/run` | Controller, private conversation engine/reducer, queues, journals, checkpoints and restoration policy |
| `internal/agentctl/run/model` | Public execution states and run status |
| `internal/agentctl/runtime` | Normalized host/session, provisioning, protocol and error contracts |
| `internal/adapters/runtime/runtimehost` | Subprocesses, stdio, task leases, provisioning, native manifests and replay evidence |
| `internal/adapters/runtime/acp` | ACP SDK communication and normalized callbacks |
| `internal/adapters/runtime/profiles` | Pure OpenCode, Claude Code, Codex, Copilot and Kiro native configuration |
| `internal/adapters/gitprovider` | Repository identity, URL-scoped Git authentication and provider CLI preparation |
| `internal/adapters/http/agentctl` | REST/WebSocket API, explicit wire mapping and embedded OpenAPI |
| `internal/contracts/agentctl/v1` | Shared transport values without worker policy or durable reducers |

Server domains own separate execution models. Neither server nor worker domains
import the other or concrete adapters. HTTP adapters translate transport values;
SDK types stay inside adapters. Configuration models and `interaction` do not depend on run
execution. Concrete runtime profiles and ACP never import the run controller or
server services. [Architecture checks](testing.md#architecture-checks) enforce
these directions.

## Run and conversation ownership

| `run.RunController` operation | Effect |
| --- | --- |
| Configure | Bind settings without starting work; identical resubmissions are idempotent |
| `Launch` | Create a native session |
| `RestoreSession` | Restore a terminated runtime from its saved session |
| `RecoverFromCheckpoint` | Recover the bound run after daemon restart, subject to credential and journal checks |

The command lifecycle schedules supervision; the controller decides eligibility.
Keep the private conversation engine with `run`: admission, shutdown, journal
commits and recovery share lifecycle and locks.

- `conversation.state` is durable reducer state. Prompt contexts, permission/question
  channels, workers and observed progress are transient. Live commits/replay use
  the same reducer.
- Commands retain their original prompt hash and typed state. Continuation
  instructions never replace accepted identity. Prompts return typed stop reasons.
- Lock order is `turnMu` → `eventMu` → `mu`. Helpers named `UnderEventLock` and
  `UnderTurnAndEventLocks` require their named locks to be held by the caller.
- Journal/runtime I/O excludes the state lock, keeping status/shutdown responsive.
  Cancellation holds `turnMu` through delivery to protect later queued commands.
  Replay/live handoff preserves gap-free ordering.
- Checkpoint/reconciliation failures close partial startup resources. Shutdown
  fences publication under the controller lock and closes late unpublished sessions.
- Only journaled infrastructure interruptions permit eligible automatic recovery.
  Ordinary failure, cancellation and terminal runs never automatically retry.
- New checkpoints omit structured Git credentials. A recovered binding marked
  `gitCredentialsRequired` waits for matching configuration with the missing list
  replenished; it cannot restore using an absent credential set. This worker gate
  does not establish automatic credential reinjection by platform reconnect.

## Host, profiles, and protocol

`runtimehost.Host` receives a `runtime.RuntimeProfile`, protocol factory and
explicitly granted environment values through constructors. Startup:

1. Validate configuration/paths, acquire exclusive task ownership, and load or
   reserve the durable native session.
2. Apply provisioning, probe the pinned executable and start the child/stdio.
3. Initialize ACP v1 and verify mandatory session-loading support.
4. Explicitly create/load the session, apply the native mode, and confirm the
   configured model through the required `ProtocolModelSelector` capability.
5. Persist a v3 manifest (v4 for new MCP-question sessions) only after successful initialization. On
   failure, close partial resources before releasing process ownership.

The host reads v1/v2/v3/v4 manifests with their compatibility rules. Native manifests
and v1 worker checkpoints are different formats; see [session storage](sessions.md#reservation-and-compatibility).

- `runtimehost.Session` owns the child, connection, callbacks and task lease.
  Children inherit leases to prevent duplicates after parent loss. Shutdown joins
  concurrent callers and confirms termination before releasing ownership.
- Manifests/checkpoints use `internal/platform/durablefile` atomic writes with file
  and directory sync, retaining storage identities and paths.
- Pure profiles receive resolved settings and paths without organization,
  workspace, task, run, process or storage state. They perform no operational I/O.
- The host rejects path escapes, immutable-file conflicts and reserved environment
  overrides; creates private files/directories; and merges granted variables.
  `ProvisioningPaths.ConfigSubdir` is relative to its absolute runtime root.
- ACP owns stream communication, without process/filesystem/persistence operations.
  Callbacks carry `interaction.Update`, `interaction.Permission`, and
  `interaction.Question`. Create/load
  receives the working directory explicitly.
- Native OpenCode configuration is rendered by
  `internal/adapters/runtime/profiles/opencode/profile.go`. Other profile packages
  render their own model, permission, instruction, skill and MCP configuration.

Structured questions share one journal/reducer and UI. OpenCode/Codex/Claude use
ACP form elicitation; new Copilot/Kiro sessions use an authenticated loopback MCP
endpoint owned by the runtime host. Legacy sessions retain original provisioning.
See [question ownership, compatibility, and qualification](user-questions.md).

## Replay evidence

Checkpoint read/validation failures fence configuration and launch even before a
run can be configured. `/health` remains process liveness; `/info` reports blocked
recovery, interrupted state, and unavailable execution. Commands return the
sanitized `recovery_blocked` error while the checkpoint fence is held. The daemon
retries the same checkpoint once per second without repeating failure logs. Repair
the saved checkpoint/storage and let it reconcile; do not delete state to manufacture
successful recovery. Missing required Git credentials still permit matching
configuration reinjection. Failed native restore remains blocked for reconciliation.

Lifecycle transitions are validated before append and during replay using the same
durable state model. Historical single-turn journals may omit `PromptStarted`.
Unknown command completion, duplicate acceptance, and ending a run with queued work
fail visibly. Workflow stage, queue state, runtime availability, and cleanup state
remain separate dimensions. A runtime interruption leaves unresolved commands for
reconciliation; cancellation/ordinary failure pauses the queue, and approval waits
retain execution ownership.

Runtime shutdown closes owned stdio, waits for exit, then kills a child still alive
after two seconds and joins it before releasing ownership. Tests cover a blocked
child, concurrent close, and an open pipe. Kernel process waits and local filesystem
I/O do not offer a universal wall-clock deadline; ownership is never released solely
because a shutdown deadline elapsed.

- `interaction` owns command markers; run/host own reconciliation and evidence.
- `LoadSession` collects normalized observations, sealed after load and preceding
  callbacks complete. Replay is never live output.
- Missing, incomplete, ambiguous or unsupported history and tool effects pause
  recovery. Equal prompt text without command identity is insufficient.
- Absence of tool activity supports interrupted-turn continuation only with
  `ReplayCoverageVerified` attesting complete history. OpenCode 2.1.4 has no such
  attestation; successful ordinary follow-up is a separate capability.
- Recovery never silently creates a replacement session or answers approvals.

## Transport, model selection, and verification

One HTTP server shares controller state, timestamp, router, logging and contract
across REST and receive-only WebSocket activity. Server cancellation closes
streams independently of accepted execution. Wire/domain mapping lives in the
HTTP adapter; JSON/HTTP helpers remain in its `protocol` package.

Every runtime profile supplies a nonempty model identity. ACP
`session/set_config_option` must confirm it before prompts on create and restore.
OpenCode uses its provider/model identity; the other profiles use their configured
native model IDs. Unsupported or unconfirmed selection fails visibly while
preserving saved state. Real support must be qualified per provider.

| Tests | Location |
| --- | --- |
| Command encoding | `internal/agentctl/interaction` |
| Run, queue, journal and recovery policy | `internal/agentctl/run` |
| ACP translation/model confirmation | `internal/adapters/runtime/acp` |
| Host, leases, manifests and preservation | `internal/adapters/runtime/runtimehost` |
| Pure provisioning | `internal/adapters/runtime/profiles` |
| Combined provider wiring/conformance | `internal/agentctl/runtime` |
| HTTP and client mapping/compatibility | `internal/adapters/http/agentctl`, `internal/adapters/runnerclient/agentctlhttp` |
| Real OpenCode handshake and next-message continuation | `tests/integration` |

Fixtures share `opencodetest.NewHost`. Deterministic v1-manifest tests and real
current/v1-manifest continuation tests establish different evidence. See the
[test commands and prerequisites](testing.md)
and [runtime setup](../reference/runtimes/README.md).
