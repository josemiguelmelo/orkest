# Database migrations

- Startup applies pending Goose migrations from [SQL](../../db/migrations/).
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
