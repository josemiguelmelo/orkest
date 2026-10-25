# Initial agentctl ACP boundary

Status: Accepted for the local execution prototype. Production transport and
recovery acceptance remain open work.

`internal/agentruntime/opencode` owns the pinned OpenCode executable and typed ACP
connection. `internal/agentctl` owns scoped configuration resolution, a durable
activity journal and explicit permission-response matching. Neither package imports
Docker or depends on the platform HTTP server lifecycle. The CLI executes one
explicitly requested work step; it does not assign tasks or alter workflow status.

Select coder's ACP Go SDK v0.13.5 for typed protocol calls and ordered notifications.
Pin OpenCode 1.2.27 as the first tested runtime baseline, not a claim about the latest
release. Verify the executable version before use. Set the model and system prompt
through a dedicated OpenCode primary agent configuration, then select that mode
over ACP. This avoids treating the system prompt as an ordinary user message or
depending on an unstable ACP model-selection method. Keep runtime default
permissions and explicitly forward each permission request.

Until the platform transport is implemented, use private durable JSONL activity
and scoped JSON permission responses over stdin/stdout. An input/output disconnect
leaves execution ownership unchanged. Never replay a prompt or permission response
automatically. Refuse an existing run journal rather than infer that an old process
has stopped. This is deliberately conservative and does not implement recovery.

Runtime HOME/XDG directories are task-scoped and retained independently of the
process. Host environment variables and MCP authentication are explicit grants.
MCP references must resolve in the agent's organization/workspace. Agent/model
configuration is fingerprinted without exposing credentials to ordinary logs.

See [usage and verification boundaries](../agentctl.md). The real pinned runtime
handshake has been verified. Real-provider execution, real permissions/MCP, and
the requirements' restoration/continuation scenarios are not yet verified.
