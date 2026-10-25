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

## Temporary platform launch endpoint

For local manual testing, start the platform with
`ORKEST_ENABLE_TEMP_AGENT_LAUNCH=true`. This enables authenticated
`POST /api/temp/agent-launch`, which creates a Docker agentctl container,
configures and starts an OpenCode session, then submits the first prompt. It
does not perform task or workspace authorization and is not a production task
start flow. Keep the platform bound to loopback while this temporary endpoint is
enabled.

Build the runtime image with `make docker-agentctl-images`. Set
`ORKEST_AGENT_PROVIDER_ENV` to a comma-separated allowlist of host environment
variable names that the container needs, such as `OPENAI_API_KEY`; values are
forwarded only for names in that list. Use a registered provider/model supported
by the configured OpenCode credentials.

Request body:

```json
{
  "config": {
    "organizationId": "local",
    "workspaceId": "local",
    "taskId": "manual-test",
    "runId": "manual-test-1",
    "runtime": "opencode",
    "model": {"provider": "YOUR_PROVIDER", "id": "YOUR_MODEL"},
    "systemPrompt": "Help test this agent."
  },
  "commandId": "prompt-1",
  "prompt": "Summarize the workspace."
}
```

The endpoint returns `202` when the prompt is accepted. Inspect the launch row
and `agent_launch_events` table for lifecycle and activity. The temporary flow
does not prepare a repository checkout; the Docker work directory starts empty.

## Runtime and recovery

- Providers implement the [agentctl provider interface](../internal/agentctl/provider.go).
  [OpenCode](providers/opencode.md) is the currently wired provider.
- Activity is synced to private JSONL files under
  `AGENTCTL_STATE_DIR/conversations` before publication. These files contain task
  content; retain them with task storage. Existing journals are not overwritten.
- WebSocket replay retains at most 4096 events. Event size is uncapped, so large
  events can use substantial memory. Older events remain on disk, but there is no
  disk-history API.
- Configuration, live status, and acknowledgement cursors are not persisted.
  Journal/session restoration, command reconciliation, and process/container
  continuation after an `agentctl` restart are not implemented. A client or
  platform disconnect does not stop work while `agentctl` remains alive.

## Verification

Run `make check` for local checks. The OpenCode handshake command and its limits
are in the [provider guide](providers/opencode.md). Real model execution,
permission round trips, and recovery continuation need separate validation.
