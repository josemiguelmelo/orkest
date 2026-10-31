# Kiro CLI runtime

Orkest runs Kiro CLI 2.37.0 through its ACP V3 stdio server. The Kiro CLI
binary and its ACP server are installed in the runtime image. Kiro models are
selected by Kiro model identifier and use the fixed `kiro` model provider.
Provide a `KIRO_API_KEY` workspace secret for headless authentication.

Kiro's API key authentication is designed for automation. The CLI's browser or
device-flow login is not provisioned by Orkest. Only grant `KIRO_API_KEY` to
workspaces whose runs need Kiro access.

Orkest provisions the task instructions, selected skills, and model in a
per-session Kiro agent profile under `KIRO_HOME`. Selected HTTP MCP servers are
supplied through ACP session creation. The `ask` policy leaves Kiro's
capability permissions in confirmation mode so ACP requests reach the task UI;
`autonomous` adds an allow rule for Kiro's capabilities inside the isolated task
container.

The shared runtime host provides prompt streaming, cancellation, pending
permission handling, session persistence, and restoration. Deterministic ACP
conformance covers common interactions. Live model execution, real approval
round trips, and interrupted-turn history replay require live qualification;
Kiro replay coverage is not marked verified.
