> Historical prototype design: the CLI and execution features below are absent.
> See [the current API and launch usage](agentctl-api.md), which pins OpenCode 2.1.4.

# Local ACP execution prototype

`agentctl` owns an OpenCode subprocess, communicates over ACP v1 stdio,
creates a session, selects the configured agent mode, and executes one prompt.
The SDK is pinned to `github.com/coder/acp-go-sdk v0.13.5`; the initial runtime
baseline is OpenCode **1.2.27**. Other executable versions fail before session
creation. See the [release](https://github.com/anomalyco/opencode/releases/tag/v1.2.27)
and [ACP SDK](https://github.com/coder/acp-go-sdk).

This is a local execution prototype. The platform does not yet launch it through
the executor or supply an authenticated reconnectable transport. Use it inside
an isolated task environment with only the credentials that task needs. The CLI
configuration is trusted resolved platform configuration, not an authorization
API. The caller must authorize organization/workspace access, supply a task-owned
working directory, and reserve the execution slot before launching it.

## Run a prompt

Install the pinned OpenCode binary into the task image or supply its absolute
path in `binary`. Put a configuration file outside version control:

```json
{
  "agent": {
    "id": "developer",
    "organizationId": "org-1",
    "workspaceId": "workspace-1",
    "name": "Developer",
    "systemPrompt": "Implement the requested task and explain the changes.",
    "runtime": "opencode",
    "model": {"provider": "YOUR_PROVIDER", "id": "YOUR_MODEL"},
    "mcpServerIDs": []
  },
  "taskId": "task-1",
  "runId": "run-1",
  "workDir": "/workspace/repositories/project",
  "stateDir": "/workspace/agentctl",
  "binary": "/usr/local/bin/opencode",
  "envNames": ["PATH", "YOUR_PROVIDER_API_KEY"]
}
```

The working directory must exist. Choose an available model and pass its required
credential variables explicitly. There is no implicit provider/model fallback.
Variables in `envNames` must exist; other host variables are not inherited.
OpenCode configuration, HOME and XDG storage variables cannot be overridden through
that list. OpenCode itself remains responsible for validating model availability.

```sh
go build -o bin/agentctl ./cmd/agentctl
bin/agentctl -config /workspace/agent-config.json -prompt-file /workspace/prompt.txt
```

`prompt.txt` contains the user's requested work. stdout emits newline-delimited
JSON task events; it is a sensitive activity channel, not an ordinary log. stderr
contains sanitized bridge errors. OpenCode stderr and raw SDK diagnostics are not
forwarded. Runtime-owned diagnostic files remain private under task storage.

The adapter creates an `orkest` primary OpenCode agent mode using the definition's
system prompt and `provider/model` selection. It selects that mode through ACP
before submitting the prompt. It leaves permissions at runtime defaults and
does not advertise client filesystem or terminal access; OpenCode owns its local
tools. User/project OpenCode configuration is disabled to keep execution tied to
the supplied definition. The task image and host-managed runtime configuration
must remain controlled by the operator. OpenCode auto-update and sharing are
disabled.

The command exits successfully only for an ACP `end_turn`. Other stop reasons,
protocol failures and runtime exits return a nonzero status. This does not change
Orkest task workflow or certify the quality of the agent's result.

## Permission responses

A `permission_requested` event contains `payload.requestId` and the complete ACP
request in `payload.request`, including `sessionId`, tool call and offered options.
An authorized user can select an offered option by sending a JSON line to stdin:

```json
{"taskId":"task-1","runId":"run-1","sessionId":"SESSION_FROM_REQUEST","requestId":"REQUEST_FROM_EVENT","optionId":"OPTION_FROM_REQUEST"}
```

Unmatched identities and unoffered options are ignored. Requests are never
automatically approved. EOF on stdin leaves requests pending; it does not select
an option or terminate the agent. SIGINT/SIGTERM intentionally cancels the active
prompt and closes the owned subprocess. Unix shutdown kills the process group
if the runtime does not exit after stdin closure. Full container termination and
external-effect reconciliation remain the executor/orchestration responsibility.

## Durable files and current recovery boundary

Mount both the working directory and state directory outside the container layer.
Storage is namespaced by a hash of organization/workspace/task identity. Its
`runtime/` directory contains dedicated HOME and XDG data/config/state/cache paths,
including OpenCode sessions. Keep these paths stable across container recreation.
Work must remain in the mounted working tree, including uncommitted/untracked work.
Different runs for a task reuse runtime storage; the caller must prevent concurrent
runs from using that task's working tree and runtime state.

`runs/<run-hash>.jsonl` is a private append-only event journal with scoped identities,
monotonic sequence numbers and timestamps. Events are synced before streaming;
startup records include runtime version, model, permission baseline, working paths,
configuration fingerprint and prompt hash. Pending requests are persisted before
being forwarded. A `permission_response` record is delivery intent, not evidence
that the agent accepted that response.

An exclusively created run journal prevents a second invocation from launching
the same run in the same durable storage. Existing journals are refused, including
after failed startup. Do not remove a journal or change run IDs to bypass uncertain
ownership. A platform disconnect neither stops the subprocess nor authorizes a
replacement. Slow/broken stdout consumers do not block durable activity: streaming
is bounded and may detach, while journal persistence continues. Full replay and
reconnection are not yet available.

Session load/resume, automatic interrupted-work continuation, acknowledgement
cursors, command replay, ownership fencing, pending-interaction reconciliation,
platform authentication/transport, and UI approval forwarding are **not implemented**.
The prototype does not satisfy requirements §7.3 yet. Retaining session files is
not proof that interrupted work can continue. No automatic retries are performed.

## Verification

```sh
make check
go test -race ./internal/agentctl ./internal/agentruntime/opencode
ORKEST_OPENCODE_BINARY=/absolute/path/to/opencode make test-integration
```

Unit tests use an actual subprocess fixture speaking ACP JSON-RPC and cover
initialization, mode configuration, prompting, streamed updates, permission round
trips, cancellation, disconnection, isolation and run duplication refusal. The real
OpenCode integration test verifies initialization/session creation/mode selection
without making a model request or requiring model credentials. It may access
OpenCode's model catalog and bootstrap dependencies. A missing executable is a
failure, not a skipped/passed integration check.

Real model execution, real-runtime permission round trips, HTTP MCP connectivity,
and restoration **and continuation** after process/container restarts still require
their own integration/recovery validation. `make test-recovery` fails explicitly.
