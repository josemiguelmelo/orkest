-- +goose Up
-- Remove the custom runner's ledger after Goose records the existing schema migration.
DROP TABLE IF EXISTS schema_migrations;

-- +goose Down
CREATE TABLE IF NOT EXISTS schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);
