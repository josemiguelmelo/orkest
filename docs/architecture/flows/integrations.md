# Integrations

Coding integrations provide repository discovery and credentials. Task
integrations import source work into local tasks. They are configured separately
and synchronize idempotently within workspace scope.

```mermaid
flowchart TD
    Admin[Workspace administrator] --> Configure[Configure provider and credentials]
    Configure --> Validate[Validate provider connectivity]
    Validate --> Store[(Encrypted integration configuration)]
    Store --> Discover[Discover repositories/projects]
    Discover --> Register[Register repositories and task links]
    Register --> Checkout[Run receives scoped repository credentials]
    Checkout --> Agent[Agent creates or updates checkout]
```

```mermaid
flowchart LR
    Source[External task source] --> Poll[Task integration poll or manual sync]
    Poll --> Fetch[Provider fetches selected issues/projects]
    Fetch --> Normalize[Normalize source identity/content]
    Normalize --> Upsert[Idempotent task upsert]
    Upsert --> Local[(Local task content and workflow state)]
    Source -->|closed| Close[Move imported task to closed stage]
    Source -->|no longer matches filter| Delete[Delete imported task]
    Close --> Local
    Delete --> Local
```

- Integrations are organization/workspace scoped and are never a substitute for
  task authorization. Viewers remain read-only.
- Coding credentials are write-only in configuration APIs, encrypted at rest,
  scoped to provider hosts, and injected only into the required execution
  environment. They must not enter logs, artifacts, or Git configuration.
- Imported title, description, labels, and source URL remain source-managed;
  local stage, assignment, and execution state remain local.
- Source synchronization updates by stable source identity. Source closure
  enters the configured closed stage; leaving a selected filter deletes the
  imported task according to the integration policy.
- Provider-specific API calls and field mapping stay in adapters; task and
  workflow services consume normalized facts.
