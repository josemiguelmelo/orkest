# agentctl

`agentctl` owns one provider process independently of the platform. It exposes
REST commands and status, plus a receive-only WebSocket for activity. See the
[API guide](agentctl-api.md) and [OpenAPI contract](../internal/agentctl/httpapi/openapi.yaml)
for request and event details.

## Run

```sh
go run ./cmd/agentctl -listen 127.0.0.1:8081 \
  -work-dir /workspace/project -state-dir /workspace/agentctl
```

- The working directory must exist; retain it and the state directory for the run.
- An optional `.env` is loaded from the current directory. Precedence: CLI flags,
  process environment, `.env`, then defaults.
- Provider environment variables must be explicitly allowed with repeatable
  `-env NAME` flags. Host variables are otherwise not inherited.
- The API has no authentication and defaults to loopback. The platform must
  authorize workspace access and reserve a member slot before configuring a run.

## Runtime and recovery

- Providers implement the [agentctl provider interface](../internal/agentctl/provider.go).
  [OpenCode](providers/opencode.md) is the currently wired provider.
- Activity is synced to private JSONL files under
  `AGENTCTL_STATE_DIR/conversations` before publication. These files contain task
  content; retain them with task storage. Existing journals are not overwritten.
- The journal retains at most 4096 events in memory; durable sessions replay older
  cursors from disk. Event size and replay response size are uncapped, so large
  events or long replay ranges can use substantial memory.
- Durable task sessions persist runtime identity, configuration, commands, queue
  state, and journal checkpoints. agentctl restores sessions after restart and
  reconciles interrupted work conservatively. See [task sessions](task-sessions.md)
  for ownership, continuation limits, secret-state handling, and API details.
- Legacy configurations without `taskSessionId` retain one-run behavior and do
  not opt into automatic daemon recovery.

## Verification

Run `make check` for local checks. The OpenCode handshake command and its limits
are in the [provider guide](providers/opencode.md). Real model execution,
permission round trips, and recovery continuation need separate validation.
