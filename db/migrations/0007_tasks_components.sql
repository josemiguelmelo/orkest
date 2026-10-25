-- +goose Up
CREATE TABLE tasks (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title text NOT NULL CHECK (length(trim(title)) > 0),
    description text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'todo' CHECK (status IN ('todo', 'in_progress', 'in_review', 'done')),
    assignee_user_id text,
    labels text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, assignee_user_id) REFERENCES workspace_memberships(workspace_id, user_id)
);
CREATE INDEX tasks_workspace_created_idx ON tasks(workspace_id, created_at DESC, id);

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

CREATE TABLE task_components (
    workspace_id text NOT NULL,
    task_id text NOT NULL,
    component_id text NOT NULL,
    PRIMARY KEY (task_id, component_id),
    FOREIGN KEY (workspace_id, task_id) REFERENCES tasks(workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, component_id) REFERENCES components(workspace_id, id) ON DELETE CASCADE
);

CREATE TABLE task_comments (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    task_id text NOT NULL,
    author_user_id text NOT NULL REFERENCES users(id),
    body text NOT NULL CHECK (length(trim(body)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, task_id) REFERENCES tasks(workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX task_comments_task_created_idx ON task_comments(task_id, created_at, id);

-- +goose Down
DROP TABLE task_comments;
DROP TABLE task_components;
DROP TABLE components;
DROP TABLE tasks;
