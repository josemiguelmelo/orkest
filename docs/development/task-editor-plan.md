# Task VS Code and terminal implementation plan

Status: backend always wired by explicit user instruction; frontend and production qualification remain.
Last updated: 2026-11-14.

This document is the implementation checklist and cross-session handoff for task
browser editing. It records the agreed direction, proposed implementation choices,
delivery slices, verification and outstanding decisions. Checkboxes indicate
completed work only when its acceptance criteria have been verified.

## 1. Objective and confirmed direction

Add an **Open VS Code** action to a task, with its integrated terminal and access
to existing task repositories.

Confirmed user instructions:

- Keep the implementation simple and reuse existing executor backends.
- While an agent works, VS Code must be read-only.
- While the agent is idle, VS Code can be read/write.
- Do not require users to close VS Code before starting an agent, or prevent
  opening VS Code because an agent is working.
- Avoid new editor database storage. Reuse task files and execution state, and
  discover editor resources through the executor.
- Preserve implementation context and track delivery in this document.

This plan does not authorize deployment, publishing images or other external
resource creation. Runtime integration tests need their normal prerequisites.

## 2. Proposed first-release behavior

| Task condition | Repository access | Integrated terminal |
| --- | --- | --- |
| Agent idle, no pending work or unresolved ownership | Read/write | Enabled |
| Agent starting or executing | Read-only | Disabled |
| Agent waiting for approval or input | Read-only | Disabled |
| Execution state uncertain | Read-only | Disabled |

- One editor environment per task, initially owned by the requesting user.
- Only users with task mutation authority can open either mode. Viewer-facing
  review and collaborative terminal sharing are deferred.
- Open in a separate tab initially. A stable Orkest launch page reconnects the
  browser after environment recreation.
- Opening Code never starts an agent, clones repositories, changes branches or
  changes workflow state. Missing checkouts produce an explanatory empty state.
- Saved repository changes survive recreation. Editor settings/cache are
  temporary; terminal processes and unsaved browser buffers are not guaranteed
  to survive recreation.
- Closing the tab leaves the editor running. Explicit Stop terminates it.
- Starting agent work automatically stops editor terminal commands and changes
  the editor to read-only; no separate user confirmation is planned.

Workflow stage and absence of chat output do not establish that an agent is idle.
Use durable execution state, queued work and reconciled execution ownership.

## 3. Existing implementation anchors

Read [requirements](../requirements.md), [tech stack](../tech-stack.md),
[status](../status.md) and repository instructions before implementation.

| Area | Starting point |
| --- | --- |
| Executor selection and construction | `internal/adapters/executors/factory.go`, `cmd/server/wiring.go` |
| Agent lifecycle interfaces | `internal/server/orchestration/agent_launcher.go` |
| Task storage and Docker lifecycle | `internal/adapters/executors/docker/executor.go` |
| Local and Kubernetes execution | `internal/adapters/executors/local/executor.go`, `internal/adapters/executors/kubernetes/executor.go` |
| Repository preparation | `internal/adapters/executors/checkout/preparer.go` |
| Task execution reservations | `internal/adapters/postgres/task_session_repository.go`, `db/queries/task_session.sql` |
| Dispatch and follow-ups | `internal/server/orchestration/task_dispatcher.go`, `internal/server/orchestration/launch_service.go` |
| Task authority | `internal/server/task/service.go` |
| HTTP API and contract | `internal/adapters/http/server/task_run_api.go`, `internal/adapters/http/server/openapi.yaml` |
| Task UI and API client | `web/src/features/tasks/tasks.tsx`, `web/src/shared/api/` |

Task worktrees are already durable and scoped by organization/workspace/task.
They are current task files, not historical run snapshots. Agent session
configuration, runtime versions and restore behavior must remain compatible.

## 4. Small service, existing backends

Proposed files:

```text
internal/server/taskeditor/
    service.go
    backend.go
    model/

internal/adapters/executors/
    docker/editor.go
    local/editor.go
    kubernetes/editor.go
```

The service owns authorization, mode selection, lifecycle and reconciliation.
It has no persistence repository. Existing concrete executors implement its
consumer-owned interface:

```go
type Backend interface {
    StartEditor(context.Context, Spec) (Instance, error)
    InspectEditor(context.Context, Identity) (Instance, error)
    StopEditor(context.Context, Identity) error
}
```

Names and signatures are illustrative; keep the final interface minimal. Specs
carry server-resolved scope, repositories, access mode and resource generation,
not an agent `RunConfig` or browser-provided host paths.

Reuse executor clients, networking and task path calculation. Keep editor and
agent resource identities and termination paths independent. Do not introduce a
generic workload framework. No agentctl API or ACP changes are expected.

## 5. Storage and resource discovery

No new database table, migration, editor event journal or persisted lease system.

| Information | Source |
| --- | --- |
| Repository files | Existing durable task directories |
| Required mode | Existing task execution state and reconciliation |
| Editor existence, mode and endpoint | Executor inspection |
| Resource identity and requesting user | Names and ownership metadata |
| Editor home/settings/cache | Temporary storage |

- Preserve the exact existing task scope hash and path format when extracting
  shared helpers. Do not migrate worktrees or session state.
- Mount the complete task-scoped durable `work` directory at `/task`, including
  agent-created clones and non-Git files. `agentctl-state` is a sibling directory
  and must never be mounted.
- Generate a `.code-workspace` file for `/task` and retain it for the editor
  container's lifetime.
- Validate resolved paths and mount sources. Recreate the editor when the
  requested read/write mode changes.
- Never mount agent state, runtime homes, provider credentials, workspace secret
  environment values or configured agent host credential mounts.
- Never reset, clean or switch branches as a side effect of opening the editor.

Use deterministic installation/task-scoped resource names. Metadata identifies
the managed editor, requesting user, mode and instance generation, without
credentials. Verify ownership before adopting an existing resource.

Cache connection details in memory only as an optimization. Inspect actual
resources to recover them after restart. Stop resources by concrete instance
identity so stale cleanup cannot delete replacements. Duplicate opens inspect
and reuse a compatible resource rather than creating duplicates.

## 6. Mode transitions and execution handoff

Enforce repository access with read-only/read/write mounts, not editor settings
alone. In both modes the editor's own temporary home/cache remains writable.

Initially, mode changes recreate the editor:

```text
Revoke old browser access
    → stop old editor and terminal processes
    → confirm termination
    → start replacement with required repository mount mode
    → reconnect browser
```

In read-only mode also disable the integrated terminal, extension installation
and automatic workspace tasks. These restrictions do not establish a complete
sandbox for untrusted Viewer access; that feature remains deferred.

### Agent starts while a writable editor exists

1. Accept work through the existing durable execution path.
2. Derive read-only as the desired editor mode from pending execution.
3. Revoke access to the writable editor generation and stop it.
4. Confirm its terminal/process environment has terminated.
5. Deliver executable work to the agent.
6. Recreate an open editor read-only; agent execution need not wait for that
   replacement to become ready.

There is no user-held editor reservation and no manual editor-close prerequisite.
However, executable agent work must not overlap an existing writable terminal.
If termination cannot be confirmed, expose the existing launch failure path;
never claim a safe handoff or allow simultaneous writes.

Apply the handoff before execution through all task entry points:

- Explicit starts and new/continued sessions.
- Chat follow-ups delivered to existing workers.
- Queue resume and pending prompt delivery.
- Workflow dispatch and stage integration actions.
- Recovery of already accepted work.

An existing worker can execute queued work independently of the platform. Trace
that behavior before enabling writable access: never make the editor writable
while accepted worker-side work can still execute.

### Return to read/write

Only after queued work is drained and execution ownership is resolved, recreate
an open editor read/write. Keep it read-only throughout consecutive prompts,
approval/input waits and uncertain ownership. Editor access never consumes an
agent member slot or changes workflow state.

### Race handling and initial deployment boundary

- Serialize lifecycle and execution handoff operations per task.
- Recheck durable execution state before creating or exposing a writable editor.
- Agent starts take precedence over a concurrent writable transition.
- Use generations to reject obsolete connections and cleanup operations.
- On startup, reconcile editors before delivering work or granting write access.
- The proposed initial scope is one active platform process coordinating editor
  transitions per installation. Cross-replica coordination is not implemented by
  an in-memory mutex. Validate and document this deployment constraint before
  release; do not claim active-active platform support for this feature.

## 7. Executor implementation

| Agent executor | Proposed editor implementation |
| --- | --- |
| Docker | Separate code-server container with bind mounts |
| Kubernetes | Separate code-server Pod and internal Service, existing RWX PVC |
| Local | Docker editor accessing the Local agent's host task directories |

### Docker

- Pinned configurable editor image; compatible non-root UID/GID.
- Repository mounts use the selected access mode.
- Private endpoint using existing network/loopback configuration patterns.
- Reasonable resource limits; no privileged access or Docker socket mount.
- Stop and remove the container without deleting repository data.

### Kubernetes

- Separate Pod, not an agent sidecar, and private ClusterIP Service.
- Mount only repository subpaths from the existing shared RWX PVC.
- Compatible UID/GID, resource limits, readiness and no service-account token.
- Confirm deletion using resource identity before replacement; preserve PVC data.
- Reuse the Kubernetes client and configuration. Real cluster/PVC qualification
  is required; current documentation does not establish that qualification.

### Local

A native same-user process cannot enforce repository read-only access merely
through VS Code settings. The proposed first version therefore uses the shared
Docker editor implementation for Local agents.

- Require Docker and daemon-visible task paths.
- Report unavailable support when those prerequisites are missing.
- Reuse Docker implementation rather than duplicating it.
- Native sandboxed Local editor support is deferred.

## 8. Image and tooling

- Provide a pinned code-server image with Git, shell, certificates and suitable
  writable temporary directories. Pin versions and verify downloaded artifacts.
- Allow an operator-configured editor image with project tools installed.
- Document that terminal commands execute in the editor environment; sharing
  files does not guarantee agent toolchain or dependency equivalence.
- Do not extend execution-profile image composition in this release.
- Do not automatically share agent credentials or expose port previews.

PR 1 finding: the isolated spike uses separate read/write and read-only image
targets. Read-only PTY creation requires an immutable upstream module guard;
settings or an inherited Node preload were insufficient. Carry equivalent
qualified enforcement into PR 3. See [evidence](task-editor-pr1-evidence.md) for
versions, browser cookies/reconnect and daemon-visible permission assumptions.

## 9. API, gateway and browser access

Proposed control endpoints:

```text
GET    /.../tasks/{taskID}/editor
POST   /.../tasks/{taskID}/editor
DELETE /.../tasks/{taskID}/editor
```

GET inspects availability/status; POST opens or reconnects; DELETE stops. Mode is
server-derived. Return status, effective mode and a browser launch URL, without
private endpoints or credentials.

The authenticated HTTP/WebSocket gateway must:

- Check task mutation authority and requesting-user ownership.
- Validate WebSocket Origin and protect lifecycle mutations against CSRF.
- Reject obsolete resource generations and close old connections on transitions.
- Resolve upstream destinations server-side and strip platform credentials.
- Expire/revoke browser access; never log grants, credentials or terminal content.
- Use an editor-specific browser session on a separate origin, launched through
  a short-lived grant from the platform.

Update platform OpenAPI and regenerate the frontend client. Keep the agentctl
contract unchanged unless an implementation finding demonstrates a concrete need.

## 10. Task UI and shutdown

- Add Open VS Code and Stop editor actions with mode and lifecycle status.
- Use a stable launch page to reconnect after recreation.
- Display switching, starting, ready, stopping, failed and unavailable states.
- Explain that starting agent work stops terminal commands and reconnects the
  editor read-only, without an extra confirmation flow.
- Explain saved-file persistence and possible unsaved-buffer loss.
- Keep Start agent available while the editor is open.
- Closing the tab leaves the editor running; explicit Stop terminates it.
- Automatic idle shutdown and custom terminal components are deferred.

## 11. Restart and deletion reconciliation

- Discover managed editor resources and compare with durable execution state.
- Never grant writable access when execution state cannot be established.
- Stop conflicting writable editors before delivering further agent work.
- Reuse compatible resources; confirm termination before replacement.
- Disconnection alone never proves resource termination.
- Task deletion stops/reconciles its editor before associated storage removal.
- Platform restart must not accidentally stop independent agent workers.

## 12. Delivery checklist

The proposed [first delivery and PR breakdown](task-editor-first-delivery.md)
expands slices 1–3 and the Docker-relevant parts of slice 6 into nine focused PRs.
The first user-facing milestone is Docker-only; Local and Kubernetes follow in
separate deliveries. PR 1 is the initial feasibility deliverable. This sequencing
is a planning proposal and does not mark any implementation work complete.

Each slice must update this document with changes, checks actually executed,
limitations and the next action. Do not mark unavailable checks passed.

### Slice 1 — Docker feasibility spike

- [x] Pin code-server and prepare an editor image.
- [x] Open a real existing checkout and multi-root workspace.
- [x] Verify Git inspection, editing and integrated terminal.
- [x] Prove read-only mounts reject repository writes.
- [x] Prove recreation terminates terminal processes and preserves saved files.
- [x] Prove authenticated proxy and browser reconnection behavior.
- [x] Record unsaved-buffer behavior and any implementation changes needed.

Acceptance: real Docker demonstrates both access modes and a safe transition;
mock-only checks do not satisfy this slice.

### Slice 2 — Core service and execution handoff

- [x] Add the minimal service/interface and reuse task storage calculation.
- [x] Implement discovery, ownership checks and instance generations.
- [x] Integrate handoff across every execution entry point listed in section 6.
- [x] Handle queued work, approval/input waits and uncertain ownership.
- [x] Serialize per-task transitions and reject stale writable transitions.
- [x] Add startup reconciliation and document deployment constraints.
- [x] Verify existing session restore and successful follow-up compatibility.

Acceptance: no writable editor overlaps agent execution, including races,
worker-side queues and restart scenarios; no editor database storage is added.

### Slice 3 — Docker feature, gateway and UI

- [x] Complete Docker resource lifecycle, networking and cleanup.
- [x] Add lifecycle endpoints and authenticated HTTP/WebSocket gateway.
- [x] Update OpenAPI and regenerate the frontend client.
- [ ] Add task actions, launch page, mode/status and automatic reconnect.
- [x] Enforce authority, requesting-user access and stale-session revocation.
- [x] Cover missing checkouts, failure messages and explicit Stop.

Acceptance: a user edits a task, starts its agent without manually closing Code,
sees read-only reconnect, and regains editing/terminal access when work finishes.

### Slice 4 — Local agent with Docker editor

- [ ] Reuse Docker editor support with Local task paths.
- [ ] Validate Docker availability and daemon-visible paths.
- [ ] Report unsupported configuration precisely.
- [ ] Verify Local agent/editor transitions and document prerequisites.

Acceptance: real Local execution and Docker editing share task files with the
same enforced access-mode behavior.

### Slice 5 — Kubernetes editor

- [ ] Implement Pod/Service discovery and lifecycle with existing client.
- [ ] Implement PVC subpath mount modes, readiness and resource limits.
- [ ] Confirm identity-aware termination, replacement and retained files.
- [ ] Document network policy and operator prerequisites.
- [ ] Qualify on a real cluster and RWX volume provisioner.

Acceptance: actual Pod, Service and PVC behavior verifies mode changes, terminal
shutdown and restart reconciliation. Otherwise report this slice unqualified.

### Slice 6 — Verification, documentation and rollout

- [ ] Complete acceptance coverage below.
- [ ] Run `make fmt`, `make lint` and relevant `make check` verification.
- [ ] Update requirements with confirmed behavior, preserving proposal labels.
- [ ] Update architecture, executor references, configuration and `docs/status.md`.
- [ ] Document upgrade/mixed-version restrictions: old platform writers do not
  enforce the editor handoff and must be stopped before enabling the feature.
- [ ] Review diff for unrelated changes, secrets and accidental session changes.

Acceptance: implemented claims match executed checks, and remaining limits are
explicit. Deployment/publishing requires separate authorization.

## 13. Verification matrix

| Scenario | Required result |
| --- | --- |
| Open during agent work | Read-only repositories; terminal disabled |
| Open while idle | Editing and terminal available |
| Start with writable editor open | Start accepted; writable environment stops before execution |
| Finish with no queued work | Open editor returns read/write |
| Approval/input wait or queued work | Remains read-only |
| Concurrent Open/Start/mode switch | No writable overlap; stale transition loses |
| Duplicate Open | One owned environment, no duplicate resource |
| Shutdown uncertain | Visible failure; no unsafe execution handoff |
| Platform restart during transition | Discover/reconcile; no duplicate or unsafe write access |
| Recreation and Stop | Saved/untracked files and branches remain; terminals terminate |
| Cross-workspace, Viewer or other-user attach | HTTP and WebSocket denied |
| Agent session continuation | Existing session restores and accepts another message |

Use real PostgreSQL-backed tests for existing execution-state concurrency, race
detection for transition logic, browser tests, and real executor checks. Report
credentials/infrastructure limitations and distinguish deterministic fixtures
from real runtime continuation.

## 14. Deferred work

- New editor database storage, durable editor history and lease frameworks.
- Viewer-facing review mode and collaborative terminal sharing.
- Native sandboxed Local editor.
- Embedded task-page editor and standalone Terminal tab.
- Automatic idle shutdown, port previews and credential sharing.
- Automatic execution-profile/editor-image composition.
- Historical run snapshots.
- Active-active editor coordination across platform replicas.

## 15. Decisions and findings to resolve during implementation

These are implementation proposals/qualification points, not additional confirmed
product requirements. Record decisions here rather than silently changing scope.

| Topic | Proposed starting point / required finding |
| --- | --- |
| Mode switch | Recreate environment; verify browser buffers/reconnect in spike |
| Deployment | One active transition coordinator; confirm acceptable operational scope |
| Local | Docker-backed editor; verify paths for supported Local deployments |
| Browser origin | Separate editor origin; verify cookies/grants/proxy configuration |
| Ownership | Requesting user attaches; specify who may stop another user's editor using existing authority policy |
| True idle boundary | Trace queue/worker ownership; identify when write access is demonstrably safe |
| Terminal cleanup | Confirm descendants cannot keep writing after stop is acknowledged |
| Inspection vs full task diff | Git working-tree inspection initially; base-commit comparisons deferred |

## 16. Cross-session progress and handoff

Current slice: task editor embedded workspace and task-page UX polish. The task
page has a clear task header with Task and VS Code views. Selecting VS Code
connects automatically; the tab indicates whether the editor is online. Leaving
or refreshing the task page requests editor stop. The editor gateway is embedded
as a full-page workspace surface. The Docker editor mounts the entire durable
`work` tree at `/task`; `agentctl-state` remains outside the mount. Production
isolation and release qualification remain outstanding. Next action: restart the
local backend and qualify the embedded editor against the current task checkout.

| Date | Slice | Changes / decisions | Verification actually run | Blockers / next action |
| --- | --- | --- | --- | --- |
| 2026-11-14 | Planning | Recorded simple design, automatic mode transitions and no new database storage | Documentation review only; no feature implementation/tests | Start Docker feasibility spike |
| 2026-11-14 | First-delivery planning | Proposed Docker-only first release in nine focused PRs, with dependencies, per-PR acceptance and enablement/rollback gates | Reviewed requirements, stack, status, delivery/testing guidance, executor paths and worker prompt/queue code; no feature checks run | Implement PR 1 feasibility spike; qualify idle boundary and browser/session approach before enablement |
| 2026-11-14 | Slice 1 / PR 1 | Added pinned editor image pair, isolated two-repository browser/proxy harness and focused Make targets; immutable read-only PTY guard required | Real Docker arm64 + Chromium passed `make test-task-editor`; mount/Git write rejection, terminal descendants stopped, offline extension denied, generation reconnect, saved files/branches retained, unsaved edit lost; lint 0 issues; syntax/format/diff checks passed | [Full evidence](task-editor-pr1-evidence.md); proceed to PR 2; production HTTPS, authorization, execution handoff and amd64 remain unqualified |
| 2026-11-14 | PR 2 / discovery | Extracted historical task storage path calculation; shared repository path validation; added authorized task selection resolver and Docker checkout inspector, symlink/Git metadata checks, structured missing roots, deterministic container manifest paths, private materialization; updated continuation fixture to use extracted storage | Focused tests passed with real `git init` fixtures and non-mutation checks; `make lint` and architecture checks passed. PostgreSQL bounded task-listing subtest passed against a disposable DB; broader repository suite hit unrelated delivery/migration failures. `make test-recovery-unit` passed outside sandbox with race detection. Docker images, launch smoke, and `TestDockerSessionRestore` passed on Colima with bundled `opencode/beam-free` selection; no model prompt sent. OpenCode continuation skipped without explicit binary/model settings | Real pinned OpenCode restore plus successful follow-up remains unverified; complete after explicit model configuration is available |
| 2026-11-14 | Embedded task editor and work summary | Added task detail status/open/stop controls and Task / Changes / VS Code tabs. Changes shows the latest agent response, tool file references, and reported before/after diffs. VS Code embeds the authenticated gateway in the task page; frame policy allows only the configured platform origin. The editor mounts the task's whole durable `work` tree at `/task`; sibling `agentctl-state` is excluded, and the manifest persists until stop. | `npm run typecheck`, `npm run lint`, `make fmt`, `make lint` (0 issues), and `git diff --check` passed. Tests were not run. Docker/browser embedding remains unverified in this sandbox. | Restart backend and qualify the embedded editor against the current task checkout. Production qualification remains outstanding |
| 2026-11-15 | Task page UX polish | Reworked task header hierarchy and task navigation into a segmented tab bar; strengthened run/change status and diff readability; tuned the embedded editor workspace and responsive layout. | `npm run typecheck`, `npm run lint`, and `git diff --check` passed. Tests and live browser rendering were not run. | Review in the browser and adjust from live feedback |
| 2026-11-15 | Task editor interaction | Removed the Changes tab, added online/offline status to the VS Code tab, connected automatically on selection, and requested editor shutdown on task-page exit or refresh. The embedded editor now fills the task workspace surface with compact controls. | `npm run typecheck`, `npm run lint`, `make fmt`, `make lint` (0 issues), and `git diff --check` passed. Tests and live browser lifecycle behavior were not run. | Restart the backend and qualify auto-open and page-exit shutdown in the browser |

At each handoff, update the current slice, next action and table. Record relevant
files/commits and exact meaningful check results. Preserve unrelated user changes.

### Backend increment verification (2026-11-14)

Implemented the Docker adapter, task editor coordinator, all platform work-delivery
guards, startup reconciliation, requesting-user HTTP/WebSocket gateway, scoped
lifecycle API and generated client. Added default-off configuration, image/test
Make targets and operator notes. No schema or worker wire changes.

Passed so far: focused package/race tests; architecture boundaries; real Colima
`TestDockerEditorLifecycle`; real Colima/Chromium `TestDockerEditorGatewayBrowser`
through the Go gateway, including terminal creation and read-only denial. These
browser fixtures do not establish production OIDC/PostgreSQL/TLS qualification.
Full check and pinned recovery results are recorded after completion below.

Pinned recovery verification passed using the pinned executable at
`/private/tmp/orkest-opencode-pinned/node_modules/@opencode/cli-darwin-arm64/bin/opencode`
and the previously qualified explicit free selection
`ORKEST_TEST_MODEL_PROVIDER=opencode ORKEST_TEST_MODEL_ID=deepseek-v4.1-flash-free`:
`make test-recovery` passed deterministic race/recovery tests plus real current/v1
manifest restore and successful follow-ups, handshake and stdio MCP handshake.
The globally installed executable was 2.3.0 and was not used as a substitute for
the pinned runtime. Production PostgreSQL editor races remain unqualified.

Final backend checks: full `make check` passed boundaries, lint, all ordinary Go
tests, generated client consistency and frontend lint, then failed three existing
scheduled-job empty-state UI tests (`New scheduled job` expected; unchanged implementation
renders `Create your first job`). Frontend typecheck and `make build` passed
separately. The final focused race suite and `make test-task-editor-backend` passed,
including a file written through the real integrated terminal. Server wiring was
corrected to consume optional executor-factory editor capabilities: it no longer
imports or type-asserts the Docker executor. Factory tests cover disabled editing,
Docker capability selection and unsupported Local execution.

### Production review and always-available editor correction

The user explicitly requires the editor feature to always be enabled, superseding
the earlier proposed default-off gate. Removed the gate from configuration,
factory selection, service wiring, routes and API descriptions. Supported Docker
executors always expose editor capabilities, using the pinned default image pair;
unsupported executors keep the same always-present API and return unavailable.
The server remains independent of concrete executor implementations.

Regression tests reproduced lost ownership during handoff, stale failure status,
revocation dependent on backend inspection, and non-cancellable workspace-removal
waits. The refactor preserves pending ownership, retries failed Stop intent,
revokes leases before inspection and uses cancellable per-workspace admission.
A set replaces nested imported-source membership scans. Gateway handlers are
separated and normalized HTTP authorities are validated for unambiguous routing.
See [the full review](task-editor-review.md) for unresolved production blockers and
verification evidence; always-available routes do not certify production readiness.

The critical network finding was reproduced against real Docker twice: direct
peer-editor access, then a host-alias bypass after separating bridge networks.
The corrected implementation keeps editors on internal-only networks; native-host
access uses a token-authenticated fixed-upstream relay, while a containerized
platform joins the private editor network directly. Strict peer/worker/host-alias
probes now pass; unauthenticated relay access is denied. The image pair includes
the relay asset, and Compose passes editor images/origins unconditionally. The
local default editor hostname is `editor.localhost` on the existing HTTP port.

Final review follow-up verification: ordinary Go tests and the focused race suite
passed; lint passed both normally and with Docker/security build tags. Generated
client consistency, frontend typecheck, relay syntax/format, Compose validation
and architecture boundaries passed. Real Colima lifecycle (including failed-relay
cleanup), peer/worker/host-alias isolation, unauthenticated relay denial and the
Chromium terminal/mode-transition flow passed using `editor.localhost` routing.
Pinned OpenCode 2.1.4 recovery and current/v1-manifest successful follow-ups passed
again using the same explicit free model. The prior full-check scheduled-job UI
failures remain unrelated and unchanged; no production qualification is implied.

### Major review fixes completed

Protected checkout ancestry through worker parent/read-write checkout mounts at
unchanged paths. Legacy/unsafe workers deny editor admission while remaining
untouched. Orphan cleanup claims generation admission, freshly inspects ownership,
and refuses network disconnection while execution endpoints survive. Independent
authorization watchdogs revoke leases outside lifecycle locks; authorization and
lifecycle work have bounded concurrency. Revoked generations cannot be renewed.

Verification: all ordinary Go packages; focused race suites; normal and
Docker/security-tagged lint; architecture boundaries; real Colima protected mounts,
legacy-worker refusal, lifecycle, peer/worker/host-alias isolation, existing Docker
storage/restart restoration and legacy-layout restore followed by successful marker
recall. The latter retained session ID, untracked checkout file and branch using
pinned OpenCode 2.1.4 and the explicit previously qualified free model
`opencode/deepseek-v4.1-flash-free`. No runtime/model/configuration/wire pins or
persisted session paths changed. Full frontend/browser/PostgreSQL/deployment
qualification was not rerun; previously documented limits remain explicit.
