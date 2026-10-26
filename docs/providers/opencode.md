# OpenCode provider

This document describes the OpenCode implementation of the `agentctl` provider
interface. Provider-independent API behavior is documented in
[agentctl-api.md](../agentctl-api.md).

## Requirements and configuration

- OpenCode **2.1.4** must be installed and available as `opencode` on `PATH`, or
  supplied with `-opencode-binary` / `AGENTCTL_OPENCODE_BINARY`.
- `-work-dir` must be an existing absolute directory.
- `-state-dir` must be an absolute directory on durable storage. Retain it with
  the working tree for the run.
- Runtime and model in `RunConfig` must be `opencode` and the requested provider
  and model identifier. The selected agent definition is written as a Markdown
  agent file under the isolated `XDG_CONFIG_HOME/opencode/agents` directory.
  Its name selects the ACP primary mode and its instructions become the system
  prompt. The materialized file remains in durable runtime state for the run.

The provider starts `opencode acp` and connects over local stdio using ACP v1.
It initializes a new session in the configured working directory. OpenCode's
default permissions are retained; permission requests are passed to the user
through `agentctl`. ACP diagnostics are discarded to avoid exposing secrets.

Example configuration:

```dotenv
AGENTCTL_OPENCODE_BINARY=/usr/local/bin/opencode
AGENTCTL_WORK_DIR=/absolute/path/to/repository
AGENTCTL_STATE_DIR=/absolute/path/to/durable/state
```

By default, the provider inherits no host environment. Grant required variables
explicitly with repeatable flags, for example `-env PATH -env YOUR_PROVIDER_API_KEY`.
`.env` values can be granted this way. `HOME`, `XDG_*`, and `OPENCODE_*` variables
are reserved; the provider sets isolated HOME/XDG directories and its own
OpenCode configuration under a hash of organization, workspace, and task scope.

## Run fencing and storage

An exclusive run marker is retained under the provider's task-scoped state
directory. It prevents duplicate launch after process restart, including after a
startup failure. An existing marker requires reconciliation; do not delete it to
force a retry. The executor must prevent separate runs from sharing a working
tree concurrently.

The provider currently creates a new ACP session. Session restoration and
continuation after process/container restart are not implemented. The marker
prevents duplicate work but does not prove that previous work stopped or restore
an interrupted session; reconcile execution ownership before retrying.

## Verification

Run the real OpenCode handshake without model calls with the pinned executable:

```sh
ORKEST_OPENCODE_BINARY=/absolute/path/to/opencode make test-integration
```

This handshake does not verify real model execution, permission round trips, or
interrupted-work continuation. Those require separate validation.
