-- +goose Up
ALTER TABLE agents
    ADD COLUMN permission_policy text NOT NULL DEFAULT 'ask'
    CHECK (permission_policy IN ('ask', 'autonomous'));

-- +goose Down
ALTER TABLE agents DROP COLUMN permission_policy;
