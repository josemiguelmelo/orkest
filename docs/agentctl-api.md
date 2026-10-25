# agentctl control API

Run the standalone internal service:

```sh
go run ./cmd/agentctl -listen 127.0.0.1:8081 \
  -work-dir /workspace/project -state-dir /workspace/agentctl \
  -opencode-binary /usr/local/bin/opencode
```

`agentctl` automatically loads `.env` from the current working directory before
parsing flags. Missing `.env` is allowed; unreadable or malformed files stop startup
without printing their contents. Precedence is CLI flags, existing process
environment, `.env`, then built-in defaults.

For example, add these to `.env` and run `make dev-agentctl`:

```dotenv
AGENTCTL_LISTEN=127.0.0.1:8081
AGENTCTL_OPENCODE_BINARY=/usr/local/bin/opencode
AGENTCTL_WORK_DIR=/absolute/path/to/repository
AGENTCTL_STATE_DIR=/absolute/path/to/durable/state
```

The working directory must already exist. State directories are created at launch.
Variables loaded from `.env` are available to `-env NAME`, but are only forwarded
to the provider when explicitly granted with that flag.

The runner has no API authentication and is intended for an internal execution
environment. The default binding is loopback. The platform owns workspace
access checks and slot reservation before configuring the runner.

Configure with `POST /agent/configure`, Content-Type `application/json`:

```json
{
  "organizationId": "org-1",
  "workspaceId": "workspace-1",
  "taskId": "task-1",
  "runId": "run-1",
  "runtime": "opencode",
  "model": {"provider": "YOUR_PROVIDER", "id": "YOUR_MODEL"},
  "systemPrompt": "Implement the requested task."
}
```

Configuration starts no work and invokes no runtime. All values above are required.
Extra fields are ignored; configuration bodies have no application-level size limit. Identical configuration can be
submitted again; changes return 409. Each process serves one run. Configuration
is memory-only and must be resubmitted after restart. No credentials are accepted
in this contract, and info responses never echo configuration.

Connect to `ws://127.0.0.1:8081/agent/stream` after configuration. The first JSON message is `{"type":"ready","info":{...}}`.
Send `{"type":"ping","requestId":"1"}` for a `pong`, or
`{"type":"info","requestId":"2"}` for an `info` response with an `info` object.
Responses echo requestId. Unknown commands return `type: error` and
`error: unsupported_command`. Prompt, cancellation and permission commands are
described below.
Messages have no application-level size limit, and extra fields are ignored.
Malformed JSON and binary messages close the stream. Default WebSocket origin
checks apply. Reconnecting sends a fresh readiness snapshot and replays events; disconnecting
does not clear configuration or stop an active prompt. SIGINT/SIGTERM closes the server and its WebSockets.

The [OpenAPI contract](../api/agentctl.openapi.yaml) is separate from the browser
API because this service has its own lifecycle and internal execution boundary.
`/info` reports Go/OS/architecture, startup time, configuration state, provider
availability, launch/conversation state (`starting`, `ready`, `running`,
`waiting_for_permission`, `failed`, `interrupted`, `exited`) and session ID.
Availability means a provider is wired, not that installation/credentials passed checks.

After configuration, explicitly send `POST /agent/launch` (no body). Success returns
200 with an info snapshot after ACP initialization, session creation and mode
selection. Missing configuration or a repeated launch returns 409; unavailable
provider returns 503; startup failure returns 502 and leaves state `failed`.
Concurrent launches cannot start two providers. Failed attempts cannot be retried
in this process. HTTP/WebSocket disconnection does not terminate the provider.
SIGINT/SIGTERM shuts down agentctl and closes its subprocess.

The OpenCode adapter requires **2.1.4**, an existing absolute working directory,
and an absolute durable state directory. Retain both working and state directories.
It stores isolated HOME/XDG data under a task scope hash, configures an `orkest`
primary mode with the configured system prompt and model, and keeps default
permissions. No host environment is inherited. Grant needed variables explicitly
with repeatable `-env PATH -env YOUR_PROVIDER_API_KEY` flags. HOME/XDG/OpenCode
configuration variables cannot be granted. Raw runtime diagnostics are discarded.

An exclusive retained run marker refuses duplicate launch after process restart,
including failed startup after reservation. Existing markers require reconciliation;
do not delete them to force retry. The executor must prevent separate runs from
sharing the same working tree concurrently. Configuration and live status remain
memory-only. MCP configuration and restoration/continuation are not implemented.
The older [prototype notes](agentctl.md) are historical and do not describe the
current CLI. See [the launch decision](decisions/0004-agent-provider-launch.md).

## Communicate over WebSocket

After configuring and launching, connect a WebSocket client to
`ws://127.0.0.1:8081/agent/stream`. Send a prompt:

```json
{"type":"prompt","requestId":"prompt-1","text":"Summarize this repository without editing files."}
```

The server returns `{"type":"accepted","requestId":"prompt-1"}` and emits
`{"type":"event","event":{...}}` frames. Activity may arrive before the command
acknowledgement. Each event includes `sequence`, `at`, organization/workspace/task/run
and session identities, and `commandId`. Event types are:

- `prompt_accepted`: the persisted user text.
- `update`: `update.kind` is `text`, `tool_call`, or `tool_call_update`; text chunks
  are in `update.text`, tool summaries in `toolCallId`, `title`, and `status`.
- `permission_requested`: includes `requestId` and a `permission` object with
  `toolCallId`, `title`, and `options` (`id`, `name`, `kind`).
- `permission_response`: records response delivery intent; `text` is the option ID.
- `cancel_requested`: cancellation delivery intent, not proof work has stopped.
- `prompt_completed`: `stopReason` from the provider, e.g. `end_turn` or `cancelled`.
- `prompt_failed`: a sanitized error; the conversation becomes interrupted.

Answer a permission using the exact identities and option ID from the event:

```json
{"type":"permission_response","requestId":"answer-1","permission":{"taskId":"task-1","runId":"run-1","sessionId":"SESSION_FROM_EVENT","requestId":"permission-1","optionId":"OPTION_FROM_EVENT"}}
```

A recorded answer returns `type: permission_response_recorded`; invalid, duplicate,
stale or unoffered answers return `type: error`. A disconnected user leaves the
request pending. Nothing is automatically approved.

Cancel the active prompt explicitly:

```json
{"type":"cancel","requestId":"cancel-1","commandId":"prompt-1"}
```

The acknowledgement is `type: cancel_requested`. Wait for the terminal prompt event;
a cancellation acknowledgement alone does not release execution ownership.

Only one prompt runs at a time. Repeating a prompt requestId with identical text
returns accepted without running it again; changing its text returns an error.
Use a new requestId for a new explicit turn. Ordinary failures never auto-retry.
This internal API does not enforce platform workflow rules or member slot allocation;
the platform remains responsible for authorizing each explicit turn.

Reconnect with `?afterSequence=LAST_PROCESSED_SEQUENCE` to replay later events,
then receive live updates. Omit the cursor to replay all events in this process.
A cursor beyond the current journal returns HTTP 400. Slow subscribers are closed
and can reconnect; they cannot block the provider. This replay works across client
or platform disconnects while agentctl stays alive.

Activity is synced to private files under `AGENTCTL_STATE_DIR/conversations` before
streaming. Keep these files with task storage; they contain sensitive task content.
An existing journal refuses a fresh prompt after restart rather than silently
replace history. Journal/session restoration, command reconciliation after an
agentctl restart, durable acknowledgement cursors, and process/container continuation
remain unimplemented. Histories are also retained in memory without compaction.
See [the conversation decision](decisions/0005-agent-provider-conversation.md).

Verify with `make check` and `go test -race ./internal/agentctl/... ./internal/agentruntime/... ./internal/app`.
The subprocess fixture covers prompts, permissions, cancellation and WebSocket
reconnects. Run `ORKEST_OPENCODE_BINARY=/absolute/path/to/opencode make test-integration`
for a real handshake without model calls. Real model execution/permissions and
requirements §7.3 recovery need separate validation.
