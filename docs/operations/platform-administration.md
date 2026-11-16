# Platform administration overview

The platform-admin overview is available at `/platform-admin` to users with
platform-administrator status. Both API endpoints enforce that status on the
server for every request.

## Analytics

- Enabled accounts are all provisioned user rows. The current account model has no
  disabled state, so all persisted accounts are enabled.
- Organizations and workspaces are exact current row counts.
- Runs are `agent_launches` created during the rolling 30 days before the query.

The overview reads these four values in one database statement and does not load
account, organization, workspace, task, or event records. The run count uses the
`agent_launches(created_at)` index added in migration 0056, so the time-window
count reads only index entries in range. Exact lifetime counts remain proportional
to the number of rows and may need maintained counters if installations grow to
very large catalogs. The browser caches results for one minute and provides a
manual refresh.

## Runtime configuration

The panel displays effective agentctl runtime images, executor, resolved built-in
agent MCP URL and source, absolute data directory, and Codex auth source when
configured. It reports the process startup load time. Values are read-only; secret
contents and signing keys are never included.
