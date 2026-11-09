-- +goose Up
CREATE TABLE workspace_host_mounts (
    workspace_id text PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    mounts jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(mounts) = 'array')
);

-- +goose Down
DROP TABLE workspace_host_mounts;
