# HTTP lifecycle and transport contracts

Status: Accepted. Extends decisions 0007 and 0008 without changing execution ownership.

The application package shares a private HTTP lifecycle helper. Each entry point
configures its own handler/timeouts and owns its non-HTTP resources. The helper
binds the listener, serves, drains requests on cancellation, and joins Serve.
agentctl still owns stream cancellation and provider shutdown independently.

REST and WebSocket declare consumer-owned controller interfaces. REST requires
configuration, launch, prompt, cancellation, permissions, and status. WebSocket
requires only status and subscriptions. The parent composition binds both to one
RunController; it does not introduce a shared service interface or registry.

Application errors use one protocol.Error shape with required errorCode/error and
an optional configuration field name. Configuration validation never echoes values.
Known domain errors map to stable codes. Busy and interrupted are separate codes
but remain errors.Is-compatible with ErrConversation. A runtime that does not match the wired provider returns 503 during
configuration. Launch without an available provider also returns 503. Unknown startup errors
return sanitized 502 launch_failed; other unexpected errors return 500 internal_error.
Journal-opening failures explicitly match ErrJournal and remain 503 storage_unavailable.
Router and WebSocket library rejections retain their own responses.

These error codes refine the existing v1 prototype API. Clients must handle the new
busy/interrupted codes, configuration/runtime statuses, and errorCode on decoding
errors. api/agentctl.openapi.yaml and docs/agentctl-api.md document the contract.
