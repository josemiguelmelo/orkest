# OpenCode runtime

## Configuration

- Runtime `opencode`; explicit provider/model ID; credentials via workspace secrets or granted variables.
- Executable `opencode`, pinned in [inventory](../../../internal/core/runtime/versions.json).

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

Loads a real session; qualifies neither inference nor credentials.
See [shared verification/recovery](README.md#verification-and-recovery).
