# Reset previously applied branch workflow migrations

The unmerged workflow migrations 0035–0039 have been replaced by a single final
`0035_workspace_workflows.sql`. If any superseded versions were applied locally,
return the database to v34 before starting the updated platform. Do not run the
new migration's Down through Goose against a database still tracked at v38/v39.

Stop `make dev` / the platform first and reconcile active runs. Platform shutdown
alone does not stop agentctl. The reset deletes workflow definitions, stage history,
queued workflow commands and workflow associations. Tasks and integrations remain;
custom stage keys become `todo`. Runtime sessions and working files remain.
Reapplying 0035 binds existing tasks/integrations to the seeded default workflows.

From the repository root, using the PostgreSQL service configured in `.env`:

```sh
# If the platform runs in Compose, stop it; keep PostgreSQL running.
docker compose stop orkest

# Back up the database before the reset.
docker compose exec -T postgres sh -c 'pg_dump -Fc -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > /tmp/orkest-before-workflow-reset.dump

# Roll back the schema AND its Goose tracking records atomically to v34.
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < tools/db/rollback-branch-workflows.sql

# Confirm the recorded version is 34.
docker compose exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied;"'

# Startup applies the final relational 0035 migration automatically.
make dev
```

Stop if backup or reset fails; do not proceed to startup. The reset script works
without the removed migration files, handles the superseded schemas from v35
through v39, and rejects databases with applied migrations newer than v39. It is
for this unmerged branch's local databases, not a production migration strategy.
For a Compose-hosted platform, rebuild and start the `orkest` service instead of
`make dev`.
