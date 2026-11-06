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
