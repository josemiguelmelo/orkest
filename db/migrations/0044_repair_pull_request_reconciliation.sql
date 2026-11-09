-- +goose Up
-- Early development databases recorded migration 0043 before it included this
-- table. Leave existing reconciliation state intact on databases with the table.
CREATE TABLE IF NOT EXISTS task_pull_request_reconciliation (
 integration_id text PRIMARY KEY REFERENCES task_trigger_integrations(id) ON DELETE CASCADE,
 next_attempt timestamptz NOT NULL DEFAULT now(),
 activated_at timestamptz NOT NULL DEFAULT now(),
 configuration_version bigint NOT NULL DEFAULT 0,
 diagnostic text NOT NULL DEFAULT ''
);

-- +goose Down
-- Retain the table and its state: it is part of the migration 0043 schema and
-- that migration owns its eventual removal.
SELECT 1;
