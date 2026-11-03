# GitHub Copilot CLI runtime

## Configuration

- Runtime/provider `github-copilot`; model ID available to account.
- Workspace secret: `COPILOT_GITHUB_TOKEN` (alternatives: `GH_TOKEN`, `GITHUB_TOKEN`).
- Local installs [pinned executable](../../../internal/core/runtime/versions.json).

## Runtime behavior

- ACP stdio; instructions/skills in private durable `COPILOT_HOME`; MCP through session creation.
- `ask`: approvals; `autonomous`: `COPILOT_ALLOW_ALL`. Disable CLI auto-updates to retain pin.
- ACP integration is preview; interrupted history replay remains unqualified.

See [shared verification/recovery](README.md#verification-and-recovery).
