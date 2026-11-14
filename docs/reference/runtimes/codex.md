# Codex runtime

## Configuration

- Runtime `codex`, provider `openai`, explicit model ID.
- Docker installs pinned `@agentclientprotocol/codex-acp`; Local needs `codex-acp` on `PATH`.
  [Inventory](../../../internal/contracts/runtimecatalog/versions.json).
- Workspace secret: `OPENAI_API_KEY` or `CODEX_API_KEY`; headless startup disables browser login.
- Local reuses existing `~/.codex/auth.json`; `ORKEST_CODEX_AUTH_FILE` overrides.
  Docker requires an explicit daemon-visible path. Copy to private Codex home;
  refreshes never update host source.

## Runtime behavior

- Instructions/skills become developer instructions; MCP uses ACP session options.
- `ask`: `workspace-write`; `autonomous`: `agent-full-access`.
- Reserve HOME, CODEX, model-provider/auth-mode variables against caller overrides.
- Retain private Codex home/adapter state with durable task storage.
- Workspace GitHub integration credentials are exported as `GH_TOKEN` on launch
  and restore, overriding workspace GitHub token variables. This authenticates
  absolute `gh` paths and shells that reset `PATH`, including outside checkouts.
  Git HTTPS authentication remains configured separately through `GIT_CONFIG_*`.

The configured model must also be confirmed through ACP before prompts on both
create and restore. Unsupported confirmation preserves state and fails visibly.
Deterministic conformance is covered; live model confirmation and continuation
remain unverified for this provider. See [shared verification/recovery](README.md#verification-and-recovery)
and [runtime qualification](../../development/testing.md#runtime-qualification).
