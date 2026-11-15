# Task editor: first delivery and PR breakdown

Status: Docker backend and task-page implementation exist; Local/Kubernetes executor support and release qualification remain.
Last updated: 2026-11-15.

This expands the [task editor plan](task-editor-plan.md), which remains the source
of confirmed behavior and overall scope. PR numbers below are local planning IDs,
not GitHub pull requests. No external PRs or resources have been created.

## Delivery boundary

The task editor target supports **Docker, Local, and Kubernetes-backed tasks**.
The current UI embeds VS Code with its integrated terminal in the task page. An authorized requesting user
can edit existing selected checkouts while the agent is demonstrably idle. During
pending or active agent work, approval/input waits, or uncertain ownership,
repositories are read-only and the terminal is disabled. Starting work stops the
writable editor before any executable work reaches the worker; the browser
reconnects automatically. Leaving or refreshing the task page requests editor
shutdown. Saved files and task branches survive recreation.

Docker is implemented. Local and Kubernetes editor backends are still required;
until each backend is implemented and qualified, it returns an explicit
unavailable result. Viewer access, collaboration, a standalone terminal,
previews, and editor toolchain composition remain deferred.

This first delivery requires one active platform process coordinating task editor
transitions. That deployment restriction is a proposal to validate before enabling
the feature. Independent agentctl workers continue through platform restarts.
There is no new editor database storage or planned worker/ACP contract change.

The first mergeable increment was PR 1, the Docker feasibility spike. It is an
engineering deliverable, not the complete executor-support goal. Overall
completion requires Docker, Local, and Kubernetes implementations plus their
acceptance evidence. A Docker-only rollout must be described as partial support.

## Review and merge strategy

Use a sequential stack of focused PRs. Each PR names its prerequisite, contains
its own relevant checks, and updates the main plan's progress table. Keep unrelated
cleanup out of the stack. Split further if implementation reveals a change with
an independent invariant or review concern; do not impose an arbitrary line limit.

The user subsequently required the editor feature to always be enabled. The
service, lifecycle API and supported executor capability are therefore always
wired; no enable/disable flag remains. Unsupported executors return an explicit
unavailable result. Stop and reconcile editors before downgrading handoff
enforcement. Always exposing the feature does not qualify unresolved production
isolation concerns; see the [review](task-editor-review.md).

| PR | Review focus | Depends on | Existing slice |
| --- | --- | --- | --- |
| 1 | Docker image and feasibility evidence | — | 1 |
| 2 | Existing task storage and authorized checkout discovery | 1 | 2 |
| 3 | Docker editor resource lifecycle | 2 | 3 |
| 4 | Access mode policy and per-task coordination | 3 | 2 |
| 5 | Execution handoff and restart reconciliation | 4 | 2 |
| 6 | Browser session and HTTP/WebSocket gateway | 5 | 3 |
| 7 | Lifecycle API and generated client | 6 | 3 |
| 8 | Task actions and reconnecting launch page | 7 | 3 |
| 9 | Full qualification and operator documentation | 8 | 6 |

## PR 1 — Prove Docker editing and recreation

**Scope:** pin a code-server image and its required tooling, verify downloaded
artifacts, and add a reproducible isolated Docker/browser harness. Use existing
test checkouts with multiple repositories. Exercise a minimal authenticated proxy
and separate-origin reconnect flow to validate feasibility; keep spike plumbing
outside production routes. Record exact versions, commands and observed behavior.
Do not publish images or change agent images/runtime pins.

**Acceptance:** real Docker and a browser demonstrate Git inspection, editing,
terminal execution, and multi-root workspace access. Read-only mounts reject writes
to files and Git metadata; read-only mode disables terminals, extension installation
and automatic tasks. Recreation terminates a long-running command and its descendants,
preserves saved/untracked files and branches, and reconnects the browser. Record what
happens to unsaved buffers. Repository mounts exclude agent state and credentials.

**Exit decision:** record whether enforced mode changes, complete terminal shutdown,
and authenticated reconnect work. Resolve the browser-origin/session approach and
UID/GID assumptions before production implementation. Failed or unavailable real
checks leave this PR's feasibility acceptance incomplete; mocks do not replace it.

## PR 2 — Resolve existing task files without preparing repositories

See the [detailed implementation plan](task-editor-pr2-plan.md) for code findings,
proposed file boundaries, implementation order and acceptance checks.

**Scope:** extract only the task storage/path helpers needed by Docker editing.
Separate pure path calculation and existing-checkout inspection from directory
creation, checkout preparation and credential setup. Resolve selected registrations
server-side; expose only their existing authorized directories, including required
Git metadata. Validate resolved paths and symlinks and produce a temporary multi-root
workspace manifest. Missing checkouts yield an explanatory result, never a clone.

**Anchors:** Docker `taskDirectories`, `checkout.Preparer`, task repository selection.
The Docker scope is currently SHA-256 of the JSON array
`[organizationID, workspaceID, taskID]`, under `tasks/<hash>/work`; repository paths
are `repositories/<provider>/<repositoryID>`. Preserve this exact layout and keep
agent state separate. Avoid refactoring Local/Kubernetes solely for future reuse.

**Acceptance:** compatibility fixtures preserve existing paths; unauthorized,
retained-but-deselected and escaping paths cannot be mounted. Inspection does not
create directories, change branches or mutate repository content. Because agent
storage code is touched, verify a persisted session restores and accepts a follow-up,
preserving its untracked file and runtime/model identity.

## PR 3 — Implement the Docker editor backend

**Scope:** introduce the minimal consumer-owned editor backend contract and Docker
implementation using the existing Engine client/network configuration. Implement
start, inspect/discover and concrete-instance stop, deterministic scoped names,
requesting-user metadata, generations, repository-list compatibility, readiness,
mount modes, temporary writable home/cache, resource limits and private connectivity.
Keep editor and agent termination identities independent. No public routes yet.

**Acceptance:** real Docker tests verify both mount modes, duplicate-resource
handling, discovery after client/service recreation, ownership mismatch rejection,
saved-file retention, and confirmed terminal/process termination. Stale cleanup
cannot remove a replacement. No agent credentials, runtime home, host credential
mounts, Docker socket or privileged access reach the editor. Private connectivity
follows the supported deployment topology validated in PR 1.

## PR 4 — Implement mode policy and serialized editor transitions

**Scope:** add the small task-editor use case with task mutation authorization,
requesting-user attachment, lifecycle/status results and a per-task coordinator.
Derive mode through a consumer-owned execution-state reader backed by existing
durable state and reconciled worker ownership. Revoke generation access, stop and
confirm termination, then replace. Recheck execution state before granting writable
access; pending starts take precedence. Wire the editor capability unconditionally.

**Acceptance:** race tests cover duplicate Open, Open/Stop, concurrent transitions,
stale generations and repository selection changes. A task stage, missing output,
heartbeat timeout or disconnected worker never establishes idle. Pending/queued
work, approval/input waits, and unresolved ownership remain read-only. Failed
termination cannot produce a replacement writable environment. Keep production
entry points unavailable until PR 5 connects every work-delivery path.

## PR 5 — Enforce handoff on all execution paths and restart

**Scope:** integrate the coordinator with initial starts, restored/new sessions,
existing-worker follow-ups, prompt outbox retries, queue resume, workflow dispatch,
stage integration actions and recovery of accepted work. Persist work through the
existing path, revoke writable access, confirm editor termination, and only then
deliver executable work. Read-only replacement readiness does not delay the agent.
Return to writable access only after queues drain and ownership is resolved.

Trace `LaunchService.deliverPrompt`, `TaskQueue`, launch/connection recovery,
`task_dispatcher.go`, `integration_dispatch.go` and worker
`conversation_queue.go` / `conversation_commands.go`. The worker starts accepted
queued commands independently; stopping new platform sends alone cannot establish
idle. Document the evidence that no accepted command can execute before allowing
writable access. If existing state/protocol cannot establish that boundary, keep
write access unavailable and revise this plan explicitly before adding a worker
contract or storage change.

Startup must discover and reconcile editors before delivering work or admitting
writable opens. Task removal paths, including imported-task removal, stop and
reconcile the editor before associated storage cleanup. Preserve independent agent
workers and existing reservation, claim, acknowledgement and outbox identities.

**Acceptance:** real PostgreSQL plus race tests cover Open/Start races, consecutive
prompts, worker queues, approval/input waits, lost acknowledgements, uncertain stop,
and restart at each transition boundary. A writable terminal never overlaps agent
execution. Real Docker verifies the stop-before-delivery ordering. Run pinned
OpenCode restore and successful follow-up; session reload alone is insufficient.
Unsupported-executor regression coverage confirms existing launches remain unchanged.

## PR 6 — Secure browser access and proxying

**Scope:** implement separate-origin editor sessions, short-lived launch grants,
and the authenticated HTTP/WebSocket gateway. Bind access to task scope, requesting
user and resource generation. Resolve upstreams server-side, strip platform
credentials, validate Origin, expire/revoke access, and close existing connections
on transitions. No UI or general-purpose proxy abstraction.

**Acceptance:** HTTP and WebSocket tests deny Viewers, other users, cross-workspace
access, expired/replayed grants, obsolete generations and unapproved origins.
An already-connected client loses access on transition or authorization revocation;
an old connection cannot attach to a replacement. A real browser verifies static
assets, WebSockets and reconnect with the proposed cookie/origin configuration.
Platform credentials never reach code-server; grants and terminal content stay out
of ordinary logs.

## PR 7 — Expose lifecycle API and generate its client

**Scope:** add scoped GET/POST/DELETE editor endpoints with server-derived mode,
availability/status and stable launch URL. Enforce mutation authority and CSRF
protection on lifecycle mutations. Update platform OpenAPI and regenerate the
frontend client in this PR. Document ownership/Stop behavior using existing policy;
resolve any new role decision before exposing that operation. Agentctl API stays
unchanged.

**Acceptance:** contract/handler tests cover open/reconnect/stop, missing checkouts,
unsupported executors, backend failure, and authorization. Responses contain no
private endpoint, credentials or browser-controlled host path. GET remains inspection;
opening creates no agent run or workflow transition. Generated-client checks pass.

## PR 8 — Add task actions and reconnecting launch page

**Scope:** add Open VS Code and Stop editor actions, mode/lifecycle status, and a
stable launch page that reconnects after recreation. Show starting, switching,
ready, stopping, failed and unavailable states. Explain terminal interruption,
saved-file persistence and possible unsaved-buffer loss. Keep Start agent available.
Closing the browser tab leaves the environment running.

**Acceptance:** browser journeys cover idle editing, opening during agent work,
starting with a writable editor, read-only reconnect, return to writable after work
drains, explicit Stop and actionable failures. Verify the separate-tab launch and
reconnect against a real backend/code-server as well as UI fixtures. No extra user
confirmation is introduced for agent start.

## PR 9 — Qualify and document the first release

**Scope:** close cross-component acceptance gaps and document editor image/config,
separate origin/networking, Docker paths and UID/GID, single-coordinator restriction,
ownership policy, toolchain differences, retained data, upgrade and rollback.
Update requirements/architecture/executor references/status to match implemented
evidence, preserving confirmed/proposed distinctions. Do not defer relevant tests
from earlier PRs into this PR.

**Acceptance:** complete every applicable scenario in the main plan's verification
matrix against real Docker, PostgreSQL and a browser, including restart with queued
work and terminal descendants. Run `make check`, applicable browser checks, the
PostgreSQL concurrency suite with race detection, and `make test-recovery` with the
pinned runtime/model prerequisites. Go changes in every PR require `make lint`
(use `make fmt` for formatting); a successful `make check` includes lint.

Record exact commands, environment and limitations in PR/task evidence. Add a
focused editor Docker test command if needed so mount/terminal checks do not
unnecessarily require a model; that command does not replace live continuation.
Missing infrastructure/credentials keep the associated qualification incomplete.

Before enablement, stop old platform writers: they cannot enforce editor handoff.
Reconcile discovered resources before delivering work. Before rollback, revoke access and confirm all writable editors have terminated while the
new handoff guards still operate. Do not stop independent agentctl workers merely
to upgrade the platform. Publishing/deployment needs separate authorization.

## Planning evidence and next action

Before PR 1, reviewed the existing editor plan, requirements, tech stack,
implementation status, testing guide, delivery ownership rules, executor storage/checkout code, prompt
delivery and worker queue code. That review was documentation planning only;
no feature, Docker, browser, database or runtime qualification had been run at that point.

PR 1 is now implemented and verified by the isolated Docker/browser harness.
See [exact evidence and exit decision](task-editor-pr1-evidence.md) and
[harness instructions](../../tools/task-editor/README.md). All Slice 1 checkboxes
are verified; production feature checkboxes remain unchecked. The spike requires
a separate immutable read-only PTY image variant; preserve equivalent enforcement
in PR 3.

Next action: PR 2 existing task storage and authorized checkout discovery, including
its required persisted-session restore and successful follow-up compatibility test.
No production wiring, routes or UI have been enabled.

## Revised backend delivery

The user requested the remaining backend as one PR. Planned PRs 3–7 are therefore
combined without changing their execution-safety or authorization requirements.
See [backend implementation and operator notes](task-editor-backend.md). PR 8
frontend and PR 9 release qualification remain separate. Older progress prose
above describes the original sequential breakdown; the main plan's current
handoff and the backend notes describe the implemented state.
