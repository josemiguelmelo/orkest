-- +goose Up
ALTER TABLE workflow_stages ADD COLUMN custom_prompt TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE workflow_stages DROP COLUMN custom_prompt;
