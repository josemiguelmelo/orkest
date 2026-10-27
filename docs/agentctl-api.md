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
| `POST /agent/restore` | Load the configured durable session. |
| `GET /agent/commands/{commandID}` | Reconcile durable command status. |
| `POST /agent/queue/resume` | Explicitly resume paused pending prompts. |
| `POST /agent/queue/discard` | Discard paused pending prompts and end the step. |
| `POST /agent/launch` | Explicitly start the configured provider. No body. |
| `POST /agent/prompt` | Submit a prompt with a stable `commandId`. |
| `POST /agent/cancel` | Request cancellation of the active prompt. |
| `POST /agent/permissions/respond` | Answer a pending permission request. |
| `GET /agent/stream` | Receive readiness and activity events. |

Each process serves one run. Supply `taskSessionId` to enable durable session
restoration and FIFO follow-up prompts. Several runs can reuse the session.
Creation remains a single attempt; restoration is explicit and never falls back
to a fresh conversation. See [task sessions](task-sessions.md).
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

- `prompt_accepted`, `prompt_started`, `prompt_continued`, `update`, `prompt_completed`, `prompt_failed`, `prompt_interrupted`
- `session_restored`, `runtime_stopped`, `queue_paused`, `queue_resumed`, `queue_discarded`, `run_completed`, `run_failed`
- `permission_requested`, `permission_response`, `cancel_requested`

Reconnect with `?afterSequence=LAST_PROCESSED_SEQUENCE` to receive later events.
Durable sessions replay older committed events from disk when the cursor precedes
the 4096-event memory window. Legacy configurations reject expired cursors; future
or otherwise invalid cursors return `400 invalid_cursor`. Slow
subscribers are disconnected and can reconnect. Client or platform disconnects do
not stop accepted work while `agentctl` remains alive.

Application errors use `errorCode` and a sanitized `error` message; configuration
validation may also return `field`. Use `errorCode` for client logic. The OpenAPI
contract lists the possible codes.
