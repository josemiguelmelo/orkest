-- +goose Up
CREATE TABLE agents (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(trim(name)) > 0),
    runtime text NOT NULL CHECK (runtime IN ('opencode')),
    provider text NOT NULL CHECK (length(trim(provider)) > 0),
    model text NOT NULL CHECK (length(trim(model)) > 0),
    description text NOT NULL DEFAULT '',
    system_prompt text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id)
);
CREATE INDEX agents_workspace_name_idx ON agents(workspace_id, name);

-- +goose Down
DROP TABLE agents;
