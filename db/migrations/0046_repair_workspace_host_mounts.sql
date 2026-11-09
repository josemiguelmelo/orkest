-- +goose Up
-- Early development databases used version 0040 for trigger integrations before
-- host mounts were merged. Their ledger does not prove this table exists.
CREATE TABLE IF NOT EXISTS workspace_host_mounts (
 workspace_id text PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
 mounts jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(mounts) = 'array')
);

-- +goose Down
-- Preserve workspace configuration; migration 0040 owns removal of this table.
SELECT 1;
