# Kandev agentctl reference

Inspected upstream commit
[`d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620`](https://github.com/kdlbs/kandev/tree/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620)
as an architectural reference. This is a source review, not a test of Kandev's
runtime or recovery guarantees.

## Process and transport boundaries

Kandev runs `agentctl` as a standalone HTTP service in the execution environment.
The same control API is used on the host and in containers; its caller handles
executor differences. A control server creates/manages agent instances. Each
instance has a process manager, a protocol adapter and an API server.

```text
Kandev backend
  → HTTP instance/process control + bidirectional WebSocket agent stream
    → agentctl process manager
      → ACP adapter / typed Go SDK
        → agent subprocess stdin/stdout (OpenCode: opencode acp)
```

The instance API separates process start/stop from ACP initialize, session/new,
session/load, prompt, cancel and permission responses. A prompt is accepted
asynchronously; completion is a separate event. Its execution context is
independent of the WebSocket connection context, so a disconnected backend does
not implicitly cancel the prompt.

Sources: [entry point](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/cmd/agentctl/main.go),
[control server](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/internal/agentctl/server/api/control_server.go),
[instance routes](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/internal/agentctl/server/api/server.go),
[prompt acceptance and dispatch](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/internal/agentctl/server/api/agent.go).

## ACP and activity

An `AgentAdapter` interface separates transport from process ownership. It
supports connecting pipes, initialization, new/load session, prompts, cancellation,
normalized update events and a permission handler. Agent command/model discovery
is separate from protocol communication. Its ACP implementation uses coder's Go
SDK and queues notifications, with ordering barriers before completion events.

Permissions are forwarded through a handler. Missing permission wiring returns a
cancelled outcome rather than selecting an offered option. OpenCode is marked as
requiring process termination because its internal HTTP/MCP process tree may stay
alive after stdin closes.

Sources: [adapter contract](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/internal/agentctl/server/adapter/adapter.go),
[ACP implementation](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go),
[permission forwarding](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_permissions.go),
[OpenCode definition](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/internal/agent/agents/opencode_acp.go).

## Delivery and recovery mechanisms

Kandev has a bounded bbolt delivery journal holding scoped streams, sequence and
acknowledgement positions, immutable prompt submissions and their outcomes. The
API exposes submission inspection, retained-event replay and acknowledgements.
Unresolved delivery blocks ordinary new prompt dispatch. Recovery descriptors
describe retained work using session, controller incarnation and harness generation
identities. Native agent session state remains a separate concern; the ACP adapter
checks the negotiated session/load capability.

Sources: [journal](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/internal/agentctl/journal/journal.go),
[recovery descriptors](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/internal/agentctl/journal/recovery.go),
[ACP session lifecycle](https://github.com/kdlbs/kandev/blob/d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620/apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go).

## Implications for Orkest

The implemented OpenCode adapter already owns ACP stdio independently of the
platform, with pinned runtime configuration, explicit permissions, retained task
storage and process-group shutdown. Its subprocess tests and the real OpenCode
handshake provide an initial execution foundation.

The current single-prompt command and JSONL journal are a prototype. They do not
provide Kandev's long-lived control API, multiple prompt turns, reconnect/replay,
acknowledgements, submission reconciliation or session restoration. An existing
Orkest journal currently blocks relaunch rather than performing recovery.

Recommended next implementation, still a proposal: keep the runtime adapter and
add an authenticated, run-scoped long-lived agentctl service. Separate command
acceptance from completion, normalize activity independently of the ACP SDK,
persist command identities/outcomes and replay cursors, and reconcile ownership
before permitting new work. A single run-scoped controller is sufficient initially;
Kandev's control-server/instance-server hierarchy is not required without a
concrete need to multiplex agents in one environment.

Retain Orkest's confirmed rules when applying this reference: users explicitly
start work, no automatic permission approval, no replacement on heartbeat loss,
workspace isolation, no credentials in ordinary logs, and session restoration
plus interrupted-work continuation must both be demonstrated. Kandev's separate
local/remote survival policies and optional workflow behavior do not override
these requirements. The exact platform transport remains a design decision;
authenticated WebSocket is the existing proposed baseline.
