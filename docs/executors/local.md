# Local agentctl executor

The Local executor runs one standalone `agentctl` process on the Orkest host for
each run. It is intended for trusted, single-user development. Host processes
do not isolate tasks from one another or from the host, so use Docker for
multi-user or otherwise untrusted workloads.

Select it with `ORKEST_EXECUTOR=local` and set `ORKEST_AGENTCTL_BINARY` to the
absolute path of an executable agentctl binary. The selected agent runtime and
its dependencies must already be installed and available to that process.
Only the standard host process environment and names listed in
`ORKEST_AGENT_PROVIDER_ENV` are inherited by agentctl; the configured provider
variables are granted to the runtime explicitly.

For Codex, Local automatically uses `~/.codex/auth.json` when it exists. Set
`ORKEST_CODEX_AUTH_FILE` to override that path. Agentctl copies the auth file
into the run's private Codex home; it does not expose the host file directly to
the runtime. If no local auth file exists, Codex uses the configured workspace
API key flow.

Local automatically grants the host `PATH` to the runtime so launchers such as
OpenCode's `#!/usr/bin/env node` can find Node. Start Orkest from a shell whose
`PATH` includes both the runtime and Node installations. For example, with nvm:

```sh
export PATH="$HOME/.nvm/versions/node/v22.19.0/bin:$PATH"
make dev
```

Local stores task worktrees and agentctl state under
`ORKEST_DATA_DIR/agent-executions`, with mode 0700. It checks out only the
registered repositories selected for the task, using the same credential
resolution as Docker. The agentctl endpoint binds to loopback on a dynamically
allocated port. Process identities include the PID and a fingerprint of the
observed process start time and command, allowing Orkest to reject a reused PID
after restart. Orkest shutdown leaves agentctl running. A process that cannot
be verified is not automatically replaced.

Local ignores execution profile image and toolchain selections and uses the
runtime installed on the host. Configure `ORKEST_EXECUTOR=docker` to run the
toolchain images built by execution profiles. Docker remains the default when
`ORKEST_EXECUTOR` is unset.
