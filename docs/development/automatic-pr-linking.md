# Automatic PR linking

This feature is on a development branch with no existing associations or deployed
observation data. It uses one current configuration and observation format. There
are no manual-link fallbacks, data-conversion migrations, or compatibility paths
for earlier iterations of this feature.

## Responsibilities

- Checkout preparation writes credential-free registered repository identities.
- The Git scanner reads branches from checkouts, clones and worktrees within the
  task directory. It does not mutate Git or control branch names.
- `repositoryobserver.Compare` calculates changes without mutating inputs or
  invoking persistence. Initial branches are ineligible; newly observed non-default
  branches become eligible for linking and retain eligibility as their commits change.
- The run coordinator scans before a prompt, periodically during it, and after
  explicitly stopping the polling loop. It journals changes, flushes successful
  snapshots once per batch, then records the accepted observer state.
- Task-level snapshots retain baselines across runs. Recovery overlays journal
  evidence and flushes it once. Failed writes fence execution; diagnostics retain
  the last successful baseline. These are recovery guarantees for the current
  implementation, rather than compatibility with a previous feature version.
- The webhook service verifies provider signatures, durably enqueues events, then
  resolves each exact integration/event identity.
- Ingestion chooses an initial status in the domain service. Its repository uses
  one transaction to check the expected configuration version and enqueue the
  entire batch idempotently. It does not bind tasks or invoke domain callbacks.
- PR association uses existing automatic ownership or unambiguous repository,
  branch and commit evidence. It validates workspace repository registration, task
  repository selection and stage timing, then commits the association and delivery
  binding with concurrency guards.
- Trigger processing owns rule evaluation. Existing workflow services own task
  movement and execution through the durable workflow outbox.

## Multiple repositories and recovery

A GitHub trigger integration has no repository list; its provider configuration is
`{}`. The same URL and secret can receive organization-wide GitHub webhooks.
Association ignores events whose provider repository identity is not registered in
that integration's workspace, then requires a task association before processing.
Polling uses task-selected registered repositories and their workspace coding
credentials. Stored lists from the retired `repositoryIds` setting are ignored;
the editor drops retired settings on save. No data conversion is required. There
is no manual association creation endpoint. Links can be listed and unlinked;
unlink suppresses automatic recreation for that integration and PR.

Missing branch evidence remains queued for five-second retries for up to 24 hours.
Conflicting evidence blocks association. Accepted automatic ownership survives
branch deletion and subsequent reviews. GitHub polling recovers missed webhook
notifications using each repository's own workspace coding credentials, with
bounded pagination and a 24-hour recovery horizon.

The schema is defined by migration 0043. Migration 0044 repairs early development
databases that recorded 0043 before its reconciliation table was added. It creates
only the missing table and preserves existing reconciliation rows. Its down step
retains the table; migration 0043 owns removal. Restart a rebuilt platform to apply
pending migrations. There is no separate configuration conversion migration.
Runtime/model/session configuration is unchanged. Unrelated runtime session
compatibility and restoration remain supported.

## Verification and limits

Behavioral coverage includes arbitrary branch names, pure comparison, failed-write
retry, missing checkouts, journal recovery, restored session follow-up, stale
configuration, duplicate deliveries, multiple repositories, suppression, stale
stages, and concurrent webhook/background association.

Current matching supports same-repository GitHub Cloud PRs. Fork heads, branches
present before their initial scan, and branches created/deleted between scans may
remain unmatched. Live GitHub delivery and credentialed runtime qualification
have not been performed for these changes.
