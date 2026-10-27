# Runtime contract simplification

Implemented: mandatory typed runtime/session contracts; managed v1 storage,
ownership and subprocess lifetime; one durable conversation reducer for live
commits and replay; explicit transport/orchestration dependencies; reusable
provider conformance tests, pinned version inventory, and CI qualification.
Go composition replaces the duplicated behavior an abstract base class would
otherwise provide. Provider-specific code retains only native protocol/configuration
responsibilities. See [runtime providers](runtime-providers.md).

Scope retained: OpenCode only; existing REST/WebSocket payloads, v1 manifests,
hashes, paths, journals, checkpoint format, and legacy conversation behavior.
No database migration or frontend change. No additional runtime is qualified.

Verification on 2026-10-27:

- `make check`: passed (lint, Go tests, frontend checks/tests, builds).
- `make test-runtime-conformance`: passed, including race detection, safe and
  uncertain reconciliation, shared configuration probes, parent-exit ownership,
  restart memory in the deterministic fixture, and four CI inventory gate tests.
- Real pinned OpenCode handshake and restoration integration: passed without
  model calls (`TestOpenCodeHandshake`).
- `make qualify-runtime`: correctly failed for absent live test setup; this is
  prerequisite enforcement, not successful provider qualification. Real model
  memory, approvals, and MCP behavior remain unverified.
- Docker smoke/restore target: failed intermittently during native restoration.
  A diagnostic rerun passed without a production change; repeated restoration
  also failed against an image built from unchanged HEAD. This pre-existing
  OpenCode/Colima integration instability remains unresolved and is not counted
  as a passed check. Temporary diagnostic test changes were removed.
- `git diff --check`: passed.

CI workflow and strict qualification target are implemented. Making **Runtime
conformance** required for merging is blocked: GitHub branch-protection API
returns HTTP 403 for the private repository and requires GitHub Pro or public
visibility, despite the current account having repository admin permission.
The repository's visibility and subscription were not changed. Configure the
protected runtime-qualification environment and explicitly selected live model,
credentials, harmless approval/recovery prompts, and MCP test endpoint before
qualifying new runtime versions. The exact existing OpenCode v2.1.4 retains its
previous availability and documented verification gaps.

## Communication-only runtime refactor

Implemented according to [agentctl-refactor.md](../agentctl-refactor.md):

- `ManagedRuntime` is the agentctl-owned runtime host and now owns provisioning
  application, version checks, process and stdio lifetime, ACP negotiation,
  explicit new/load session behavior, identity checks, and partial-failure
  cleanup. Profiles and communication factories are injected at executable
  wiring; concrete runtime/ACP imports do not enter agentctl host code.
- OpenCode native configuration is a pure provisioning profile. Runtime
  creation no longer happens in OpenCode code; the SDK client is shared in
  `internal/agentctl/runtime/acp` and receives normalized callbacks.
- Agentctl owns prompt marker encoding, replay collection, and recovery policy.
  OpenCode 2.1.4 has no verified complete history coverage, so the host preserves
  its session but pauses when recovery cannot prove a safe continuation.
- Conversation status, controller checkpoint storage, and daemon restoration
  eligibility use explicit service methods. Automatic recovery is limited to
  recorded infrastructure interruption; ordinary failures, user cancellation,
  and terminal runs do not auto-retry.
- Added a dependency guard for profile and communication packages, v1 checkpoint
  and command marker fixtures, provisioning escape/conflict tests, and the
  missing scope-ID info snapshot mapping test.

Verification after the refactor:

- `make fmt`, `make check`, `git diff --check`: passed.
- `make test-runtime-conformance`: passed with race detection and dependency
  checks.
- Real OpenCode 2.1.4 `TestOpenCodeHandshake`: passed against the installed
  pinned executable without model calls.
- `make test-docker`: passed when `DOCKER_HOST` selected the active Colima socket;
  the default `/var/run/docker.sock` was absent.
- Live model continuation, real approval round trips, and MCP live behavior
  remain unverified because explicit model credentials and operator test setup
  are not configured. No new runtime or version is qualified.

## Code review improvements

The follow-up review fixes replay collection to start before `LoadSession` and
seals evidence when its callbacks have drained. Collection and reconciliation
share a lock; later live activity cannot mutate restored evidence. A fixture
profile attests coverage only in tests; production OpenCode coverage remains
unqualified.

`RunConfig` embeds the common `RuntimeConfig` definition and adds executor inputs.
JSON fields remain flat, and v1 manifests and checkpoint fixtures remain
compatible. Configuration is deep-copied at ownership boundaries. Runtime
validation is shared and reports reasons without rejected values.

Command queries and queue transitions now belong to `ConversationService`.
Controllers delegate through named resume/discard operations; REST routes and
response shapes are preserved.

Verification on 2026-10-27:

- `make fmt` and `make check`: passed, including zero lint findings, Go tests,
  frontend checks/tests, and builds. Go cache access required execution outside
  the sandbox; the initial sandboxed lint/package load was blocked.
- `make test-runtime-conformance`: passed with race detection, including replay
  collection through the ACP fixture and concurrent observations after sealing.
- Real pinned OpenCode `TestOpenCodeHandshake`: passed, including session loading
  without model calls.
- Regression tests cover configuration isolation, flat JSON compatibility,
  existing v1 checkpoints, and validation reasons without sensitive values.
- `git diff --check`: passed.

The existing live model, approval, and MCP qualification limits still apply.


## Readability and responsibility organization

Implementation follows the readability review: runtime startup, sessions,
callbacks, provisioning, and recovery evidence have focused files in the same
agentctl package. Conversation commands, permissions, queues, recovery, events,
and subscriptions retain one service owner. Runtime session shutdown and task
ownership release now have one implementation instead of an interface wrapper.

Conversation event operations own their state-lock acquisitions. Callers retain
the event-ordering lock through snapshot, persistence, reducer application, and
publication; filesystem and runtime I/O occur outside the state lock. Startup
steps have explicit preparation, process creation, protocol initialization, and
manifest persistence phases with coordinated partial-failure cleanup.

Names distinguish runtime sessions, execution state, command IDs, task-session
mode, relative configuration directories, and verified replay coverage. One
explicit run-controller constructor replaces the dependency-free constructor;
logger and expected-run arguments are explicit. Redundant interface fragments,
forwarding methods, and the no-op communication close operation are removed.
The controller still owns startup and restoration policy. Wire names, v1 state,
configuration hashes, and permission behavior are preserved.

Verification on 2026-10-27:

- `make fmt` and `make check`: passed, including zero lint findings, Go tests,
  frontend checks/tests, and builds.
- `make test-runtime-conformance`: passed with race detection, including
  blocked commits/startup/reconciliation, cancellation, approval correlation,
  event ordering, replay sealing, and ownership tests.
- Real pinned OpenCode `TestOpenCodeHandshake`: passed, including session loading
  without model calls.
- `DOCKER_HOST=unix:///Users/josemiguelmelo/.colima/default/docker.sock make test-docker`:
  passed both image builds and Docker launch/session-restore tests.
- `git diff --check`: passed.

Live model continuation, approvals, and MCP qualification still require explicit
credentials and operator test setup. No additional runtime is qualified.


## Runtime host package extraction

Plan: move the concrete runtime host, sessions, subprocess lifecycle,
provisioning, native session storage, and replay evidence into
`internal/agentctl/runtimehost`; update executable/test wiring and enforce the
new dependency boundary; verify storage compatibility and runtime lifecycle.

Implemented `runtimehost.New`, `Host`, `Session`, and `Environment`. Supporting
process, provisioning, storage, and evidence types remain private. Configuration,
profile and communication contracts, error types, and command encoding remain in
`agentctl`, alongside the controller and conversation service. Executable wiring
selects the host, ACP client, and OpenCode profile without application-to-host
imports. Moved implementation tests and the v1 session fixture beside the host.

The run checkpoint and native session manifest now share
`internal/core/durablefile.Write`. The writer's atomic replacement and file and
directory sync semantics are unchanged. Architecture checks reject concrete
host imports in the application/profile/communication packages and reject
concrete runtime or platform dependencies in the host. Checks now normalize full
module import paths before matching package boundaries.

Verification on 2026-10-27:

- `make fmt` and `make check`: passed, including zero lint findings, Go tests,
  frontend checks/tests, and builds.
- `make test-runtime-conformance`: passed with race detection across the
  application and extracted host, plus nine script tests and dependency guards.
- The moved v1 session fixture, configuration fingerprint/path checks,
  provisioning protections, ownership-after-parent-exit checks, and replay
  evidence tests passed.
- Real pinned OpenCode `TestOpenCodeHandshake`: passed, including session loading
  without model calls.
- `DOCKER_HOST=unix:///Users/josemiguelmelo/.colima/default/docker.sock make test-docker`:
  passed both image builds and Docker launch/session-restore tests.
- `git diff --check`: passed. No module, API schema, or database changes.

Live model continuation, approvals, and MCP qualification retain their existing
credential and operator-setup prerequisites. No new runtime is qualified.

## Follow-up readability implementation

Implemented the review's naming, grouping, and state-ownership changes. The
conversation and checkpoint store are private, durable state has a named field,
command hash/state records share one map, and the queue uses `Command` directly.
Protocol contracts use `ProtocolClient`, `ProtocolClientFactory`,
`ProtocolCallbacks`, and `PromptObserver`. Controller methods distinguish
`RestoreSession`, `RecoverFromCheckpoint`, and command status lookup; journal
reconciliation remains conversation-owned. Startup uses a named mode instead of
a boolean. Status projection and interruption classification have named helpers.

Grouped command encoding, permission types, runtime configuration, and REST
lifecycle handlers. Native naming belongs to OpenCode. Runtime callbacks own
observer attachment, task leases pass explicitly to process creation, and pure
environment validation is separate from provisioning I/O. Moved host and combined
wiring tests to their relevant packages, added focused ACP callback coverage, and
split conformance into named sequential phases. Added regression coverage for
original-command deduplication during a restored continuation.

Compatibility scope remains unchanged: JSON names, routes, event values,
checkpoint/manifests, configuration fingerprints, and permission defaults.
Verification on 2026-10-27:

- `make fmt` and `make check`: passed, including zero lint findings, Go tests,
  frontend checks/tests, and builds. The frontend build retains its existing
  dependency directive and chunk-size warnings.
- `make test-runtime-conformance`: passed with race detection, all nine script
  tests, dependency guards, moved host/wiring tests, ACP callback tests, and the
  continuation/deduplication regression.
- Real pinned OpenCode `TestOpenCodeHandshake`: passed, including session loading
  without model calls.
- `DOCKER_HOST=unix:///Users/josemiguelmelo/.colima/default/docker.sock make test-docker`:
  passed both image builds and Docker launch/session-restore tests.
- `git diff --check`: passed. Review against the pre-change working-tree snapshot
  confirmed the production string-literal set is unchanged, including wire names
  and sanitized error messages. No module, schema, or database changes.

Initial sandboxed transport tests could not bind local listeners, and lint could
not load Go packages. Both were rerun successfully with the required access.
Live model continuation, real runtime approval round trips, and live MCP behavior
retain their explicit credential and operator-setup prerequisites. No new
runtime or version is qualified.

## Simplification follow-up

Plan: consolidate HTTP server wrappers; remove unreachable shutdown branches;
return one typed stop reason; share startup-failure cleanup; replace capability
dispatch with direct required checks; group reducer decisions; remove narrow
interfaces, singleton parameter wrappers, forwarding methods, and duplicated
fixture configuration types. Verify compatibility and compare against the
working-tree snapshot taken before this follow-up.

All seven changes are implemented. One HTTP server owns routing and shared
transport state. `StopReason` replaces the result/outcome pair. Startup has one
persistence/reconciliation cleanup path, and process shutdown waits before
ownership release. Runtime profiles no longer declare a string-based capability
list. The reducer calculates command outcomes once and groups the events that
revoke recovery eligibility. Tests share a fixture host constructor. Legacy
conversation/session behavior, wire payloads, state formats, and recovery and
permission policies are retained.

Verification on 2026-10-27:

- `make fmt` and `make check`: passed, including zero lint findings, Go tests,
  frontend checks/tests, and builds. Existing frontend dependency directive and
  chunk-size warnings remain.
- Focused tests and `make test-runtime-conformance`: passed, including race
  detection, all nine script tests, startup checkpoint/replay failure cleanup,
  shutdown during startup, cancellation, and late activity/permission preservation
  of restoration eligibility.
- Real pinned OpenCode `TestOpenCodeHandshake`: passed, including session loading
  without model calls.
- `make test-docker` with the active Colima socket: both image builds and smoke
  launch passed; session restoration failed at container replacement with HTTP
  502 (`session_integration_test.go:117`). A diagnostic run passed. An image built
  from the saved pre-simplification working-tree code reproduced that exact
  failure in one of three bounded comparisons (the other two passed). This
  pre-existing intermittent failure remains unresolved; the target is not
  reported as passed. Temporary diagnostic test changes were removed.
- `git diff --check` and scoped diff review: passed. The OpenAPI contract and v1
  compatibility fixtures are unchanged. No module, schema, or database changes.
- Compared with the snapshot at the start of this follow-up, agentctl and its
  command have 4,289 production Go lines instead of 4,425 (-136), and 3,324 test
  lines instead of 3,287 (+37). Total Go decreases by 99 lines, including added
  regression coverage. These counts include comments, whitespace, and the shared
  fixture helper; they do not compare against the older committed HEAD.

Sandbox Go-cache access required rerunning the initial compilation check with
the required access. Live model continuation, real runtime approval round trips,
and live MCP behavior retain their credential and operator-setup prerequisites.
No new runtime or version is qualified.

## Domain package organization

Plan: move resolved settings into `config`, interaction values into `interaction`,
normalized host/profile/protocol contracts into `runtime`, and execution,
conversation, journaling, and checkpoint recovery into `run`. Update all consumers
without root forwarding aliases, preserve wire/storage contracts, strengthen
boundary guards, then verify and review the scoped diff.

The package moves and consumer imports are implemented. The private conversation
engine stays with the controller because they share session lifecycle and locking
invariants. Runtime implementations import contracts and values rather than the
execution service. The root now contains a package navigation README. Both v1
fixtures stay beside their owning tests with unchanged content.

Verification on 2026-10-27:

- `make fmt` and `make check`: passed, including zero lint findings, all Go tests,
  frontend checks/tests, and both binary builds. Existing frontend dependency
  directive and chunk-size warnings remain.
- `make test-runtime-conformance`: passed with race detection, all 12 script
  tests, and guards covering the moved domains and runtime implementations.
- Real pinned OpenCode `TestOpenCodeHandshake`: passed, including session loading
  without model calls.
- `DOCKER_HOST=unix:///Users/josemiguelmelo/.colima/default/docker.sock make test-docker`:
  passed both image builds and smoke/session-restoration tests on this run.
  The previously reproduced intermittent restoration failure remains documented;
  a passing run does not establish that it is fixed.
- A read-only comparison of all 353 agentctl Go functions found no signature/body
  token differences after removing the changed domain import qualifiers. There
  are no added execution services, runtime algorithms, or forwarding types.
- The OpenAPI contract, v1 checkpoint/session fixture content, module files, and
  Makefile are unchanged. Consumers outside agentctl have import/reference changes
  only. `git diff --check` and scoped diff review passed.
- Agentctl plus its command now contain 4,356 production Go lines versus 4,289
  before the move (+67), and 3,360 test lines versus 3,324 (+36). The increase is
  package imports, package documentation, and the private event-builder file
  boundary; the function count and function logic are unchanged. These counts
  include comments and whitespace and compare against this turn's snapshot.

Live model continuation, real runtime approval round trips, and live MCP behavior
retain their explicit credential and operator-setup prerequisites. No new runtime
or version is qualified.
