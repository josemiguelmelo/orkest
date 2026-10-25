-- +goose Up
CREATE TABLE code_repositories (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider_name text NOT NULL CHECK (provider_name IN ('github')),
    url text NOT NULL CHECK (length(trim(url)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, provider_name, url)
);
CREATE INDEX code_repositories_workspace_idx ON code_repositories(workspace_id, id);

-- +goose Down
DROP TABLE code_repositories;
