# Provider-independent agentctl launch

Status: Accepted for the launch increment; execution and recovery validation remain open.

The agentctl service owns the Provider and Process interfaces. Application wiring
injects OpenCode; a different provider can implement the same interfaces without
changing service or transport code. ACP types stay inside the OpenCode adapter.
Runtime identity is separate from model provider and model identity.

POST /agent/launch explicitly starts the configured provider, negotiates ACP v1,
creates a session and selects the configured primary mode. It submits no prompt.
Configure and WebSocket connections never start work. Startup uses a bounded
service-owned context so a request disconnect cannot abandon process ownership.
Only agentctl shutdown closes its provider; platform shutdown/disconnection does not.

Reserve launch atomically before I/O. A failed attempt remains failed and is not
retried automatically. A private exclusive run marker in retained task storage
also prevents relaunch of that run after restart. This is conservative fencing,
not durable command reconciliation or recovery. Operators must not delete markers
to bypass uncertain ownership. The executor must prevent concurrent runs using
the same task storage and working tree.

Pin OpenCode 2.1.4 and ACP Go SDK v0.13.5. This supersedes the earlier
1.2.27 pin in ADR 0002; real-runtime compatibility with 2.1.4 remains unverified.
Runtime permissions remain at defaults. No prompting or approval delivery is
exposed yet; unexpected permission calls fail without approval. HOME/XDG paths
are task-scoped; environment grants are explicit and diagnostics are discarded.

Subprocess fixture tests validate protocol and lifecycle behavior. Real OpenCode
is not installed in this development environment; real handshake, model/prompt
configuration, approvals, MCP and restoration/continuation remain unverified.
This increment supersedes the absent executable prototype described in ADR 0002;
it does not establish the recovery acceptance scenarios.
