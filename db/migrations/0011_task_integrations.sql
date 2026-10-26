-- +goose Up
CREATE TABLE task_integrations (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider text NOT NULL CHECK (provider IN ('github', 'jira')),
    name text NOT NULL CHECK (length(trim(name)) > 0),
    enabled boolean NOT NULL DEFAULT false,
    base_url text NOT NULL DEFAULT '',
    repositories text[] NOT NULL DEFAULT '{}',
    projects text[] NOT NULL DEFAULT '{}',
    labels text[] NOT NULL DEFAULT '{}',
    interval_seconds integer NOT NULL DEFAULT 300 CHECK (interval_seconds >= 60 AND interval_seconds <= 86400),
    encrypted_credential text NOT NULL,
    last_synced_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id)
);
CREATE INDEX task_integrations_due_idx ON task_integrations (enabled, last_synced_at);

-- +goose Down
DROP TABLE task_integrations;
