# Task editor backend

The backend increment combines the Docker lifecycle, mode coordinator, execution
handoff, browser gateway and lifecycle API (planned PRs 3–7) in one review unit.
It reuses PR 2 discovery. Task-page actions and automatic browser reconnection
across generations remain frontend work; the backend launch page exchanges a
single-use grant and opens the current editor. This is not release enablement.

## Implementation and ownership

`internal/server/taskeditor` owns authorization and per-task serialization. The executor factory exposes optional editor backend and checkout-inspector
capabilities; server wiring has no Docker import or concrete executor assertion.
The editor service and gateway depend only on consumer-owned contracts, independently
of the agent runtime, model or Git provider. The
Docker adapter shares the existing Engine client and data root, using a numeric
non-root container UID/GID. Each generation has an internal-only bridge; it never
joins the worker control network. A containerized platform joins that editor
bridge directly as the trusted gateway. A native-host platform uses a fixed-upstream,
token-authenticated relay; only that relay has an external network attachment.
The shell has no route to peer editors or Docker host aliases. The private relay
token is never forwarded to code-server or returned by the lifecycle API. Only selected existing checkouts and a generated workspace manifest are
mounted. Temporary manifests live beneath `agent-executions/editor-workspaces`
so the daemon can see them; they are deleted after startup. Editor home/cache use
bounded tmpfs. No agent state, credentials, configured host mounts, Docker socket,
privileged mode or agent environment is supplied. Containers have memory/CPU/PID
limits, a read-only root filesystem and no added capabilities.

Read-only containers use the separately built immutable PTY-denial image, readonly
repository/Git mounts, disabled extensions and disabled automatic tasks. Saved
files and branches remain in task storage. Recreation and Stop terminate the
container and terminal descendants; terminal commands and unsaved buffers are not
retained. Closing the browser leaves the editor running.

The requesting user owns attachment and explicit Stop. Every lifecycle/access
request rechecks task mutation authority; Viewers cannot open either mode. Another
member cannot attach to or explicitly stop an owned editor. Selection updates and
source removal are internal lifecycle transitions and revoke access before their
mutation, regardless of the editor owner.

## Checkout mount safety

New Docker workers mount `/workspace/repositories` read-only and overlay selected
checkouts as writable binds at their existing paths. Checkout roots and their
parents cannot be renamed or replaced by the worker; files and Git metadata inside
each selected checkout remain writable. Task storage, session paths, runtime/model
configuration and wire formats are unchanged. New scratch directories belong
outside the protected repository parent.

Editor admission inspects existing worker mounts before binding checkouts. Legacy
workers, privileged workers, missing parent protection, uncertain inspection or
writable aliases to checkout ancestry return unavailable. Stopped legacy containers
also deny admission because restart policies can reactivate them. No worker is
stopped or replaced by opening an editor. Let legacy workers retire through their
existing lifecycle; later runs use the protected layout and can restore the same
session. The Docker continuation test proves an old-layout session restores with
the protected layout and successfully answers a follow-up while retaining files
and its branch. Single-coordinator handoff remains required.

## Idle proof and execution handoff

A live worker can start previously accepted commands without a platform send.
Therefore a worker's `ready` status, empty queue snapshot, heartbeat timeout or
silent chat never establishes editor write safety. Writable access requires all
recorded task launches to be durably completed/failed **and** executor inspection
to confirm every corresponding worker has stopped. Any nonterminal launch or
inspection/database failure yields read-only access. The existing worker emits a
terminal run event when its task-session queue drains; existing completion cleanup
terminates that worker before recording the final launch state. No worker/ACP
contract, session configuration, runtime pin, storage hash or migration changes.

A shared task guard revokes generation access, stops a writable editor by concrete
container ID and confirms termination before process startup/session restore,
initial/follow-up prompt delivery, queue resume/discard or connection outbox
replay. Workflow and stage-integration dispatch already converge on these paths.
The guard stays held through delivery. Failed/uncertain editor shutdown fails the
existing work-delivery path; no executable work is sent. Existing read-only editors
may remain during delivery; creation still shares task admission. Four bounded
lifecycle workers recreate explicitly opened editors in the currently allowed mode.

Each generation has an independent authorization watchdog, outside task/workspace
locks and Docker/filesystem operations. It checks every second with a two-second
budget including admission; at most eight authorization checks run concurrently.
Failure or admission timeout cancels the lease, closing existing proxy connections.
A revoked generation is never renewed, even after authority returns. Service shutdown
cancels watchdogs and leases. Sanitized logs contain scope IDs and reason codes.

Startup stops discovered editor instances before recorded prompt recovery or
admitting opens. Independent agent workers continue. Repository-selection updates
hold the same task lock and revoke all editor modes before saving. Imported-task
removal fences editor transitions while it stops editors whose provider/source ID
may be removed. Retained source IDs and manual tasks remain connected; editors of
another integration using the same provider may also reconnect conservatively.
Authorization loss or removed tasks revoke active connections through the independent
watchdog; lifecycle reconciliation subsequently cleans up resources. Database
unavailability is fail closed.

## API and browser access

The platform always exposes authenticated
`GET`, `POST`, `DELETE /api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/editor`.
GET inspects only. POST opens/reuses and returns state, mode, generation, missing
checkouts and a `launchUrl`. DELETE stops. Failures are sanitized; private upstream
addresses and host paths never appear in responses. Unsupported executors return
`503 editor_unavailable` and preserve their existing execution behavior.

Cookie-authenticated mutations require the configured platform `Origin`.
Non-browser Bearer clients may omit it. Browser access uses a distinct origin:
POST returns a one-minute, single-use grant in the launch URL fragment (not the
request URL); the launch page removes the fragment and exchanges it with an
Origin-validated POST. It receives a 15-minute HttpOnly, path-scoped, SameSite
Strict cookie. Cookies are Secure on HTTPS. Grants and sessions are memory-only;
a platform restart invalidates them. Launch URLs must not be logged or persisted.

HTTP and WebSocket requests are bound to the requesting user, task scope and
resource generation. The proxy validates browser Origin, resolves upstreams
server-side, strips Cookie/Authorization/forwarding credentials, rewrites the
validated upstream Origin and strips upstream cookies. Expiry and generation
revocation close existing upstream connections, including WebSockets. Old sessions
cannot attach to replacements. Open VS Code again to obtain a replacement grant;
the future frontend reconnect flow must use the authenticated lifecycle API.

## Configuration and operations

The editor service and API are always wired. There is no enable/disable flag;
legacy `ORKEST_EDITOR_ENABLED` values are ignored. Unsupported executors return
an explicit unavailable result. Image defaults are the pinned tags below; custom
images and origins remain configurable. Production readiness is assessed in
[the code review](task-editor-review.md), including unresolved isolation blockers.
Build the two pinned code-server 4.145.2 targets with
`make docker-task-editor-images`. Configure distinct
`ORKEST_EDITOR_IMAGE` and `ORKEST_EDITOR_READONLY_IMAGE`; immutable deployment
references are recommended. No images are automatically built, pulled or published.
The editor does not automatically inherit an agent's execution-profile toolchain.

Set `ORKEST_PLATFORM_ORIGIN` and `ORKEST_EDITOR_ORIGIN` to distinct absolute origins
without trailing slashes. Route both hosts to the existing platform HTTP listener;
the editor Host selects the dedicated gateway. HTTPS is required outside loopback.
The local default is `http://editor.localhost:8080`, alongside platform
`http://localhost:8080`, using the same listener and different Host headers.
Override both origins when using a different public port or TLS hostnames.
Do not route editor-host requests directly to code-server. TLS termination must
preserve Host and support WebSocket upgrades. The gateway strips platform
credentials even when development origins share a hostname.

The containerized platform must use its Docker container ID/name as its hostname
(the default container-ID hostname works) and be attached to
`ORKEST_AGENTCTL_NETWORK`; the backend verifies that identity before attaching it
to an editor's private bridge. No editor joins that worker network. Native-host
mode publishes only the authenticated relay on loopback, never code-server itself.
Both modes keep editor shells on internal-only networks; internet-dependent
terminal commands require a future controlled-egress design. This is a current
security constraint, not a newly confirmed product requirement.

The daemon must see the configured data directory. Checkouts must already be
accessible to the numeric non-root UID/GID; a root platform UID falls back to
editor UID/GID 1000:1000. Discovery never changes checkout permissions. A remote
Docker daemon without these path/network properties is unsupported. Local and
Kubernetes editors are deferred. The relay and network are stopped/removed with
the editor; discovery also reconciles orphan relays/networks after uncertain
creation. Cleanup claims generation admission and re-inspects concrete ownership;
independent list snapshots never establish absence. Networks with execution endpoints
are rejected before the gateway can be disconnected. Unrelated endpoints are never
forcibly removed.

Exactly one active platform process may coordinate editors in an installation.
There is no active-active fencing or distributed editor lock. Before enabling,
stop old platform writers that lack handoff enforcement; leave independent workers
running. Startup reconciliation must succeed before work delivery resumes. Before
rollback, use the guarded version to Stop all writable editors and verify
termination. There is no feature gate to use as a cleanup mechanism. Restart reconciliation preserves saved checkouts but stops old
editor environments; users obtain new grants after restart.

## Verification and remaining release qualification

Focused race tests cover concurrent Open, serialization with delivery, uncertain
stops, stale generations, authority/ownership, selection/removal fencing and idle
proof. HTTP/WebSocket tests cover grant replay, Origin checks, credential stripping,
revoked authority and closing an existing socket on handoff. Real local Docker
verifies mount modes, saved-file retention, discovery and confirmed Stop. Real
Chromium through the Go gateway/code-server verifies grant exchange, static assets,
WebSockets, terminal creation, handoff and readonly terminal denial.

Run `make test-task-editor-backend` with a reachable Docker daemon and
`TASK_EDITOR_BROWSER=/absolute/path/to/chromium`. The target builds the pinned production image pair locally; the Docker lifecycle test accepts `ORKEST_EDITOR_IMAGE`
and `ORKEST_EDITOR_READONLY_IMAGE` overrides. The browser test uses the same local image pair. Its test-only task/identity fixture is not production OIDC or a
PostgreSQL qualification. Run `make check`, the focused race suite and
`make test-recovery` as well. Recorded commands/results are in the main plan.

Production TLS/cookie routing, amd64, real PostgreSQL Open/Start/restart concurrency,
queued-work crash-boundary qualification and full frontend journeys remain release
qualification work. Do not infer those from fake repositories, a browser fixture,
or successful runtime continuation. Deployment and image publishing require their
own authorization.

Final broad-check result: `make check` passed architecture checks, `make lint`
(0 issues), all ordinary Go tests, generated-client consistency and frontend lint,
then failed three existing scheduled-job UI tests in
`web/src/features/scheduled-jobs/jobs.test.tsx` (103 frontend tests passed).
Those tests expect `No scheduled jobs` and a `New scheduled job` button in the
empty state; the unchanged implementation renders `No scheduled jobs yet` and
`Create your first job`. Frontend typecheck and
`make build` passed separately. These unrelated UI files were not changed.
The final focused race suite and `make test-task-editor-backend` passed, including
writing `browser.txt` from the real integrated terminal.

Review follow-up: ownership and failed-stop intent survive recreation; leases are
revoked before daemon inspection; task locks are cancellable and scoped per
workspace, and inactive task entries are evicted without replacing locks held by
waiters. The strict `make test-task-editor-security` target first reproduced
cross-workspace editor access and host-alias access to an unauthenticated worker.
With internal-only editor networks plus the authenticated native relay, both
probes are denied, and unauthenticated relay access returns 403. The browser still
saves a file through the terminal and reconnects through mode replacement.
Worker parent protection, stale-snapshot cleanup and independent authorization
revocation are covered by the focused review fixes below.

### Major review fix verification

Focused ordinary/race tests cover stale container snapshots, uncertain cleanup,
active network endpoints, legacy/privileged worker mounts, writable aliases,
revocation while a lifecycle lock is held, authorization admission timeout,
generation invalidation after authority returns, and shutdown.

Real Colima arm64 `TestDockerProtectedCheckoutMounts` denies checkout/parent renames,
retains writable checkout contents and branches, admits a read-only editor beside
a protected worker, and refuses an editor beside a legacy worker without stopping
that worker. `TestDockerEditorLifecycle` and `TestDockerEditorNetworkIsolation`
passed. `TestDockerLegacyCheckoutContinuation` restores an old-layout session into
a protected-layout worker and successfully recalls a marker on its next prompt,
preserving the session ID, untracked file and branch. It used pinned OpenCode 2.1.4
and the explicit previously qualified `opencode/deepseek-v4.1-flash-free` model.

Run the continuation check with `DOCKER_HOST` configured for the local daemon and
explicit `ORKEST_DOCKER_TEST_MODEL_PROVIDER` / `ORKEST_DOCKER_TEST_MODEL_ID`:
`go test -tags=docker ./internal/adapters/executors/docker -run '^TestDockerLegacyCheckoutContinuation$' -count=1 -timeout=4m`.
This check makes two model calls; the separate storage-only restore test does not.
