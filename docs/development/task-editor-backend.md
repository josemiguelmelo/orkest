# Task editor architecture

The task editor exposes an authenticated VS Code session for existing task files.
The editor service owns authorization, per-task serialization, access mode,
generation leases and handoff with agent execution. It does not own a database
repository or a concrete infrastructure provider.

## Ownership and launch flow

- `internal/server/taskeditor` decides whether the requesting user may open,
  inspect or stop an editor and whether its storage is read-only or writable.
- `internal/adapters/executors/editor` inspects task storage and builds a generic
  `engine.WorkloadPlan`. It owns editor images, generated workspace files,
  readiness checks and the optional authenticated forwarding workload.
- `internal/adapters/executors/engine` defines the provider-neutral workload,
  storage, network, endpoint, credential, security and lifecycle contracts.
- Docker, Kubernetes and Local engines validate those plans and implement only
  their supported infrastructure primitives. Unsupported requirements fail
  before resource creation. Engines contain no editor or agent dispatch logic.
- Executor composition selects and owns provider clients. The editor borrows the
  selected engine through `engine.InfrastructureEngine`; it does not retain an
  agent executor.

The planner uses direct private routing when an engine advertises it. Otherwise,
it requires both denied egress and platform ingress and includes an authenticated
forwarding workload in the plan. Docker supports direct routing when the platform
is attached to the configured Docker network; otherwise it publishes only the
forwarder on loopback. The editor shell remains on an egress-denied network. The
token is held by the executor for the generation and is excluded from manifests,
metadata and public lifecycle responses.

Docker is the only editor backend currently supported. Local and Kubernetes
engines support other generic plans but do not enforce the editor's full network,
container and storage requirements, so editor opens return unavailable. See the
[executor references](../reference/executors/docker.md) and
[Kubernetes qualification limits](../reference/executors/kubernetes.md).

## Storage and execution handoff

Task worktrees remain in durable task storage. The editor receives the selected
task workspace and a generated multi-root workspace file; it receives no agent
state, runtime home, credentials, configured host mounts, Docker socket or agent
environment. Read-only mode uses a separate PTY-denial image and read-only storage.
The editor does not clone repositories, change branches or prepare checkouts.

Server policy holds the task admission guard through handoff. Writable access
requires durable terminal execution state and provider inspection confirming that
the corresponding worker has stopped. Starting work revokes writable editor access
and confirms terminal shutdown before delivering executable work. Uncertain
execution or infrastructure state fails closed. A worker heartbeat, empty queue
snapshot or quiet chat does not prove it has stopped.

Docker storage admission inspects every container for mounts intersecting the
protected task root. Such a workload must have verifiable Orkest ownership and
safe mount ancestry; privileged access, unknown ownership, writable aliases or
unverifiable mount paths deny admission. This check enforces infrastructure
safety; authorization and handoff remain server-owned.

## Lifecycle, browser access and recovery

Each editor generation has a stable owner/generation identity and a durable
infrastructure manifest. Shared engine helpers record start, stop and cleanup
state; manifests contain plan fingerprints and provider handles, not plan contents
or secret values. Engines reconcile uncertain operations before retry. Uncertain
resources are not replaced or cleaned up based on a timeout or missing heartbeat.
Removed executor-specific resource formats are outside generic reconciliation.

The API is always wired. Unsupported engines return `503 editor_unavailable`.
Browser access uses a separate editor origin, a short-lived single-use grant, and
a generation-bound cookie. The gateway validates browser Origin, resolves the
upstream server-side, strips platform credentials and closes active HTTP/WebSocket
connections when the generation is revoked. Route both configured origins to the
platform listener; do not route the editor origin directly to code-server. HTTPS
is required outside loopback.

One active platform process may coordinate editors in an installation. There is
no active-active editor lock. Startup reconciles editors before resuming work
delivery. Rollback requires stopping writable editors with a version that enforces
handoff. Independent `agentctl` workers continue during platform shutdown.

## Configuration and verification

Image defaults and origins are configured with `ORKEST_EDITOR_IMAGE`,
`ORKEST_EDITOR_READONLY_IMAGE`, `ORKEST_PLATFORM_ORIGIN` and
`ORKEST_EDITOR_ORIGIN`. Build the pinned image pair locally with
`make docker-task-editor-images`; this does not publish images. Check the daemon's
data-path visibility and editor UID/GID access before using a remote Docker Engine.
Workspace editor images do not inherit an agent execution-profile toolchain.

Run `make test-task-editor-backend` with a reachable Docker daemon and
`TASK_EDITOR_BROWSER` set to a Chromium executable for real browser/network checks.
Run focused race tests and `make test-recovery` for handoff and restore coverage.
These fixtures do not establish production OIDC, PostgreSQL concurrency, or
credentialed model continuation. See [testing prerequisites](testing.md) and
[status](../status.md) for current qualification limits.
