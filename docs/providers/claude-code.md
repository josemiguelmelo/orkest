# Claude Code provider

Claude Code runs through the pinned `@agentclientprotocol/claude-agent-acp`
adapter and the shared agentctl ACP v1 host. The adapter is pinned to **0.99.0**
in the runtime inventory and Docker image. It uses the Claude Agent SDK bundled
by that package; install it with the agentctl runtime image rather than using a
host login or an unpinned `npx` invocation.

## Configuration

- Set an Anthropic API key in the workspace environment as the secret
  `ANTHROPIC_API_KEY`. Secret values are encrypted by the platform and supplied
  only to the execution environment. Do not put the key in agent prompts or
  ordinary environment values.
- Select runtime `claude-code`, provider `anthropic`, and a valid Claude model
  identifier in the agent definition. Orkest does not choose a fallback model.
- Agent instructions and selected skill contents are sent as Claude Code custom
  instructions. Selected HTTP MCP servers and their headers are sent in the ACP
  session configuration. Headers remain sensitive and are redacted from runtime
  activity.
- Claude Code's configuration directory is isolated below durable agentctl
  state. The adapter is forced to ACP v1 for compatibility with the shared Go
  ACP client.

## Process configuration

`agentctl` starts the pinned adapter using `claude-agent-acp`. The orchestrator
assigns this worker runtime `claude-code` with the mandatory `-runtime` flag;
for local development pass `-runtime claude-code`. The profile starts
`claude-agent-acp` from `PATH`. The Docker image built by
`make docker-agentctl-claude-code` contains the
Claude Code ACP adapter, and the Docker executor passes the runtime assignment.
OpenCode is packaged separately in its own runtime image.

## Recovery and qualification

When startup fails, task chat shows the failed launch stage and a bounded,
sanitized ACP or runtime startup diagnostic when available. Configured
environment values are redacted before the diagnostic is returned to the
platform; raw provider output is not exposed.

ACP session loading is implemented, but Claude Code is not yet qualified for
automatic interrupted-work continuation. The runtime profile therefore does
not attest complete replay coverage; uncertain interrupted commands remain
paused for reconciliation. Session loading and fixture tests do not prove real
model continuation.

Credentialed qualification must verify a real Claude model response, model
selection, custom instructions and skill context, HTTP MCP headers, approval
round trips, cancellation, session loading, and interrupted-work behavior with
the pinned adapter. Do not claim recovery continuation until the runtime
conformance suite establishes it.
