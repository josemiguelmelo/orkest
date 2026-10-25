# Initial agentctl control API

Status: Accepted for the initial API slice. Execution and recovery are deferred.

agentctl runs as a standalone HTTP service with `/health`, `/info`,
`/agent/configure` and `/agent/stream`. Use the already pinned coder/websocket
v1.8.15 implementation with Chi v5.2.3 on net/http. The platform browser API stays separate.

The runner is an internal service without API authentication, as directed by the
project owner. Its default listen address is loopback. The platform owns workspace
authorization and slot reservation. Configuration binds one process to one
organization/workspace/task/run and is immutable, except for identical
resubmissions. It neither reserves a platform slot nor starts execution.

Configuration is deliberately memory-only for this API slice. The stream provides
readiness snapshots, info and ping responses; unsupported commands cannot start
work or answer approvals. Execution availability is explicitly false. Runtime
integration must add durable acceptance, events and ownership reconciliation before
execution commands are exposed. A reconnect does not grant execution ownership.

`internal/agentctl.Service` owns validation, atomic run binding and status without
transport dependencies. `internal/agentctl/httpapi` owns Chi routing,
JSON handling, process metadata and the WebSocket protocol. `internal/app` constructs
both explicitly and manages the HTTP lifecycle. Service tests exercise business
rules directly; transport tests verify the API contract and stream behavior.

See the [API contract](../../api/agentctl.openapi.yaml) and
[usage and limitations](../agentctl-api.md). Earlier local-prototype notes do not
establish that the absent prototype packages are implemented by this API.
