# agentctl API

- REST commands/status; receive-only WebSocket activity.
- No authentication: platform must authorize access/reserve the run before configure/launch.
- Schemas/codes: [OpenAPI](../../internal/agentctl/httpapi/openapi.yaml); startup: [agentctl](../guides/agentctl.md).

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

- One run/process; `taskSessionId` enables durable restore/FIFO follow-ups. Sessions span runs.
- Creation is single-attempt; explicit restore never falls back to a fresh conversation.
- Limits: prompt 256 KiB; JSON body 1 MiB. See [sessions](../guides/sessions.md).

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

- `202` confirms acceptance, not completion. Same `commandId`/text deduplicates;
  each explicit turn needs a new ID.
- Cancel/approval responses acknowledge delivery intent; await activity for completion.
- Approvals match task/run/session/request/offered option; never automatic.

## Durable session lifecycle

- Stable `taskSessionId`: `/agent/launch` creates; `/agent/restore` loads saved identity,
  preserving state on failure without replacement.
- `GET /info`: session, active command, queue depth, paused/ended, `recoveryBlocked`.
- **Platform** task updates accept `sessionMode: continue|new`, optional `promptText`,
  stable `commandId`; `startWork: true` requests another step after failure while in
  progress. Reconcile repeat starts against durable history. These are not worker
  configure fields; see [platform API](../../internal/app/httpapi/openapi.yaml).

## Activity stream

- URL: `ws://127.0.0.1:8081/agent/stream`; `ready`, then `event` frames; no inbound app messages.
- Event fields: sequence, timestamp, run identities, command ID. Types:

- `prompt_accepted`, `prompt_started`, `prompt_continued`, `update`, `prompt_completed`, `prompt_failed`, `prompt_interrupted`
- `session_restored`, `runtime_stopped`, `queue_paused`, `queue_resumed`, `queue_discarded`, `run_completed`, `run_failed`
- `permission_requested`, `permission_response`, `cancel_requested`

- Reconnect: `?afterSequence=LAST_PROCESSED_SEQUENCE`.
- Memory window: 4096 events. Durable sessions replay older disk records; legacy
  configurations reject expired cursors. Future/invalid cursors → `400 invalid_cursor`.
- Slow subscribers disconnect/reconnect. Uncapped event/replay response sizes can
  consume substantial memory. Disconnects never stop accepted work while agentctl lives.
- Errors: `errorCode` for client logic, sanitized `error`, optional validation `field`;
  possible codes in OpenAPI.
- Tool activity: `toolKind`, bounded `input`/`output` previews, `locations`, text or
  before/after `content`. Chat shows supplied paths/expandable previews; old title/status
  events cannot be enriched retroactively.
- Updates require rebuilt images/platform restart as needed; running binaries retain
  behavior. Check [Docker](executors/docker.md) and [compatibility](../guides/sessions.md) first.
