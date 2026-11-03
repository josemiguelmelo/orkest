# Runtime setup

- Choose runtime and model separately. Pins: [versions.json](../../../internal/core/runtime/versions.json).
- Docker installs pinned runtimes; Local requires matching host runtime/dependencies.

| Runtime | Model provider | Credential / setup |
| --- | --- | --- |
| [OpenCode](opencode.md) | Configurable | Credential required by the selected model provider |
| [Claude Code](claude-code.md) | `anthropic` | `ANTHROPIC_API_KEY` |
| [Codex](codex.md) | `openai` | Workspace API key or explicit local auth-file convenience |
| [GitHub Copilot](github-copilot.md) | `github-copilot` | `COPILOT_GITHUB_TOKEN`, or `GH_TOKEN` / `GITHUB_TOKEN` |
| [Kiro](kiro.md) | `kiro` | `KIRO_API_KEY` |

## Shared behavior

- Instructions, selected skills, HTTP MCP; ACP v1 for all profiles.
- `ask`: forward approvals; `autonomous`: native noninteractive permissions.
  Sandbox varies by runtime/executor; Local has no container isolation.
- Standalone: `-runtime`, existing absolute work directory, absolute durable state
  directory. Host variables require repeatable `-env NAME` grants; [worker setup](../../guides/agentctl.md).
- Native configuration/replay exceptions: individual runtime pages.

## Verification and recovery

- [Qualify](../../development/testing.md#runtime-qualification) model, instructions/skills,
  MCP/headers, approvals, cancellation, restore plus follow-up.
- Fixtures/loading alone do not qualify live continuation. Existing sessions keep
  model/configuration; incompatible restore fails visibly, preserving state.
- [Sessions](../../guides/sessions.md): behavior; [status](../../status.md): evidence limits.
