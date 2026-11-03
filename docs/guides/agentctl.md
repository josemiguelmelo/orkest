# agentctl

Independent worker owning one provider process; REST commands/status and receive-only
WebSocket activity. See [API](../reference/agentctl-api.md) / [OpenAPI](../../internal/agentctl/httpapi/openapi.yaml).

## Run

```sh
go run ./cmd/agentctl -runtime opencode -listen 127.0.0.1:8081 \
  -work-dir /workspace/project -state-dir /workspace/agentctl
```

- Install the [pinned runtime](../reference/runtimes/README.md); `-runtime` is mandatory,
  and mismatched run configurations are rejected.
- `make dev-agentctl` defaults to OpenCode. Select `AGENTCTL_RUNTIME=<runtime>`;
  additional flags: `AGENTCTL_ARGS="-work-dir /absolute/project -state-dir /absolute/state"`.
- Work directory must exist; retain it and durable state directory.
- Optional current-directory `.env`; precedence: CLI → process environment → `.env` → defaults.
- Host provider variables require repeatable `-env NAME` grants; otherwise not inherited.
- Unauthenticated API defaults to loopback. Platform must authorize workspace access
  and reserve task runs first; member capacity remains unfinished.

## Runtime and recovery

- Private JSONL activity: `AGENTCTL_STATE_DIR/conversations`, synced before publication.
  Contains task content; never overwrite journals; retain with task storage.
- Durable sessions retain runtime/configuration/commands/queues/checkpoints across restart.
  Without `taskSessionId`, legacy one-run behavior has no automatic daemon recovery.

References: [replay/limits](../reference/agentctl-api.md#activity-stream), [session controls](sessions.md),
[persistence](../development/sessions.md), [ownership](../development/agentctl.md),
[testing](../development/testing.md), [live qualification](../reference/runtimes/README.md#verification-and-recovery).
