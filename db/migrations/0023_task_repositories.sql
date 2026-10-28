-- +goose Up
CREATE TABLE task_repositories (
    workspace_id text NOT NULL,
    task_id text NOT NULL,
    repository_id text NOT NULL,
    PRIMARY KEY (workspace_id, task_id, repository_id),
    FOREIGN KEY (workspace_id, task_id) REFERENCES tasks(workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, repository_id) REFERENCES code_repositories(workspace_id, id) ON DELETE RESTRICT
);

INSERT INTO task_repositories (workspace_id, task_id, repository_id)
SELECT DISTINCT tc.workspace_id, tc.task_id, c.repository_id
FROM task_components tc
JOIN components c ON c.workspace_id = tc.workspace_id AND c.id = tc.component_id
ON CONFLICT DO NOTHING;

DROP TABLE task_components;
DROP TABLE components;

-- +goose Down
-- Restores one component per repository from task repository links. Original
-- component identifiers and descriptions cannot be recovered after this migration.
CREATE TABLE components (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    identifier text NOT NULL CHECK (length(trim(identifier)) > 0),
    name text NOT NULL CHECK (length(trim(name)) > 0),
    repository_id text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, identifier),
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, repository_id) REFERENCES code_repositories(workspace_id, id)
);
CREATE INDEX components_workspace_name_idx ON components(workspace_id, name, identifier);

INSERT INTO components (id, workspace_id, identifier, name, repository_id)
SELECT id, workspace_id, id, COALESCE(NULLIF(display_name, ''), url), id
FROM code_repositories;

CREATE TABLE task_components (
    workspace_id text NOT NULL,
    task_id text NOT NULL,
    component_id text NOT NULL,
    PRIMARY KEY (task_id, component_id),
    FOREIGN KEY (workspace_id, task_id) REFERENCES tasks(workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, component_id) REFERENCES components(workspace_id, id) ON DELETE CASCADE
);

INSERT INTO task_components (workspace_id, task_id, component_id)
SELECT workspace_id, task_id, repository_id FROM task_repositories;

DROP TABLE task_repositories;
