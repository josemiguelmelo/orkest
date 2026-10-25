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

The `RunConfig` model binds one execution run to its resolved scope, runtime, model,
and system prompt. Configuring it starts no work and invokes no runtime. All values above are required.
Extra fields are ignored; JSON bodies are limited to 1 MiB. Identical configuration can be
submitted again; changes return 409. A runtime that does not match the wired provider returns 503. Each process serves one run. Configuration
is memory-only and must be resubmitted after restart. No credentials are accepted
in this contract, and info responses never echo configuration.

REST endpoints own commands and status; the WebSocket owns event delivery:

| Transport | Endpoint | Purpose |
| --- | --- | --- |
| REST GET | `/health`, `/info` | Liveness and current runtime status |
| REST POST | `/agent/configure`, `/agent/launch` | Bind and explicitly launch a run |
| REST POST | `/agent/prompt` | Accept a prompt with a stable `commandId` |
| REST POST | `/agent/cancel` | Request cancellation of that command |
| REST POST | `/agent/permissions/respond` | Answer a specific pending permission |
| WebSocket GET | `/agent/stream` | Readiness, ordered replay, and live activity |

Commands do not require a WebSocket connection. HTTP JSON bodies are limited to
1 MiB (413 when exceeded); prompt text is limited to 256 KiB of UTF-8 bytes.
HTTP/WebSocket disconnection does not stop an accepted prompt. SIGINT/SIGTERM closes
the server, streams, and owned provider.

The [OpenAPI contract](../api/agentctl.openapi.yaml) is separate from the browser
API because this service has its own lifecycle and internal execution boundary.
`/info` reports Go/OS/architecture, startup time, configuration state, provider
availability, provider process liveness (`processAlive`), launch/conversation state (`starting`, `ready`, `running`,
`waiting_for_permission`, `failed`, `interrupted`, `exited`) and session ID.
Availability means a provider is wired, not that installation/credentials passed checks.
Failed/interrupted states remain visible after process exit; `processAlive` reports
liveness independently.

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
current CLI.

## Send commands through REST

After configuring and launching, send `POST /agent/prompt` with Content-Type
`application/json`:

```json
{"commandId":"prompt-1","text":"Summarize this repository without editing files."}
```

The server returns HTTP 202 with `{"status":"accepted","commandId":"prompt-1"}`.
This acknowledges durable acceptance, not completion. Activity can arrive on an
already-connected WebSocket before the HTTP response.

Connect to `ws://127.0.0.1:8081/agent/stream` to receive
`{"type":"ready","info":{...}}`, followed by `{"type":"event","event":{...}}`.
The stream accepts no application messages; sending one closes it with status 1008.
Use protocol ping/pong for connection checks and `GET /info` for current status.
Each event includes `sequence`, `at`, organization/workspace/task/run
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

Answer a permission with `POST /agent/permissions/respond`, using the exact identities
and option ID from the event:

```json
{"taskId":"task-1","runId":"run-1","sessionId":"SESSION_FROM_EVENT","requestId":"permission-1","optionId":"OPTION_FROM_EVENT"}
```

A recorded answer returns HTTP 200 with `status: permission_response_recorded` and
the permission `requestId`. Invalid, duplicate, stale or unoffered answers return 409. A disconnected user leaves the
request pending. Nothing is automatically approved.

Cancel the active prompt with `POST /agent/cancel`:

```json
{"commandId":"prompt-1"}
```

The acknowledgement is HTTP 202 with `status: cancel_requested` and the `commandId`. Wait for the terminal prompt event;
a cancellation acknowledgement alone does not release execution ownership.

Only one prompt runs at a time. Repeating a prompt commandId with identical text
returns accepted without running it again; changing its text returns an error.
Use a new commandId for a new explicit turn. At most 4096 distinct prompt identities
are accepted per process; reaching the cap rejects new identities with `command_limit`.
Existing identities remain deduplicated using SHA-256 prompt hashes. Ordinary failures never auto-retry.
Application errors from REST and pre-upgrade stream validation use one JSON shape:

```json
{"errorCode":"invalid_configuration","error":"model.id is required","field":"model.id"}
```

`errorCode` and sanitized `error` are always present. `field` appears only for
configuration validation and names the missing field; values are never echoed.
Clients should branch on `errorCode`, not the human-readable message.

| HTTP | Error codes |
| --- | --- |
| 400 | `invalid_configuration`, `invalid_command`, `invalid_json`, `unreadable_body`, `invalid_cursor` |
| 409 | `configuration_conflict`, `not_configured`, `launch_conflict`, `invalid_permission`, `conversation_unavailable`, `conversation_busy`, `conversation_interrupted`, `command_limit` |
| 413 | `body_too_large` |
| 415 | `unsupported_media_type` |
| 500 | `internal_error` |
| 502 | `launch_failed`, `cancellation_failed` |
| 503 | `provider_unavailable`, `storage_unavailable` |

Busy means another prompt is active; interrupted requires reconciliation rather
than automatic retry. Both still match the domain `ErrConversation` category.
Unknown startup failures return a sanitized `launch_failed`; other unexpected
service failures return `internal_error` rather than being mislabeled as storage
failures. Router-generated errors and WebSocket upgrade/protocol rejections are
owned by Chi/the WebSocket library and do not use this application error shape.

This internal API does not enforce platform workflow rules or member slot allocation;
the platform remains responsible for authorizing each explicit turn.

Reconnect with `?afterSequence=LAST_PROCESSED_SEQUENCE` to replay later events,
then receive live updates. Omit the cursor to replay all events in this process.
Replay retains at most 4096 events and 8 MiB of serialized JSON. A cursor beyond
the current journal or older than this window returns HTTP 400 with
`errorCode: invalid_cursor`; it never silently omits activity. Omitting the cursor
works only while the full history fits in that window. Older events remain on disk;
there is no disk-history API yet. Slow subscribers are closed
and can reconnect; they cannot block the provider. This replay works across client
or platform disconnects while agentctl stays alive.

Activity is synced to private files under `AGENTCTL_STATE_DIR/conversations` before
streaming. Keep these files with task storage; they contain sensitive task content.
An existing journal refuses a fresh prompt after restart rather than silently
replace history. Journal/session restoration, command reconciliation after an
agentctl restart, durable acknowledgement cursors, and process/container continuation
remain unimplemented. Each serialized event is capped at 1 MiB; an oversized event interrupts the
conversation before publication. Disk journal retention remains unlimited; only
the in-memory replay window is bounded.

Verify with `make check` and `go test -race ./internal/agentctl/... ./internal/agentruntime/... ./internal/app`.
The subprocess fixture covers prompts, permissions, cancellation and WebSocket
reconnects. Run `ORKEST_OPENCODE_BINARY=/absolute/path/to/opencode make test-integration`
for a real handshake without model calls. Real model execution/permissions and
requirements §7.3 recovery need separate validation.

The earlier bidirectional WebSocket command protocol is removed: move prompt
`requestId` to REST `commandId`, remove command `type` fields, and send permission
identity fields directly in the REST request body. Command acknowledgements are
HTTP responses; readiness and activity remain WebSocket frames.

Implementation modules live in `internal/agentctl/httpapi/rest` (commands/status)
and `internal/agentctl/httpapi/websocket` (event delivery). The parent `httpapi`
package composes their routes; `httpapi/protocol` holds shared wire representations.
Both transports declare consumer-owned controller interfaces: REST consumes
commands and status; WebSocket consumes status and subscriptions. The parent
composes them with the same `RunController` and startup timestamp. The controller
composes a `ConversationService` for prompts, permissions, journaling, and
subscriptions; process launch and shutdown remain with the controller.
