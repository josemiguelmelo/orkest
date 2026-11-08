# Connect repositories and task sources

- Coding integrations: discovery/Git credentials; task integrations: source imports.
  Configure each separately per workspace.
- Workspace Admins/inherited organization Owners/Admins manage integrations;
  Members register repositories; Viewers read only.

## Coding integrations

1. Set the deployment [encryption key](../operations/storage-and-secrets.md).
2. Open **Workspace settings → Coding integrations**; select provider.
3. Supply configuration/PAT; validate connectivity.
4. Register repositories through the paginated picker; associate them with tasks before start.

| Provider | Configuration | Credential access needed |
| --- | --- | --- |
| GitHub Cloud | Provider `github`; no server URL | Repository metadata for discovery; contents for clone/fetch; write access for push |
| GitLab Cloud | Provider `gitlab_cloud`; hosted endpoint | API access for discovery and repository access for Git operations |
| GitLab Server | Provider `gitlab_server`; HTTPS `baseUrl` | Same operations, on the configured server |

Credentials:

- Limit PATs to intended repositories/operations. GitHub fine-grained scopes:
  Metadata read for discovery, Contents read for checkout, Contents write for push;
  PRs need corresponding permissions. Connection validation proves no push/PR access.
- GitLab needs API/repository scopes for intended operations, subject to project
  membership/server policy.
- One integration per provider/workspace. Credentials are write-only: omit to
  preserve, replace to rotate. Explicit validation needs a newly supplied credential.
- Every task run receives the workspace coding integration PATs, including tasks
  with no linked repositories. HTTPS Git authentication is scoped to each provider
  host and configured GitLab Server base path. Tasks can access repositories allowed
  by the PAT, so limit its repository access and permissions. Platform snapshots
  encrypt them; newly written worker checkpoints omit the structured credential
  list. Older checkpoints may still contain credentials. Never put them in `.git/config`.
- A restarted worker requiring Git credentials waits for matching configuration
  with fresh credentials before restoring. Automatic platform reinjection after
  reconnect is not established by the worker recovery guard; see
  [session recovery](../development/sessions.md#restore-and-ownership).
- Run `gh`/`glab` from a checkout on the matching provider host: wrappers select
  its workspace integration credential, including for agent-created checkouts.
  Workspaces without a matching coding integration lack authenticated push/CLI access.
- Commit as `Orkest Agent <agent@orkest.local>`.

## Task integrations

1. Create integration/provider; select workflow, source, optional labels, polling interval.
2. Supply source-readable PAT; validate and enable.
3. Sync manually or wait for polling; inspect last sync/errors.

| Provider | Configuration example | Read access |
| --- | --- | --- |
| GitHub Projects v2 (`github_cloud`) | `{"projectUrl":"https://github.com/orgs/example/projects/1"}` | Selected project and its issue content |
| Jira Server (`jira_server`) | `{"baseUrl":"https://jira.example.com","projects":["APP"]}` | Bearer PAT and browse access to selected projects |

- Configuration schemas: authenticated provider catalog.
- GitHub: issues/draft issues only, no PRs; **all** selected labels required.
- Jira: selected projects, `labels in (...)` matches **any** label; done status category means closed.
- Imported title/description/labels/URL are source-managed. Local workflow never writes source status.
- Source closure → workflow closed stage. Leaving source/filters → **delete imported task**;
  review filters before enabling. Updates are idempotent by source identity.

## Troubleshooting and limits

| Problem | Check |
| --- | --- |
| Credentials rejected | Validate replacement PAT; repository/project/organization policies |
| Incomplete discovery | Pagination, filters, token visibility |
| Sync failure | Sanitized error and platform→source connectivity before manual retry |

Outbound comments/status and PR webhooks are out of scope. Live private-repository
access/source sync need credentials. See [status](../status.md), [testing](../development/testing.md),
and [migrations](../operations/migrations.md).
