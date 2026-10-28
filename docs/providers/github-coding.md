# GitHub coding integration

GitHub Cloud is the first coding provider. Workspace Admins and inherited
organization Owners/Admins can add an integration in **Workspace settings →
Coding integrations**. Each workspace can have one integration per provider.
Members can then browse repositories from that integration and register them
from the Repositories page. Credential values are write-only; the UI confirms
storage and connection validation without showing the saved token.

Use a fine-grained personal access token scoped only to the repositories Orkest
should access. Repository discovery needs **Metadata: read-only** permission.
Clone and fetch need **Contents: read-only** permission. Push and pull request
operations are outside this checkout integration and should use a separately
scoped agent credential in a future extension.

Coding integration secrets use `ORKEST_INTEGRATION_ENCRYPTION_KEY`, a 64
character hexadecimal encoding of a 32-byte key. Keep the key in deployment
secret management and preserve it with the durable database; losing it makes
stored integration credentials unreadable. The key is shared with task
integrations, workspace secrets, MCP headers, and encrypted run snapshots.

Migration `0020_coding_integrations.sql` adds workspace-scoped integration
storage and repository references. Rolling it back drops the integration table
and repository integration identity, display name, visibility, and default
branch fields. Export integration-backed registrations before rollback if they
must be retained.

Migration `0022_unique_coding_integration_provider.sql` enforces one integration
per provider in each workspace. Resolve any existing duplicate provider
integrations before applying it.
