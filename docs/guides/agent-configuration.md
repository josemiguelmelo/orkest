# Configure an agent

Definitions are workspace configuration, not team-member capacity. Viewers cannot edit them.

## Create a definition

1. Set name, description, system instructions.
2. Choose [runtime](../reference/runtimes/README.md), provider, and model ID directly or via runtime profile.
3. Add required credentials as workspace secrets.
4. Select visible skills and same-workspace HTTP or stdio MCP servers.
5. Choose permission policy and optional Docker execution profile.
6. Assign to a task/stage; assignment never starts work.

| Resource | Contains | Updating it |
| --- | --- | --- |
| Runtime profile | Runtime, model provider/ID, provider environment settings | Atomically updates linked definitions; existing sessions keep their configuration |
| Execution profile | Runtime and toolchain image | Builds a new image for later runs; active queues retain their image |
| Agent definition | Instructions, resolved runtime settings, skills, MCP selection, permissions | Applies changed session settings to new sessions |

- One runtime profile per definition; deletion clears the link, retaining resolved settings.
- Reject profile updates conflicting with the execution profile.
- API: workspace `/agent-runtime-profiles`; role checks: platform OpenAPI.

## Permissions, skills, and tools

| Setting | Rules |
| --- | --- |
| Permissions | `ask`: forward approvals; `autonomous`: native noninteractive mode. Sandbox varies; Local has no task isolation. [Policy conflict](../status.md) remains open |
| Skills | Globally unique lowercase ID, matching frontmatter `name`, nonempty `description`, Markdown instructions; snapshot selected `skillMD`; no companion files |
| Skill scope | Global: platform admins; organization/workspace: respective administrators. Scope determines visibility |
| MCP | HTTP: absolute HTTP(S) URL and optional encrypted headers. Stdio: absolute executable, literal arguments, optional encrypted environment. Omit credentials to preserve, empty map to clear. Transport is immutable. Delete removes selections. OAuth setup remains deferred |

Built-in task/repository/artifact tools are supplied separately.

## Environment and session changes

- New sessions: workspace values first; agent/runtime-profile values override matching names.
- Secrets/provider configuration: encrypted at rest, write-only. Host values require
  grants; runtime-reserved names cannot be overridden.
- Continued sessions retain runtime, model, instructions, skills, permissions, and MCP
  transport/endpoints or command/arguments/environment names. Re-resolve current MCP
  credential values by ID; missing required stdio variables fail visibly.
- Current workspace environment overrides matching saved values; absent names retain
  saved values. **Removing an entry does not guarantee removal from an existing session.**
- Every profile must confirm its configured model through ACP before a prompt.
  Unsupported confirmation fails startup or restore; live support for each
  provider must be qualified separately. [Qualification prerequisites](../development/testing.md#runtime-qualification).
- New sessions use current configuration. Missing credentials/incompatibility fail
  visibly; never substitute a model or conversation.

See [sessions](sessions.md) and [key management](../operations/storage-and-secrets.md).

## Stdio MCP servers

Choose **Stdio** in workspace MCP settings. Enter an absolute executable path such
as `/opt/mcp/server` and a JSON argument array such as `["--root", "/workspace"]`.
Arguments are passed literally, without shell or variable expansion. Put tokens in
**Environment variables**, an encrypted, write-only JSON map. The server runs under
the runtime's task working directory; arbitrary per-server directories are not supported.

Install the executable and dependencies in the agent's execution image/profile
(or provision them in the Kubernetes image). Local execution runs on the trusted
host without task isolation. Orkest does not install packages when a session starts.
Runtime child processes may inherit the runtime environment; per-server environment
settings are overrides, not a separate credential sandbox. Only select trusted servers.

Catalog edits apply to new sessions. Continued sessions keep their original command,
arguments and environment names, refreshing values for those names. Adding a variable
requires a new session; removing a required variable blocks continuation. Transport
changes require registering a new server. Both API contracts require an explicit
`http` or `stdio` transport.

Upgrade both platform and agentctl binaries/images before selecting stdio servers;
older workers do not understand the transport. Saved MCP configurations without an
explicit transport cannot continue; restoration fails visibly and preserves their
session and working state. Migration `0041` preserves HTTP rows and adds stdio columns.
Rollback refuses while stdio rows exist: explicitly remove those registrations first.
Never downgrade workers responsible for stdio sessions; keep their durable state.

Session snapshots retain stdio environment variable names with empty placeholder
values because those snapshots are plaintext JSON. Credentials remain encrypted in
the MCP catalog and are resolved by server ID before launch/continuation. The worker
receives the actual values. Compatibility fingerprints likewise exclude credential
values so rotation does not invalidate a session, while retaining names to detect
changes to its required environment.
