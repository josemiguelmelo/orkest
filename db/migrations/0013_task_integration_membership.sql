-- +goose Up
ALTER TABLE tasks ADD COLUMN source_integration_id text REFERENCES task_integrations(id) ON DELETE CASCADE;
ALTER TABLE tasks DROP CONSTRAINT tasks_source_identity_unique;
ALTER TABLE tasks ADD CONSTRAINT tasks_source_identity_unique
    UNIQUE (workspace_id, source_provider, source_integration_id, source_id);
CREATE INDEX tasks_source_integration_idx ON tasks (source_integration_id) WHERE source_integration_id IS NOT NULL;

-- +goose Down
DROP INDEX tasks_source_integration_idx;
ALTER TABLE tasks DROP CONSTRAINT tasks_source_identity_unique;
ALTER TABLE tasks ADD CONSTRAINT tasks_source_identity_unique UNIQUE (workspace_id, source_provider, source_id);
ALTER TABLE tasks DROP COLUMN source_integration_id;
