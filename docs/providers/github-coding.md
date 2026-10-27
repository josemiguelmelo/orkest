# GitHub coding integration

GitHub Cloud is the first coding provider. Its provider implementation supports
PAT authentication, authenticated repository discovery, and credential-scoped
Git operations. Platform integration management and passing credentials to task
checkout preparation remain under implementation, so this guide is not yet an
available setup flow.

When setup is enabled, use a fine-grained personal access token scoped only to
the repositories Orkest should access. Repository discovery needs **Metadata:
read-only** permission. Clone and fetch need **Contents: read-only** permission.
Push and pull request operations are outside this checkout integration and
should use a separately scoped agent credential in a future extension.

Coding integration secrets use `ORKEST_INTEGRATION_ENCRYPTION_KEY`, a 64
character hexadecimal encoding of a 32-byte key. Keep the key in deployment
secret management and preserve it with the durable database; losing it makes
stored integration credentials unreadable. The key is shared with task
integrations, workspace secrets, MCP headers, and encrypted run snapshots.

Migration `0020_coding_integrations.sql` adds workspace-scoped integration
storage and repository references. Rolling it back drops the integration table
and repository integration identity/default branch fields. Export integration-
backed registrations before rollback if they must be retained.
