# agentctl API

The internal API uses REST for commands and status, and a receive-only WebSocket
for activity. See the [OpenAPI contract](../internal/agentctl/httpapi/openapi.yaml) for request
schemas and response codes. For startup and operational details, see the
[agentctl guide](agentctl.md).

The API has no authentication. The platform must authorize access and reserve a
member slot before configuring or launching a run.

## Endpoints

| Method and path | Purpose |
| --- | --- |
| `GET /health` | Check process liveness. |
| `GET /info` | Read provider, process, and conversation status. |
| `POST /agent/configure` | Bind this process to one run; does not start work. |
| `POST /agent/launch` | Explicitly start the configured provider. No body. |
| `POST /agent/prompt` | Submit a prompt with a stable `commandId`. |
| `POST /agent/cancel` | Request cancellation of the active prompt. |
| `POST /agent/permissions/respond` | Answer a pending permission request. |
| `GET /agent/stream` | Receive readiness and activity events. |

Each process serves one run. Configuration is in memory, and launch is a single
attempt per process. Submit a new prompt only after the previous prompt finishes.
Prompt text is limited to 256 KiB; JSON bodies are limited to 1 MiB.

Example configuration:

```json
{
  "organizationId": "org-1",
  "workspaceId": "workspace-1",
  "taskId": "task-1",
  "runId": "run-1",
  "runtime": "opencode",
  "model": {"provider": "YOUR_PROVIDER", "id": "YOUR_MODEL"},
  "agent": {
    "name": "Implementer",
    "description": "Implements the requested task.",
    "instructions": "Follow repository guidance and implement the requested task."
  }
}
```

Example prompt:

```json
{"commandId":"prompt-1","text":"Summarize this repository."}
```

Prompt acceptance (`202`) confirms the command was accepted, not completed.
Reuse of a `commandId` with identical text is deduplicated; use a new ID for each
explicit turn. Cancellation and permission responses acknowledge delivery intent,
not runtime completion. Wait for the corresponding activity event. Permission
answers must match the task, run, session, request, and offered option. Requests
are never automatically approved.

## Activity stream

Connect to `ws://127.0.0.1:8081/agent/stream`. The server sends a `ready` frame,
then `event` frames. The stream accepts no application messages. Events include
sequence, timestamp, run identities, and command ID. Types include:

- `prompt_accepted`, `update`, `prompt_completed`, `prompt_failed`
- `permission_requested`, `permission_response`, `cancel_requested`

Reconnect with `?afterSequence=LAST_PROCESSED_SEQUENCE` to receive later events.
Without a cursor, the stream replays retained events. Replay retains up to 4096
events; invalid, expired, or future cursors return `400 invalid_cursor`. Slow
subscribers are disconnected and can reconnect. Client or platform disconnects do
not stop accepted work while `agentctl` remains alive.

Application errors use `errorCode` and a sanitized `error` message; configuration
validation may also return `field`. Use `errorCode` for client logic. The OpenAPI
contract lists the possible codes.
