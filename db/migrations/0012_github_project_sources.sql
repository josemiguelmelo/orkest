-- +goose Up
ALTER TABLE task_integrations
    DROP COLUMN repositories,
    ADD COLUMN project_url text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE task_integrations
    DROP COLUMN project_url,
    ADD COLUMN repositories text[] NOT NULL DEFAULT '{}';
