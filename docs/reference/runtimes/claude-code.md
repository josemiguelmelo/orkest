# Claude Code runtime

## Configuration

- Runtime `claude-code`, provider `anthropic`, explicit Claude model ID.
- Workspace secret: `ANTHROPIC_API_KEY`.
- Pinned `claude-agent-acp` bundles Claude Agent SDK; Local installs matching adapter.
  [Inventory](../../../internal/core/runtime/versions.json); avoid unpinned launchers.

## Runtime behavior

- Start `claude-agent-acp` from PATH, force ACP v1; private durable config directory.
- Instructions/skills become custom instructions; MCP endpoints/headers use session config.
- `ask`: approvals; `autonomous`: `bypassPermissions` plus required adapter option.
- Startup diagnostics are bounded/sanitized; ordinary logs exclude raw output/secrets.
- No complete replay attestation; uncertain turns pause. Real interrupted continuation is unqualified.

See [shared verification/recovery](README.md#verification-and-recovery).
