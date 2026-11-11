# OpenCode runtime

## Configuration

- Runtime `opencode`; explicit provider/model ID; credentials via workspace secrets or granted variables.
- Executable `opencode`, pinned in [inventory](../../../internal/contracts/runtimecatalog/versions.json).

## Runtime behavior

- Start `opencode acp`. Agent Markdown: isolated `XDG_CONFIG_HOME/opencode/agents`;
  name selects ACP primary mode, body supplies system instructions.
- Skills use project skill directory. `ask`: approvals; `autonomous`: native allow configuration.
- Reserve HOME/XDG/OPENCODE variables; retain durable runtime files.
- Configure model natively/confirm through ACP before prompts; no fallback on failure.

## Handshake check

```sh
ORKEST_OPENCODE_BINARY=/absolute/path/to/opencode make test-integration
```

With only the binary configured, this runs the handshake/create/load check and
skips model continuation. The handshake test currently selects `opencode/big-pickle`
without sending a prompt; the model still must be available. Set explicit provider/model
variables and run `make test-recovery` for actual restore plus a next model response.
See [test configuration](../../development/testing.md#real-continuation) and
[dated results](../../status.md#verification-record). A passing handshake qualifies
neither inference nor credentials.
See [shared verification/recovery](README.md#verification-and-recovery).

## Stdio MCP

The profile writes native `mcp.servers` entries using `type: "local"`, a command
array containing the executable and arguments, and `environment` overrides. Other
profiles receive stdio configuration through ACP on both creation and load. The
runtime owns the subprocess and protocol; the platform does not launch MCP servers.
See [configuration](../../guides/agent-configuration.md#stdio-mcp-servers) and the
live qualification limits in [status](../../status.md). The no-model
`TestOpenCodeStdioMCPHandshake` integration test checks real initialization, restore
with credential rotation, and graceful MCP shutdown; it does not qualify tool use.

The native remote-server configuration sets `oauth: false`. This disables
OpenCode's default OAuth discovery/login and uses Orkest's configured HTTP headers
(or an unauthenticated endpoint). It is an OpenCode adapter option, not a domain
OAuth feature; OAuth setup remains unsupported. See the
[OpenCode MCP authentication documentation](https://opencode.ai/docs/mcp-servers/#oauth).
