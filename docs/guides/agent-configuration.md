# Configure an agent

Definitions are workspace configuration, not team-member capacity. Viewers cannot edit them.

## Create a definition

1. Set name, description, system instructions.
2. Choose [runtime](../reference/runtimes/README.md), provider, and model ID directly or via runtime profile.
3. Add required credentials as workspace secrets.
4. Select visible skills and same-workspace HTTP MCP servers.
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
| MCP | Absolute HTTP(S) URL; optional encrypted headers. Omit headers to preserve, empty map to clear. Delete removes selections. No stdio/OAuth setup |

Built-in task/repository/artifact tools are supplied separately.

## Environment and session changes

- New sessions: workspace values first; agent/runtime-profile values override matching names.
- Secrets/provider configuration: encrypted at rest, write-only. Host values require
  grants; runtime-reserved names cannot be overridden.
- Continued sessions retain runtime, model, instructions, skills, permissions, MCP endpoints.
  Re-resolve current MCP headers by ID.
- Current workspace environment overrides matching saved values; absent names retain
  saved values. **Removing an entry does not guarantee removal from an existing session.**
- New sessions use current configuration. Missing credentials/incompatibility fail
  visibly; never substitute a model or conversation.

See [sessions](sessions.md) and [key management](../operations/storage-and-secrets.md).
