# GitHub Copilot CLI runtime

## Configuration

- Runtime/provider `github-copilot`; model ID available to account.
- Workspace secret: `COPILOT_GITHUB_TOKEN` (alternatives: `GH_TOKEN`, `GITHUB_TOKEN`).
- Local installs [pinned executable](../../../internal/contracts/runtimecatalog/versions.json).

## Runtime behavior

- ACP stdio; instructions/skills in private durable `COPILOT_HOME`; MCP through session creation.
- `ask`: approvals; `autonomous`: `COPILOT_ALLOW_ALL`. Disable CLI auto-updates to retain pin.
- ACP integration is preview; interrupted history replay remains unqualified.

The configured model must also be confirmed through ACP before prompts on both
create and restore. Unsupported confirmation preserves state and fails visibly.
Deterministic conformance is covered; live model confirmation and continuation
remain unverified for this provider. See [shared verification/recovery](README.md#verification-and-recovery)
and [runtime qualification](../../development/testing.md#runtime-qualification).
