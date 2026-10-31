-- +goose Up
CREATE TABLE agent_runtime_profiles (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(trim(name)) > 0),
    runtime text NOT NULL CHECK (runtime IN ('opencode', 'claude-code', 'codex')),
    model_provider text NOT NULL CHECK (length(trim(model_provider)) > 0),
    model_identifier text NOT NULL CHECK (length(trim(model_identifier)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, name)
);
ALTER TABLE agents ADD COLUMN runtime_profile_id text;
ALTER TABLE agents ADD CONSTRAINT agents_runtime_profile_fk
    FOREIGN KEY (workspace_id, runtime_profile_id)
    REFERENCES agent_runtime_profiles(workspace_id, id) ON DELETE SET NULL (runtime_profile_id);
CREATE INDEX agent_runtime_profiles_workspace_name_idx ON agent_runtime_profiles(workspace_id, name);

-- +goose Down
ALTER TABLE agents DROP CONSTRAINT agents_runtime_profile_fk;
ALTER TABLE agents DROP COLUMN runtime_profile_id;
DROP TABLE agent_runtime_profiles;
