# Server and agentctl improvement plan

Status: implementation completed, 2026-11-04; final verification evidence below.

## Objective and boundaries

Make lifecycle ownership explicit, bound startup and recovery work, and reduce
orchestration complexity without weakening durable execution guarantees.
Deliver each phase as a focused, independently reviewable change.

Follow [requirements](../requirements.md), [architecture](../architecture.md),
[session compatibility](sessions.md), and [delivery ownership](workflow-delivery.md).
Repository instructions take precedence over unresolved product-policy differences
listed in [status](../status.md). This plan does not resolve or change those policies.

Preserve these invariants throughout:

- Platform shutdown disconnects supervision; active agentctl workers continue.
- Cancellation, disconnection, heartbeat expiry, and delivery-lease expiry do not
  prove termination or authorize duplicate execution.
- Persist terminal intent before cleanup; release ownership only after termination
  is established. Preserve stable run, session, command, and approval identities.
- Continued sessions retain compatible runtime/model configuration. Refreshing
  credentials must not silently replace pinned noncredential settings.
- Keep event ordering, durable acceptance, claim fencing, and short transactions.
- Keep server/worker domain boundaries and explicit constructor injection.
- Do not introduce a broker, service registry, generic plugin framework, or shared
  state-machine framework. Team-member capacity is a separate product workstream.

## Sequence

| Phase | Deliverable | Dependency |
| --- | --- | --- |
| 1 | Joinable platform supervision and cleanup | None |
| 2 | Bounded connection readiness and one supervisor per run | Phase 1 |
| 3 | Simpler constructors, dependencies, and session preparation | Phase 2 |
| 4 | Visible recovery failures and explicit transition invariants | Phase 3 |
| 5 | Bounded recovery and independent workflow delivery | Phases 1, 2, 4 |
| 6 | Operational diagnostics, compatibility evidence, final review | All applicable phases |

Phases 1–4 address correctness and maintainability first. Phase 5 workflow
parallelism is conditional on measured blocking/throughput; bounded startup
reconciliation remains required. Avoid combining concurrency changes with broad
structural edits in the same review.

## Phase 1 — Own and join platform background work

Primary files: `cmd/server/wiring.go`,
`internal/server/orchestration/connection_service.go`, and `launch_completion.go`.

- Create a platform-owned child context at the start of `runServer` and cancel it
  on every exit, including startup failures after recovery has begun.
- Give connection supervision and terminal cleanup explicit stop/wait ownership.
  Track reconnect, monitor, reconciliation, and retry goroutines, including nested
  launches. Share a small lifecycle helper only if both owners need identical rules.
- Serialize admission and shutdown so no positive WaitGroup admission can occur
  after shutdown begins. Make shutdown safe for repeated/concurrent callers.
- Order teardown: stop admission and cancel background work, drain HTTP handlers
  and join service work, then close executor clients and the database. Keep existing
  bounded terminal cleanup/persistence semantics; cancellation alone must not
  terminate healthy workers.
- Inspect startup error paths before and after router creation. Ensure HTTP drain
  and background cancellation order does not prevent durable reconciliation.

Acceptance and verification:

- Inject router/auth initialization failure after reconnect starts; no background
  repository or executor calls occur after dependencies close.
- Exercise concurrent Start/Close and reconnect/Close under the race detector.
- Block cleanup until explicitly released and verify dependency closure waits.
- Cancel platform ownership and verify no termination call targets active workers.
- Verify cleanup of already-terminal runs can finish within its existing bounded
  cancellation-independent operations.

## Phase 2 — Bound readiness and consolidate per-run supervision

Primary files: `connection_service.go`,
`internal/adapters/runnerclient/agentctlhttp/http_agentctl_client.go`, and client
contracts/tests affected by changes to connection establishment.

- Separate initial dial/readiness timeout from the lifetime of an established
  stream. A ready connection must survive expiration of the startup deadline.
- Allow the startup caller to stop waiting using its context without accidentally
  making that context own an accepted worker or long-lived stream.
- Define timeout handling explicitly: cancel/join a failed initial connection
  attempt, retain durable launch ownership, and use existing failure reconciliation.
- Reserve one supervision record per run. Start its stream and status monitor
  exactly once. Remove only the matching record when that supervisor exits.
- Replace boolean mode arguments with named internal modes if multiple startup
  policies remain necessary. Keep retry behavior explicit.

Acceptance and verification:

- A peer that accepts WebSocket upgrade but never sends `ready` fails within the
  configured readiness deadline and leaves no connection goroutine behind.
- A stream that becomes ready remains connected after that deadline.
- Concurrent/repeated starts create one stream and one monitor for the run.
- Cancellation during handshake and reconnect preserves identity and reservation.
- Old supervisor completion cannot remove a newly installed supervisor record.

## Phase 3 — Simplify composition and dispatch preparation

Primary files: `agent_launcher.go`, `launch_service.go`, `task_dispatcher.go`,
`direct_chat.go`, `cmd/server/wiring.go`, and `internal/adapters/http/server/router.go`.

- Remove `ExecutorAgentLauncher`; let `LaunchService` consume `AgentctlStarter`
  directly. Retain the separate process-inspection/termination capability.
- Replace long positional router/dispatcher constructors with named dependency
  structs. Validate required dependencies and optional capability combinations.
- Inject dispatcher session persistence through a narrow consumer-owned interface;
  remove access through `LaunchService.repo`.
- Parse server environment configuration once. Reuse runtime-image mappings;
  preserve existing defaults and document any intentional validation changes.
- Move OIDC bootstrap into explicit composition, or pass startup context into router
  initialization so shutdown cancels discovery immediately.
- Extract cohesive helpers for fresh-session configuration, continued-session
  credential refresh, repository resolution, and prompt preparation. Keep atomic
  reservation immediately before external launch and preserve failure semantics.
- Resolve Orkest MCP configuration once after run identity is final. Replace the
  anonymous capability assertion with an explicit run-scoped contract; retain an
  adapter only if an actual compatibility consumer needs the older form.
- Replace nested MCP credential matching with validated ID lookup. Preserve saved
  URLs and settings, clone mutable headers, and fail visibly on missing identities.
- Use functional options only for optional policies with defaults. Do not convert
  required dependencies into options or add options solely to demonstrate a pattern.

Acceptance and verification:

- Existing launch, direct-chat, authorization, artifact binding, and session tests
  preserve observable behavior.
- Fresh sessions use current definitions; continued sessions retain pinned settings
  and refresh only permitted values. Changed environment values either preserve
  compatibility or fail visibly with saved state intact.
- Run-scoped tools receive the final producing-run identity exactly once.
- Missing MCP identities fail before launch without partially modifying configuration.
- Architecture checks pass without new exceptions. No new persistence or wire
  format is required by these structural changes.

## Phase 4 — Expose recovery failures and enforce state invariants

Primary files: `internal/agentctl/run/controller.go`, `conversation_state.go`,
`conversation_recovery.go`, `cmd/agentctl/daemon.go`, and server launch-status planners.

- Represent failed checkpoint recovery even when configuration could not load.
  Expose a sanitized blocked reason and prevent conflicting launch/configuration
  from bypassing unresolved saved state. Keep liveness distinct from readiness.
- Define an explicit recovery exit path: a transient read failure may be retried;
  corrupt/incompatible state remains preserved and requires reconciliation. Never
  silently clear it or create a replacement conversation.
- Inventory legal combinations of durable conversation flags and transient runtime
  states. Keep these dimensions separate; document invariants before changing types.
- Keep the shared live/replay reducer. Validate commands before append and validate
  replay consistency without rejecting supported historical journals.
- Centralize duplicated server launch-transition decisions in pure planners;
  inventory existing persistence guards and use expected-state updates where needed
  to prevent concurrent terminal-status regression.
- Inspect blocked provider shutdown and inherited pipes. Preserve ownership until
  termination is confirmed; document limits instead of claiming a universal deadline.

Acceptance and verification:

- Corrupt/unreadable checkpoints and incompatible manifests produce visible blocked
  recovery, preserve files, and cannot admit replacement execution.
- Cancellation never enables automatic restore. Approval waits retain ownership.
- Current and historical sessions restore and accept a successful next message.
- Live commits and replay yield equivalent durable state for supported sequences.
- Concurrent terminal events and cleanup cannot regress final launch status.
- Blocked provider/pipe fixtures establish actual shutdown behavior and diagnostics.

Before any SQL/persistence change, read the `database-change` skill. If needed,
version migrations, regenerate sqlc, and add real PostgreSQL concurrency coverage.
If status fields change, update the agentctl OpenAPI contract and wire mappings;
update the platform contract/generated client if those fields become platform-facing.

## Phase 5 — Bound reconciliation and isolate workflow throughput

Primary files: connection/completion services, workflow automation `worker.go` and
`execution_service.go`, and their PostgreSQL claim adapters.

- Bound concurrent startup reconciliation and cleanup attempts. Do not hold a pool
  slot indefinitely for a persistent stream or a retry loop; schedule bounded
  attempts and track deferred retries under lifecycle ownership.
- Preserve per-run ordered activity processing and durable cursor advancement.
  Do not parallelize event persistence within a run.
- Measure how a stalled command delivery affects unrelated workflow events. If
  material, run event processing independently from a bounded delivery worker pool.
- Claim a command only when a worker can process it. Keep external calls outside
  transactions; preserve current claim tokens, acceptance checks, and fenced acks.
- Keep reclaims reconciliation-only. Worker count and lease expiry grant no new
  permission to start, retry, release capacity, or replace a runtime.
- Add bounded configurable worker counts only where operationally useful; document
  defaults and limits. Leave the existing image-build pool intact unless evidence
  identifies a specific defect.

Acceptance and verification:

- A large recovery backlog respects concurrency bounds and cancels cleanly.
- One unreachable worker does not starve unrelated reconciliation indefinitely.
- If workflow pooling is implemented, slow delivery does not block event processing.
- Real PostgreSQL tests with multiple workers establish fencing, stale-ack rejection,
  and absence of duplicate reservation/launch after lease expiry.
- Cancellation after reservation/before acknowledgement reconciles the same command
  and run. No transaction or connection is held during external delivery.

## Phase 6 — Diagnostics, documentation, and release evidence

- Extend existing Chi middleware and HTTP instrumentation rather than introducing
  another interceptor abstraction. Audit request/run correlation and preservation of
  WebSocket response-writer capabilities before changing logging wrappers.
- Replace swallowed operational failures with sanitized reason codes and relevant
  task/run/command identity. Do not log credentials, prompts, or arbitrary provider
  error bodies. Avoid repeated reconnect-error log floods.
- Add metrics/tracing only against an established sink/requirement. Useful measures
  include readiness duration, reconnect attempts, active supervisors, queue wait,
  cleanup backlog, and recovery-blocked counts; avoid unbounded metric labels.
- Record accepted architecture decisions in `docs/architecture.md`, configuration
  in `.env.example` and operations docs, and recovery mechanics in session/agentctl
  docs. Keep this task document updated with completed work, evidence, and blockers.
- Preserve runtime pins, manifests, journal formats, and wire contracts unless a
  phase explicitly requires a compatible change. Review mixed-version behavior;
  retain the upgrade restrictions in the delivery ownership document.

## Verification gates

For each Go change, run `make fmt` as needed and `make lint`; run focused observable
behavior tests for the affected package. Use finite test timeouts and synchronization
barriers for concurrency tests rather than timing-dependent sleeps.

After lifecycle/concurrency changes:

```sh
go test -race ./cmd/server ./cmd/agentctl \
  ./internal/agentctl/run ./internal/server/orchestration \
  ./internal/server/workflowautomation ./internal/server/executionprofile \
  ./internal/platform/httpserver \
  ./internal/adapters/http/agentctl \
  ./internal/adapters/runnerclient/agentctlhttp -timeout=3m
make test-recovery-unit
```

For changed runtime/session behavior, run `make test-runtime-conformance` and
`make test-recovery` with the pinned executable and explicit model configuration.
For delivery/persistence concurrency, run `make test-postgres` and the documented
PostgreSQL race suite against an isolated database. Run `make test-docker` for
process independence/restart effects and real provider qualification only for
affected journeys with the required credentials and endpoints.

Finish with `make check` and a focused diff review. Read
[testing prerequisites](testing.md) before infrastructure checks. Record unavailable
checks as blocked/unverified; fixtures and session loading do not prove live
continuation. Do not repeat broad checks without a new change or unresolved concern.

## Completion checklist

- [x] Platform startup failure and shutdown join all owned background work.
- [x] Initial readiness is bounded; established streams retain independent lifetimes.
- [x] Each run has one supervisor and recovery concurrency is bounded.
- [x] Redundant launcher and hidden dispatcher repository access are removed.
- [x] Constructors and session preparation are explicit and readable.
- [x] Recovery failures are visible and preserve execution/session ownership.
- [x] Historical restore plus follow-up remains compatible.
- [x] Workflow parallelism is either justified and verified or explicitly deferred.
- [x] Contracts, generated outputs, operations docs, and architecture agree.
- [x] Verification evidence and remaining infrastructure/live-provider limits are recorded.

## Progress and evidence

Implementation is based on `1cef7464cb8374e83dda46cb14a408be62db67bb`
(`refactor/go-architecture`), on branch `refactor/lifecycle-and-orchestration`.
Implementation commits: `3b3f75b` (server), `4a6c12e` (worker), and `c8a4816`
(workflow/persistence). Verification covers their combined code tree.
The preceding read-only review is historical evidence, separate from this work.

| Phase | Implemented result |
| --- | --- |
| 1 | Platform child context, gated background admission, joined reconnect/monitor/cleanup work, dependency teardown ordering |
| 2 | One supervisor per run, caller-cancellable initial readiness, independent established streams, bounded handshakes, real stalled-WebSocket test |
| 3 | Direct executor start contract, named dependencies, direct session-store injection, configuration snapshot/image reuse, contextual OIDC bootstrap, shared capability resolution, atomic MCP header refresh and explicit producing-run tools |
| 4 | Checkpoint failure fence and repair/retry, joined checkpoint recovery, live/replay transition validation including legacy single-turn journals, monotonic terminal SQL guards, blocked-child shutdown coverage |
| 5 | Four bounded reconciliation/cleanup attempts and handshakes; independent event polling and four delivery workers; unchanged claim/reservation/ack fencing |
| 6 | Sanitized deduplicated disconnect/cleanup diagnostics, worker API semantics documented, architecture and upgrade guidance updated |

The blocking-delivery test holds every delivery worker and observes another event
polling cycle, establishing the reason for separating event processing from delivery.
No metrics/tracing sink is configured, so no new telemetry backend is introduced.
Existing HTTP logging wrappers retain hijacking/flush behavior and real WebSocket
coverage. No new environment keys or defaults are introduced. The readiness timeout
is an optional constructor policy (ten seconds by default); worker bounds are four.

Verification on 2026-11-04:

- `make check`: lint, architecture checks, Go tests, frontend generated contract,
  lint/tests/types, and builds. An intermediate empty-critical-section lint finding
  was corrected; only the final successful run qualifies the final tree.
- Focused final lifecycle/concurrency suite with `-race -count=1 -timeout=3m`:
  both commands, worker run, orchestration, workflow automation, execution profiles,
  HTTP lifecycle, worker HTTP API and runner client.
- `make test-runtime-conformance`: deterministic contracts for all five profiles,
  architecture checks and race detection.
- `make test-recovery`: deterministic recovery plus real pinned OpenCode 2.1.4
  handshake/current-manifest/v1-manifest restore and successful next response,
  using explicit `opencode/beam-free`, with no granted provider credentials.
- `make test-postgres` and the real PostgreSQL repository/migration suite with race
  detection, including new
  concurrent pending/final transition fencing and existing multi-worker delivery
  tests, passed in isolated database `orkest_architecture_20261104` on the local
  PostgreSQL 18.3 container. This database is removed after verification.
- `make test-docker` built all five pinned images and passed real OpenCode launch,
  agentctl restart, container replacement, session identity, fresh-session and
  retained-file checks. Following the final checkpoint shutdown lock-scope fix,
  the OpenCode image was rebuilt and the same Docker smoke/restore tests passed
  against the final code. These tests make no model prompts.
- `make sqlc` regenerated query bindings; `git diff --check` passed. No schema migration, runtime pin, checkpoint version, manifest version,
  journal encoding, or worker wire field changes are introduced. The worker OpenAPI
  change documents the existing recovery flag's behavior.

Qualification limits:

- Live Claude Code/Codex/Copilot/Kiro model calls, provider approvals/MCP, deployed
  platform restart during active work/approval, and uncertain external tool effects
  remain unqualified. Fixture success does not establish those journeys.
- Checkpoint repair and follow-up are tested deterministically through the worker
  controller; real current/v1 continuation uses the pinned runtime host adapter.
- Shutdown joins owned work and preserves ownership; kernel waits/local filesystem
  stalls do not have a universal wall-clock deadline.
- Team-member capacity and worker transport authentication remain separate unfinished
  work. Query guards require stopping old platform instances before upgrade; see
  [delivery ownership](workflow-delivery.md). No product-policy conflicts are resolved.
