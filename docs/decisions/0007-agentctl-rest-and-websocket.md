# Agentctl REST commands and WebSocket activity

Status: Accepted. Supersedes the bidirectional WebSocket command protocol in
[decision 0005](0005-agent-provider-conversation.md) and command dispatch details in
[decision 0006](0006-agentctl-composition-and-limits.md).

REST owns configuration, launch, prompts, cancellation, permission responses, and
current status. POST /agent/prompt accepts commandId/text; POST /agent/cancel accepts
commandId; POST /agent/permissions/respond accepts the existing task/run/session/
request/option binding directly. Prompt and cancellation return 202; a recorded
permission response returns 200. Error status codes and stable errorCode values
are documented in the separate agentctl API contract.

GET /agent/stream upgrades to a receive-only WebSocket. It emits readiness, replay,
and live events, supports protocol control frames, and rejects all client
application messages with close status 1008. Application ping/info requests are
removed; clients use protocol ping/pong and GET /info. The stream uses the same
cursor, persistence-before-delivery, bounded replay, and slow-subscriber behavior.

Commands and stream connections share the same agentctl RunController; handlers remain
thin and no transport owns accepted execution. A prompt may run without any stream
and may emit events before its HTTP acknowledgement arrives. A cancellation
acknowledgement is not proof of termination; permission acknowledgement is delivery
intent. Existing idempotency and approval bindings remain enforced by the service.

This is a breaking change to the internal prototype API. Prompt requestId becomes
commandId in the REST body; permission identity fields are no longer nested in a
WebSocket envelope. No compatibility command dispatcher remains on the stream.
The split adds no platform authorization or process/container recovery.

Transport code is organized into separate Go packages under internal/agentctl/httpapi:
rest owns command/status handlers and routes; websocket owns the stream lifecycle
and route. Neither imports the other. The parent httpapi package composes both
against the same service and startup timestamp. A small protocol package shares
wire representations and JSON responses without owning routes or business rules.
The repository retains its single root Go module.
