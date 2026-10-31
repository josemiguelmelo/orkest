# GitHub Copilot CLI runtime

Orkest runs GitHub Copilot CLI through its ACP stdio server using the shared
agentctl host. The pinned package is `@github/copilot@1.0.105`; ACP support is
currently a GitHub public preview and can change between releases.

Choose **GitHub Copilot** when creating an agent or runtime profile, enter a
model identifier available to the account, and set the provider to
`github-copilot`. Add a `COPILOT_GITHUB_TOKEN` workspace secret to authenticate
the CLI. `GH_TOKEN` and `GITHUB_TOKEN` are accepted as alternatives. Only grant
the selected credential to environments that need it.

Agent instructions and selected skills are provisioned into the isolated,
persistent `COPILOT_HOME`. Selected HTTP MCP servers are supplied in ACP session
creation. The `ask` permission policy keeps Copilot's ACP approval requests
interactive; the `autonomous` policy opts into Copilot CLI's `COPILOT_ALLOW_ALL`
mode inside the task container. The runtime disables CLI auto-updates so the
installed version remains pinned.

The shared runtime host provides prompt streaming, cancellation, pending
permission handling, session persistence, and restoration. Deterministic ACP
conformance covers common interactions. Live model execution, real approval
round trips, and interrupted-turn history replay require live qualification;
the runtime does not claim verified replay coverage until that qualification
establishes it.
