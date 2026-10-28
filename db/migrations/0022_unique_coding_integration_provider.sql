-- +goose Up
CREATE UNIQUE INDEX coding_integrations_workspace_provider_unique
    ON coding_integrations (workspace_id, provider_name);

-- +goose Down
DROP INDEX coding_integrations_workspace_provider_unique;
