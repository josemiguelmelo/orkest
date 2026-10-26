-- +goose Up
ALTER TABLE users ADD COLUMN platform_admin boolean NOT NULL DEFAULT false;

CREATE TABLE skills (
    id text PRIMARY KEY CHECK (id ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(id) <= 64),
    scope text NOT NULL CHECK (scope IN ('global', 'organization', 'workspace')),
    organization_id text REFERENCES organizations(id) ON DELETE CASCADE,
    workspace_id text,
    skill_md text NOT NULL CHECK (length(trim(skill_md)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((scope = 'global' AND organization_id IS NULL AND workspace_id IS NULL)
        OR (scope = 'organization' AND organization_id IS NOT NULL AND workspace_id IS NULL)
        OR (scope = 'workspace' AND organization_id IS NOT NULL AND workspace_id IS NOT NULL)),
    FOREIGN KEY (organization_id, workspace_id) REFERENCES workspaces(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX skills_scope_org_idx ON skills(scope, organization_id, id);
CREATE INDEX skills_scope_workspace_idx ON skills(scope, workspace_id, id);

CREATE TABLE agent_skills (
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id text NOT NULL,
    skill_id text NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_id, skill_id),
    FOREIGN KEY (workspace_id, agent_id) REFERENCES agents(workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX agent_skills_workspace_idx ON agent_skills(workspace_id, agent_id);

-- +goose Down
DROP TABLE agent_skills;
DROP TABLE skills;
ALTER TABLE users DROP COLUMN platform_admin;
