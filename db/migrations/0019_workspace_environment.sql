-- +goose Up
CREATE TABLE workspace_environment (
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (name ~ '^[A-Za-z_][A-Za-z0-9_]*$'),
    value text NOT NULL,
    is_secret boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, name)
);

-- +goose Down
DROP TABLE workspace_environment;
