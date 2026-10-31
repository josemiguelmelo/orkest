# OpenCode provider

This document describes the OpenCode implementation of the `agentctl` provider
interface. Provider-independent API behavior is documented in
[agentctl-api.md](../agentctl-api.md).

## Requirements and configuration

- OpenCode **2.1.4** must be installed and available as `opencode` on `PATH`.
- This `agentctl` worker must be assigned runtime `opencode` with
  `-runtime opencode`. The Docker executor passes this from the run configuration.
- The `-runtime` flag is mandatory at worker startup.
- `-work-dir` must be an existing absolute directory.
- `-state-dir` must be an absolute directory on durable storage. Retain it with
  the working tree for the run.
- Runtime and model in `RunConfig` must be `opencode` and the requested provider
  and model identifier. The selected agent definition is written as a Markdown
  agent file under the isolated `XDG_CONFIG_HOME/opencode/agents` directory.
  Its name selects the ACP primary mode and its instructions become the system
  prompt. The materialized file remains in durable runtime state for the run.

The provider starts `opencode acp` and connects over local stdio using ACP v1.
It initializes a new session in the configured working directory. The agent's
permission policy is applied in native OpenCode configuration. `ask` retains
the existing approval behavior and passes requests to the user through
`agentctl`; `autonomous` allows runtime actions without interactive approval.
ACP diagnostics are discarded to avoid exposing secrets.

Example configuration:

```dotenv
AGENTCTL_WORK_DIR=/absolute/path/to/repository
AGENTCTL_STATE_DIR=/absolute/path/to/durable/state
```

By default, the provider inherits no host environment. Grant required variables
explicitly with repeatable flags, for example `-env PATH -env YOUR_PROVIDER_API_KEY`.
`.env` values can be granted this way. `HOME`, `XDG_*`, and `OPENCODE_*` variables
are reserved; the provider sets isolated HOME/XDG directories and its own
OpenCode configuration under a hash of organization, workspace, and task scope.

## Run fencing and storage

An exclusive creation marker is retained under each durable session directory
(or the legacy task directory). It prevents duplicate creation after restart,
including after a startup failure. An existing marker requires reconciliation; do not delete it to
force a retry. The executor must prevent separate runs from sharing a working
tree concurrently.

Creation uses `session/new`; explicit restoration verifies the saved manifest and
uses ACP v1 `session/load` only when advertised. Runtime replay is separated from
live activity. A task ownership lock prevents concurrent processes across runs.
See [durable task sessions](../task-sessions.md) for continuation policy and limits.

## Verification

Run the real OpenCode handshake without model calls with the pinned executable:

```sh
ORKEST_OPENCODE_BINARY=/absolute/path/to/opencode make test-integration
```

This handshake does not verify real model execution, permission round trips, or
interrupted-work continuation. Those require separate validation.
