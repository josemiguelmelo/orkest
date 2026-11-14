# Task editor production review

Reviewed baseline: `e2d4e30` in PR #111. Findings below distinguish the reviewed
code from the subsequent refactor. The user's later instruction requires the
feature to always be available; the former configuration gate has been removed.

## Executive Summary

The backend has useful boundaries: consumer-owned editor interfaces, a concrete
executor factory, scoped task authorization, generation-bound access, and one
handoff guard across platform delivery paths. Go formatting, contextual I/O and
focused tests are generally sound. Nevertheless, the reviewed revision was not
production-ready: real Docker probes demonstrated cross-workspace editor access
and access to an unauthenticated worker through a Docker host alias.

The refactor closes those demonstrated network paths and fixes reproducible
ownership, revocation, cleanup and locking faults. Always-available APIs and
capabilities do not constitute production sign-off. In particular, adversarial
mount-source replacement and complete PostgreSQL/restart qualification remain
open. No current backend version should be described as fully production-qualified.

## Architectural & Pattern Feedback

- **Dependency inversion / Factory / Adapter:** keep executor selection in the
  executor factory. Server wiring consumes editor capabilities; it has no Docker
  import, type assertion or runtime/model-specific branch. No service registry or
  generic workload framework is needed.
- **Single responsibility:** separate lifecycle intent from external observations.
  An editor being absent during handoff does not erase its owner, and a failed Stop
  must retain cleanup intent. Gateway launch, exchange and proxy handlers are now
  separate methods. Network and relay concerns live in concrete Docker files.
- **KISS and least privilege:** one private network per editor generation is the
  isolation boundary. Native mode uses a small fixed-upstream authenticated relay
  because loopback publishing from an internal-only Docker network is unavailable.
  The relay is required by that concrete topology; it is not a general proxy product.
  A containerized platform attaches directly as the trusted gateway.
- **DRY and concurrency:** common task/workspace admission handles cancellation,
  ownership survives concrete-resource replacement, and one helper revokes access
  before external cleanup. A workspace source sync does not acquire a global lock.
- **Testability:** deterministic service tests cover failed transitions and races;
  real Docker security tests verify negative reachability, separately from browser
  success. Fixture authentication does not establish production OIDC qualification.

## Detailed Findings

### Critical

**C1 — Network reachability bypasses application authorization. Fixed for the
qualified native topology; containerized-platform topology still needs live qualification.**

Baseline: [editor network selection](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/adapters/executors/docker/editor.go#L159),
[worker HTTP routes](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/adapters/http/agentctl/server.go#L54).
Violation: organization/workspace isolation and least privilege. Code-server and
agentctl are unauthenticated on their container interfaces. Sharing a network lets
one editor bypass the gateway and reach another workspace's editor or worker.
A dedicated routable bridge alone was also insufficient: a real probe reached a
worker's `/health` through `host.docker.internal`.

Resolution: editors now have internal-only generation networks. Native-host mode
uses an authenticated, fixed-upstream relay; its token is stripped before forwarding
to code-server and excluded from public serialization. A containerized platform
joins the private editor bridge directly. Relays/networks have scoped ownership,
concrete termination and orphan reconciliation. The strict security test denies
peer-editor, worker-IP and Docker-host-alias probes and verifies unauthenticated
relay access returns 403. Editor shells consequently have no internet egress;
controlled egress remains a product/technical decision.

### Major

**M1 — Checkout inspection and Docker mount resolution are not atomic. Open.**

Baseline: [path validation followed by container creation](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/adapters/executors/docker/editor.go#L127).
Violation: check/use consistency at a security boundary. An active worker can modify
the worktree while a read-only editor is created. `Lstat`/directory inspection is a
preflight check; the daemon later resolves the bind source by pathname. A directory
rename/symlink substitution between those operations is not prevented by repeating
the check. This race has not been adversarially qualified here; do not call the
current checks atomic isolation.

The refactor rejects sources/targets outside the exact task repository layout
before any Engine call. Completing the remedy requires protected mount parents or
a daemon-side inode-pinning design that also preserves exact selected-repository
identity. Moby's [safe-path implementation](https://raw.githubusercontent.com/moby/moby/master/daemon/volume/safepath/join_linux.go)
illustrates descriptor pinning, but its
[path evaluation](https://raw.githubusercontent.com/moby/moby/master/daemon/volume/safepath/common.go)
can first resolve in-volume symlinks: mounting subpaths of an alias to the entire
installation is not an adequate drop-in authorization fix. Any worker mount-layout
change must preserve existing session paths and prove restore plus follow-up.

**M2 — Ownership disappears between writable shutdown and replacement. Fixed.**

Baseline: [Open ownership check](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/server/taskeditor/service.go#L130).
Violation: lifecycle invariants. Checking only discovered-container ownership lets
another mutating member claim the task's editor during the handoff gap. Pending
request ownership is now authoritative until an explicit Stop or completed cleanup.
Open, Inspect and Stop apply the same check, including failed/pending states.

**M3 — Revocation depends on successful daemon inspection. Fixed once the
reconciler examines the task; scheduling latency remains an open performance concern.**

Baseline: [Run](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/server/taskeditor/service.go#L317).
Violation: fail-closed authorization. After membership loss, a Docker inspect error
previously prevented cancellation of an already-connected browser's lease. The
refactor revokes first, records Stop intent, then reconciles external resources.
A regression test reproduces the combined authorization-loss/backend-outage case.

**M4 — Failed Stop loses cleanup intent and successful Stop can retain failure. Fixed.**

Baseline: [Stop](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/server/taskeditor/service.go#L239),
[failure shortcut](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/server/taskeditor/service.go#L223).
Violation: reliable state transitions. Clearing the open request before an uncertain
Stop left no background cleanup work. An earlier `failed` bit could also survive a
later successful Stop. Explicit Stop intent is now retained until confirmed cleanup;
reconciliation cannot reopen it. Successful cleanup clears owner/failure state. A
failed relay remains inspectable so it cannot strand the editor's termination path.

**M5 — Global, non-cancellable admission can stall unrelated workspaces. Fixed.**

Baseline: [bulk-removal barrier](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/server/taskeditor/service.go#L388).
Violation: isolation, context-aware I/O and availability. A global `RWMutex` around
Docker/database operations ignored waiting requests' deadlines and blocked all
workspaces. Admission now uses cancellable workspace-scoped weighted semaphores,
then the task lock. Reference-counted task entries are removed only when no holder
or waiter remains; inactive GETs no longer accumulate task locks indefinitely.

**M6 — Polling cost and revocation latency grow with history and filesystem size. Open.**

Baseline: [idle reader](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/server/orchestration/editor_handoff.go#L27),
[reconciliation loop](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/server/taskeditor/service.go#L317).
Violation: bounded background work and operational visibility. A pass can cost
approximately `O(E × (H + F))`, with E editors, H historical runs and F Git metadata
entries, plus remote I/O. Sequential twenty-second attempts delay other tasks'
authorization checks. Errors are reflected in coarse status but lack safe operator
diagnostics. Use a cheap authorization path independent of slow recreation, bounded
concurrency, and minimal durable ownership reads; add structured reason codes and
identifiers without logging tokens, task contents or terminal data. Do not remove
ownership checks merely to make the loop faster.

### Minor

**m1 — HTTP origin equality and Host routing can disagree. Fixed.**

Baseline: [gateway constructor](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/adapters/http/server/task_editor_api.go#L38).
Differing schemes alone do not distinguish Host-based routes. Normalize case and
default ports, reject colliding authorities, and share the same authority matcher
between the gateway and server wiring. Tests cover those configurations. The local
default uses `editor.localhost` on the existing listener; HTTPS remains required
outside loopback names/addresses.

**m2 — A new HTTP transport for every proxied request disables connection reuse. Open.**

Baseline: [proxy transport](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/adapters/http/server/task_editor_api.go#L247).
This increases TCP/TLS setup, allocations and descriptor churn for editor assets.
Use generation/session-owned transports with explicit connection revocation and
expiry. A naive global transport would weaken the current cancellation guarantee;
optimize only with socket-lifetime regression coverage.

**m3 — Ticket stores are unbounded and pruning scans all tickets. Open.**

Baseline: [ticket pruning](https://github.com/josemiguelmelo/orkest/blob/e2d4e30/internal/adapters/http/server/task_editor_api.go#L119).
Repeated grant issuance costs `O(G + S)` per pruning operation and can retain large
numbers of tickets between expirations. Add per-user/global bounds and predictable
expiry cleanup. Imported source-ID matching was separately improved from nested
`O(E × S)` scans to set membership with `O(E + S)` work.

## Refactored Implementation

The actual repository implementation, rather than an untested replacement snippet,
now contains the following changes:

- `internal/server/taskeditor/service.go`: retained request ownership, revocation
  before daemon I/O, retryable Stop intent, cancellation-aware workspace admission,
  shared handoff transitions and safe inactive-entry eviction.
- `internal/adapters/executors/docker/editor_network.go`: private network ownership,
  trusted-gateway attachment and orphan reconciliation.
- `internal/adapters/executors/docker/editor_relay.go` and
  `tools/task-editor/relay.cjs`: fixed-upstream authenticated native-host access
  without an external route in the editor shell.
- `internal/adapters/http/server/task_editor_api.go`: separated launch/exchange/proxy
  handlers, canonical authority validation and server-derived relay credentials.
- `internal/adapters/executors/factory.go`, `cmd/server/wiring.go` and OpenAPI:
  always-available capability/API, independent of concrete executor selection.
- Regression/security/browser tests cover the reproduced faults and network fix.

The former enable/disable flag is ignored and removed from supported configuration.
Unsupported executors return unavailable. Runtime/model pins, worker wire formats
and persisted session paths are unchanged. Unresolved M1/M6 and remaining production
qualification prevent certifying this refactor as fully production-ready.

### Verification actually run

- `go test ./cmd/... ./internal/... ./db/... -timeout=3m`: passed ordinary Go tests.
- `go test -race ./internal/server/taskeditor ./internal/server/orchestration ./internal/adapters/executors/... ./internal/adapters/http/server ./cmd/server`: passed.
- `make lint`: passed, zero issues. A separate pinned-linter run with
  `--build-tags=docker,security` passed for Docker and server HTTP packages.
- `make check-runtime-boundaries`: passed; a sandbox cache-read denial was retried
  outside the sandbox. No architecture check remains skipped.
- `npm --prefix web run check:generated` and frontend typecheck: passed.
- `node --check tools/task-editor/relay.cjs`, relay/browser Prettier checks and
  `docker compose config --quiet`: passed.
- Real Colima arm64 Docker `TestDockerEditorLifecycle`,
  `TestDockerEditorNetworkIsolation` and Chromium `TestDockerEditorGatewayBrowser`:
  passed. The lifecycle run includes cleanup of a stopped relay. The security
  test probes both worker container addresses and Docker host aliases, as well as
  peer editors and unauthenticated relay access. Browser qualification uses the
  `editor.localhost` host routed to the existing listener.
- `make test-recovery`: passed deterministic race/recovery checks and pinned
  OpenCode 2.1.4 handshake, stdio MCP handshake and current/v1-manifest restoration
  followed by successful prompts, using the explicit previously qualified free
  model `opencode/deepseek-v4.1-flash-free`. No paid/different model was substituted.
- Last full `make check` remains blocked by three existing scheduled-job UI tests:
  stale empty-state text/button expectations, with those implementation/test files
  unchanged. It is not reported as passing. PostgreSQL editor concurrency,
  containerized-gateway networking, production TLS/OIDC and amd64 qualification
  were not established by the local fixture runs.
