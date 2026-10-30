# Agentctl domains and runtime providers

Agentctl is organized by ownership under `internal/agentctl`. Start with
`cmd/agentctl/main.go` for executable wiring, then `run/controller.go` for the
lifecycle of one execution run.

| Package | Owns |
| --- | --- |
| `config` | Resolved run/runtime settings, validation, cloning, executor checkout inputs |
| `interaction` | Commands, command status, approvals, updates, durable event records, command marker encoding |
| `run` | Controller, private conversation engine/reducer, queue, journal, subscriptions, checkpoints, restoration policy |
| `runtime` | Normalized host/session contracts, prompt observers, stop reasons, provisioning profiles, protocol contracts, sanitized runtime errors |
| `runtimehost` | Subprocess and stdio ownership, provisioning application, native session manifests, task leases, replay evidence |
| `runtime/acp` | ACP SDK communication and translation into normalized values |
| `runtime/opencode` | Pure OpenCode native configuration rendering |
| `runtime/claudecode` | Claude Code ACP session options and isolated configuration paths |
| `httpapi` | One REST/WebSocket server, transport-owned controller interface, wire representations |
| `daemon` | Standalone server lifecycle and periodic restoration supervision |

There is no Go forwarding package at the root. Consumers import the domain that
owns a value or operation. Configuration and interaction values have no dependency
on run execution. Runtime contracts depend on those values; the run controller,
host, protocol client, and native profile consume the contracts. Concrete runtime
implementations never import the run service, HTTP API, or platform services.
`make check-runtime-boundaries` enforces these directions, including the newly
separated packages. There are no new service layers or registries.

## Run and conversation ownership

`run.RunController` configures a run without starting work. `Launch` creates a
native session, `RestoreSession` replaces a terminated runtime using its durable
session, and `RecoverFromCheckpoint` reloads a configured run after daemon
restart. The daemon schedules supervision; the controller decides eligibility.

The private conversation engine remains in `run` alongside its controller.
Session binding, shutdown, command admission, journal commits, and recovery share
lifecycle and locking rules. Keeping them together avoids exposing locks or
adding forwarding interfaces between services. Commands, permissions, queue
operations, replay, event commits, and subscriptions have focused files there.

`conversation.state` is the durable reducer state. Prompt contexts, pending
permission channels, workers, and observed execution progress are transient.
Live commits and journal replay apply the same reducer. One command record keeps
the original prompt hash and typed command state; continuation instructions do
not replace the accepted prompt's identity. Runtime prompts return one typed
`runtime.StopReason`, including normalized cancellation.

The lock order is `turnMu`, `eventMu`, then `mu`. Helpers named `UnderEventLock`
or `UnderTurnAndEventLocks` require those caller-held locks and own their state
lock acquisitions. Journal and runtime I/O do not hold the state lock, so status
reads and shutdown admission remain responsive. Cancellation holds `turnMu`
through delivery to prevent it from reaching a later queued command.
Replay/live subscription handoff retains event ordering without gaps.

Checkpoint reads/writes belong to the private checkpoint store in `run`.
Startup shares one cleanup path for checkpoint and reconciliation failures.
Shutdown fences session publication under the controller lock; startup closes
unpublished sessions returned after shutdown. Only a journaled infrastructure
interruption permits automatic recovery. Ordinary failures, cancellation, and
terminal runs do not auto-retry. Late activity and permission events preserve
recorded recovery eligibility. Legacy non-task conversations retain their
existing behavior.

## Host, profiles, and protocol

`runtimehost.Host` receives a `runtime.RuntimeProfile`,
`runtime.ProtocolClientFactory`, and explicitly granted environment values through
constructor injection. It validates configuration/paths, acquires exclusive task
ownership, loads or reserves the v1 native session, applies a provisioning plan,
probes the pinned executable version, starts the child and stdio, initializes ACP,
checks protocol version and mandatory loading support, creates or loads the
session explicitly, applies native mode settings, and persists the manifest.
Startup failures close partial resources before releasing ownership.

`runtimehost.Session` owns the child, protocol connection, callbacks, and task
lease. The child inherits that lease to prevent duplicate execution after an
unexpected parent exit. Concrete child shutdown joins concurrent callers and
confirms termination before the session releases its ownership handle. Session
manifests and run checkpoints share `internal/core/durablefile` for atomic writes
with file and directory sync. Existing storage identity and paths are preserved.

Profiles receive only resolved runtime settings and selected paths, excluding
organization, workspace, task, run, process, and storage state. Profiles render
plans without operational I/O. The host validates relative paths, prevents root
escapes and immutable-file conflicts, creates private files/directories, and
merges explicitly granted environment variables after reserved-name checks.
`ProvisioningPaths.ConfigSubdir` is relative to its absolute runtime root.

`runtime/acp` binds agentctl-owned streams to the ACP SDK. It performs no process,
filesystem, persistence, or platform operations. SDK types stay in that package;
callbacks carry `interaction.Update` and `interaction.Permission` values.
Session creation/loading takes its working directory directly. OpenCode's agent
Markdown, native names, model/mode selection, skills, HTTP MCP JSON, XDG paths,
and `OPENCODE_*` values belong to `runtime/opencode/profile.go`. OpenCode's native
HTTP MCP client and default permission behavior remain unchanged.

Recovery marker encoding belongs to `interaction`; reconciliation policy and
evidence belong to agentctl's run/host implementations. During `LoadSession`,
the host collects normalized replay observations and seals them when loading and
its preceding callbacks have completed. Replay is never published as live output.
Missing, incomplete, ambiguous, or unsupported history and recorded tool effects
pause recovery. Matching command identity is mandatory; equal prompt text alone
is insufficient. `ReplayCoverageVerified` must attest complete coverage before
absence of tool activity can support continuation. OpenCode 2.1.4 has no such
attestation. No recovery path silently creates a fresh session.

## Transport, compatibility, and verification

One `httpapi.Server` shares controller state, timestamp, router, logging, and API
contract across REST and receive-only WebSocket handlers. The server context
closes streams independently of accepted execution. Transport types remain in
`httpapi/protocol`; they map domain values without exposing resolved secrets.

Package moves preserve REST/WebSocket payloads, event names, command markers,
v1 checkpoints and session manifests, configuration fingerprints, ownership locks,
working paths, and legacy behavior. Go import paths changed to their owning
subpackages; repository consumers were updated directly without compatibility
aliases. The checkpoint fixture moved to `run/testdata`; its bytes are unchanged.

Tests stay beside their owners: command encoding in `interaction`, execution and
recovery in `run`, ACP translation in `runtime/acp`, host/storage behavior in
`runtimehost`, pure provisioning in `runtime/opencode` and `runtime/claudecode`, and combined runtime wiring
in `runtime/*_test.go`. Configuration ownership tests remain with `run`, where
configuration is accepted. Fixture-backed tests share `opencodetest.NewHost`.

- `make check` runs lint, tests, frontend checks, and builds.
- `make test-runtime-conformance` runs deterministic behavior with race detection
  plus dependency and qualification-script checks.
- Real pinned OpenCode handshake/session loading and `make test-docker` exercise
  the executable and container boundaries without model calls.
- Live qualification requires the pinned executable, explicit model credentials,
  approval/cancellation prompts, and an operator-supplied HTTP MCP endpoint.

Fixture success and real session loading do not prove live model continuation,
real approval round trips, or full history coverage. Docker restoration has a
known intermittent HTTP 502 at container replacement, reproduced on the code
before domain organization. Report it as a failed check whenever it occurs.
OpenCode 2.1.4 and Claude Code `claude-agent-acp` 0.99.0 are production profiles.
Claude Code model execution and interrupted-work continuation remain unqualified.
See [task sessions](task-sessions.md) for prerequisites and limitations.

`internal/core/runtime/versions.json` is the single runtime version inventory.
Go embeds it for supported versions, provider metadata, and default image tags.
Each Dockerfile reads it directly for the installed package version; Make reads
it for default image tags. Direct Docker builds use the same pinned packages.
CI reads the same file to install pinned adapters and identify changed runtimes
that need live qualification.

The runtime conformance workflow handles changed runtimes in separate matrix
jobs and installs each pinned adapter when live qualification is enabled.
OpenCode uses
`ORKEST_TEST_MODEL_PROVIDER` and `ORKEST_TEST_MODEL_ID`; Claude Code uses
`anthropic` and the variable `ORKEST_CLAUDE_CODE_TEST_MODEL_ID`
in the GitHub `runtime-qualification` environment. Both use the existing shared
approval/cancellation/recovery prompts, MCP settings, and granted credential names.
Set `ORKEST_RUN_LIVE_QUALIFICATION=true` in that environment to enable paid live
qualification. Until enabled, CI runs deterministic tests and reports that live
qualification is pending. Once enabled, missing settings fail qualification.
Locally, `make qualify-runtime` checks all runtimes;
`make qualify-runtime QUALIFY_RUNTIME=claude-code` checks Claude alone with
`ORKEST_TEST_MODEL_ID` set to its model and `ORKEST_CLAUDE_CODE_BINARY` pointing
to the pinned adapter.
