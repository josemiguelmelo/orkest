# Local agentctl executor

One host `agentctl` process per run, for trusted single-user development.
**No isolation from host/other tasks**; use Docker for multi-user/untrusted work.

## Setup

1. [Build/start local infrastructure](../../../README.md#run-locally); `make build` creates `bin/agentctl`.
2. Configure `.env`:

```dotenv
ORKEST_EXECUTOR=local
ORKEST_AGENTCTL_BINARY=/absolute/path/to/orkest/bin/agentctl
```

3. Install [pinned runtime/dependencies](../runtimes/README.md) on host; run `make dev`.
   Docker still supplies database/Keycloak, not Local runtime images.

- agentctl inherits standard host environment plus `ORKEST_AGENT_PROVIDER_ENV` names;
  provider variables require explicit runtime grants.
- Local ignores profile images/toolchains. `ORKEST_EXECUTOR=docker` (default when
  unset) uses execution-profile images.

## Credentials

- Codex uses existing `~/.codex/auth.json`; `ORKEST_CODEX_AUTH_FILE` overrides.
  Copy into private Codex home, never expose host source directly. Without a file,
  use workspace API keys.
- Runtime inherits host `PATH` for launchers such as OpenCode's `#!/usr/bin/env node`.
  Include runtime/Node installations; with nvm, activate a [supported Node](../../../README.md#prerequisites) before `make dev`.

## Storage and lifecycle

- Worktrees/state: `ORKEST_DATA_DIR/agent-executions/tasks/<scope-hash>/{work,agentctl-state}`,
  mode 0700. Process identity records live under `agent-executions/runs`.
- Checkout only task-selected registered repositories; same credential resolution as Docker.
- Endpoint: loopback/dynamic port. Identity: PID plus observed start-time/command
  fingerprint to reject PID reuse after restart.
- Platform shutdown preserves workers; unverifiable processes are never automatically replaced.
