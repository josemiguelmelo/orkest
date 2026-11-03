# Codex runtime

## Configuration

- Runtime `codex`, provider `openai`, explicit model ID.
- Docker installs pinned `@agentclientprotocol/codex-acp`; Local needs `codex-acp` on `PATH`.
  [Inventory](../../../internal/core/runtime/versions.json).
- Workspace secret: `OPENAI_API_KEY` or `CODEX_API_KEY`; headless startup disables browser login.
- Local reuses existing `~/.codex/auth.json`; `ORKEST_CODEX_AUTH_FILE` overrides.
  Docker requires an explicit daemon-visible path. Copy to private Codex home;
  refreshes never update host source.

## Runtime behavior

- Instructions/skills become developer instructions; MCP uses ACP session options.
- `ask`: `workspace-write`; `autonomous`: `agent-full-access`.
- Reserve HOME, CODEX, model-provider/auth-mode variables against caller overrides.
- Retain private Codex home/adapter state with durable task storage.

See [shared verification/recovery](README.md#verification-and-recovery).
