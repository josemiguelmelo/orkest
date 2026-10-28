---
name: database-change
description: Change Orkest PostgreSQL schema, queries, or persistence code.
---

# Database changes

- Add versioned Goose migrations in `db/migrations/`; document destructive effects.
- Keep static SQL in `db/queries/`; regenerate `internal/postgres/sqlc` with `make sqlc`—never edit generated code.
- Keep repository interfaces in owning domain packages; map sqlc types to domain types in `internal/postgres`.
- Verify with `make test-postgres` when relevant and report any blocker.
