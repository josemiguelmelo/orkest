-- +goose Up
CREATE TABLE coding_integrations (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(trim(name)) > 0),
    provider_name text NOT NULL,
    provider_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    encrypted_credential text NOT NULL CHECK (length(encrypted_credential) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, name)
);

ALTER TABLE code_repositories DROP CONSTRAINT code_repositories_provider_name_check;
ALTER TABLE code_repositories ADD COLUMN integration_id text;
ALTER TABLE code_repositories ADD COLUMN provider_repository_id text;
ALTER TABLE code_repositories ADD COLUMN default_branch text NOT NULL DEFAULT '';
ALTER TABLE code_repositories ADD COLUMN display_name text NOT NULL DEFAULT '';
ALTER TABLE code_repositories ADD COLUMN is_private boolean NOT NULL DEFAULT false;
ALTER TABLE code_repositories ADD CONSTRAINT code_repositories_integration_fk
    FOREIGN KEY (workspace_id, integration_id)
    REFERENCES coding_integrations(workspace_id, id) ON DELETE RESTRICT;
ALTER TABLE code_repositories ADD CONSTRAINT code_repositories_integration_identity_unique
    UNIQUE (workspace_id, provider_name, provider_repository_id);

-- +goose Down
-- Rollback is data-destructive when repositories have been registered through
-- coding integrations. Export those registrations before reverting this schema.
ALTER TABLE code_repositories DROP CONSTRAINT code_repositories_integration_identity_unique;
ALTER TABLE code_repositories DROP CONSTRAINT code_repositories_integration_fk;
ALTER TABLE code_repositories DROP COLUMN default_branch;
ALTER TABLE code_repositories DROP COLUMN display_name;
ALTER TABLE code_repositories DROP COLUMN is_private;
ALTER TABLE code_repositories DROP COLUMN provider_repository_id;
ALTER TABLE code_repositories DROP COLUMN integration_id;
ALTER TABLE code_repositories ADD CONSTRAINT code_repositories_provider_name_check CHECK (provider_name IN ('github'));
DROP TABLE coding_integrations;
