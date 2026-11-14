# Database migrations

- Platform startup applies pending Goose migrations from [SQL](../../db/migrations/);
  independent agentctl workers do not migrate the platform database.
- Before rollback: back up database/configuration, stop affected workers, reconcile executions.
- Rollback neither stops independent agents nor restores deleted files/external images.

## Upgrade and rollback index

Documented hazards below; SQL is the complete inventory.

| Area | Migrations |
| --- | --- |
| [Earlier schemas/repository links](#earlier-schemas-and-repository-links) | Initial runner, 0023 |
| [Sessions/direct conversations](#sessions-and-direct-conversations) | 0018, 0032 |
| [Coding integrations](#coding-integrations) | 0020, 0022 |
| [Execution profiles](#execution-profiles) | 0025 |
| [Numbering repair](#migration-numbering-repair) | 0026 |
| [Workflows](#workflows) | 0035, 0036 |
| [Artifacts](#artifacts) | 0037 |
| [Workflow delivery claims](#workflow-delivery-claims) | 0038 |
| [Workspace host volumes](#workspace-host-volumes) | 0040 |

## Earlier schemas and repository links

- Initial Goose migration replays the custom runner schema. A follow-up removes only
  old `schema_migrations` tracking after Goose records its version; application data remains.
- `0023_task_repositories.sql`: component links → repository links; drops component catalog.
  Rollback reconstructs one component per repository, losing original IDs/descriptions.

## Migration numbering repair

- Duplicate `0023_gitlab_coding_providers.sql` became `0026_gitlab_coding_providers.sql`;
  0023 belongs to task repositories.
- 0026 changes constraints, preserving registrations; rollback retains the unrestricted
  provider state established by 0020's whitelist removal.
- GitLab-only version-23 installations must reconcile history/task-repository schema.
  Never blindly relabel Goose versions; verify with PostgreSQL migration tests.

## Sessions and direct conversations

- **0018:** preserves runs without inventing session IDs. Active-task uniqueness fails
  on overlapping active runs; reconcile before upgrade. Rollback drops session
  metadata/prompt outbox, retaining task files. Host/disk loss remains outside guarantees.
- **0032:** adds direct-conversation identities to sessions. Rollback deletes direct
  conversations and their sessions/launch history; running agents remain alive.

## Coding integrations

- **0020** (`0020_coding_integrations.sql`): workspace integration storage/repository links.
  Rollback drops integration table and repository integration identity, display name,
  visibility, default branch. Export registrations first if needed.
- **0022** (`0022_unique_coding_integration_provider.sql`): one provider/workspace;
  resolve duplicates before applying.

## Execution profiles

- **0025:** profiles/run image metadata. Rollback drops profiles, builds, agent selections,
  run image metadata; retains images/task files. Stop profile execution and export configuration first.
- Numbering repair: [0026](#migration-numbering-repair).

## Workflows

- **0035:** relational workflows on schema 34.
- **0036:** booleans → explicit-destination rules; pins initial/closed stages; moves
  pending outcomes to durable inbox. Preserves queued-start IDs, tasks, files, sessions, agents.
- Deploy writers/workers together: workflow API/configuration shape changes.
- 0036 rollback deletes event audit and approximates rules with booleans, losing
  nonadjacent destinations/separate manual policies. Back up configuration; drain events/stop workers.
- Superseded branch schemas 0035–0039 require backup, applied-schema identification,
  SQL comparison, reviewed migration plan. Never relabel Goose/reset populated databases to bypass reconciliation.

## Artifacts

- **0037:** artifact tables, run input links, stage-binding JSON; preserves tasks/runs/workflows.
- Rollback permanently deletes content, revisions, input links, bindings. Export needed outputs first.

## Workflow delivery claims

- **0038:** adds `claim_token`, `claim_until`, `claim_attempt` and `delivery_error`;
  preserves command JSON, task/run/session identity, events and working files.
- Stop all old platform workers before upgrading; mixed old/new delivery binaries
  are unsupported. Independent agentctl processes continue during platform downtime.
- An expired claim grants reconciliation only, never replacement execution.
  `delivery_uncertain` requires an explicit user retry after reviewing run state.
- Rollback refuses to drop metadata while pending claimed commands exist. Stop
  workers and reconcile claims/runs first; dropping claim metadata removes delivery
  diagnostics. Neither upgrade nor rollback terminates agentctl or releases its ownership.
- Protocol and crash cases: [delivery ownership](../development/workflow-delivery.md).

## Stage custom prompts

- **0039:** adds an empty-by-default custom prompt to workflow stages, preserving
  existing workflow and session behavior.
- Rollback permanently removes saved stage prompts. Export needed prompts first.
  Existing reserved prompt outbox entries remain intact.

## Workspace host volumes

- **0040:** adds workspace bind-mount selections with cascading workspace deletion.
  No existing workspace, task, file, or session is changed.
- New sessions retain mount snapshots in their existing configuration JSON.
  Legacy sessions without that field remain mount-free. No worker protocol or
  native runtime storage format changes are needed.
- Home-relative `~/` targets require a matching platform version that resolves
  private session homes. Older mount-aware versions only support `/mnt/host` and
  cannot continue these snapshots. The native session storage layout is unchanged.
- Do not run older platform binaries against sessions created with host mounts:
  old readers discard unknown snapshot fields and can recreate containers without
  their required mounts. Drain/stop affected runs and preserve/export session
  configurations before downgrade; use the matching platform version to continue
  mounted sessions.
- Rollback drops saved workspace selections, not host files or independent
  containers. It does not erase snapshots from session JSON or unmount active
  containers. Export selections before rollback; stop affected containers to
  revoke their host access.

## Workflow stage integrations (0048–0050)

Main already includes `0047_scheduled_jobs.sql`. Workflow-stage integration
migrations therefore start at 0048; gaps before 0047 are retained to preserve
merged migration identities. These replace the unmerged branch's former
0042–0044 filenames. Development databases that applied those earlier branch
versions require explicit Goose-history reconciliation before using the renamed
files; do not rerun the renamed schema migrations over the same tables. No
existing database or migration history is modified by this file renumbering.

Stop all old platform ingestion and workflow workers before deploying migrations
0048–0050 and matching platform/UI builds. Older workers cannot interpret the new
integration rule/event kinds. Independent agentctl processes continue; no worker
wire protocol or runtime session format is changed. New optional platform session
metadata records agent-definition identity for guarded integration continuation.
Legacy manual continuation remains supported; an integration cannot infer missing
legacy agent identity and fails visibly while preserving saved state.

0048 adds encrypted connection configuration, receipt/semantic deduplication,
observations and admissions. 0049 extends rules/events and adds scoped connection
foreign keys. Keep new connections disabled until configured and validated.
Webhook secrets and coding credentials are distinct; encryption must be configured.

Before rollback, stop ingestion, reconcile all pending/uncertain callback commands,
and retain an export of integration history/configuration. 0049 refuses rollback while integration rules/events or pending integration commands remain; 0048 refuses
rollback while integration events or commands are pending.
Do not remove deduplication history while redelivery remains possible. Dropping
these tables after explicit history removal loses their diagnostic/audit data.
Rollback does not cancel, replace or terminate agentctl processes.

0050 adds indexes for deterministic observation identity, subject-scoped event
selection and paginated activity. It preserves connection/event/session data;
rollback only removes these indexes. Index creation runs inside the migration
transaction and can block writes while existing rows are indexed, so apply it
with ingestion stopped as described above. Deploy the matching UI/API together:
the status endpoint now returns a bounded `{items, nextCursor}` page rather than
an unbounded array. Worker wire and native session formats remain unchanged.

0051 permits integration movement rules targeting their source stage, matching
the workflow editor's "Keep task in this stage" option. Existing rules and event
history are preserved. Internal success/completion movement rules still require
a different target. Constraint replacement validates existing rows and takes a
table lock; apply it with ingestion stopped. Rollback refuses existing same-stage
integration rules: remove them or select different targets before rolling back.
