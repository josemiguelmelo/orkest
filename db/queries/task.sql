-- name: ListTasks :many
SELECT t.id, t.workspace_id, t.title, t.description, t.source_provider, COALESCE(t.source_id, '') AS source_id, COALESCE(t.source_url, '') AS source_url, t.status, COALESCE(t.assignee_agent_id, '') AS assignee_agent_id, t.labels, t.created_at, t.updated_at
FROM tasks t JOIN workspaces w ON w.id = t.workspace_id
WHERE w.organization_id = $1 AND t.workspace_id = $2
ORDER BY t.created_at DESC, t.id;

-- name: GetWorkspaceOrganization :one
SELECT organization_id FROM workspaces WHERE id = $1;

-- name: GetTask :one
SELECT t.id, t.workspace_id, t.title, t.description, t.source_provider, COALESCE(t.source_id, '') AS source_id, COALESCE(t.source_url, '') AS source_url, t.status, COALESCE(t.assignee_agent_id, '') AS assignee_agent_id, t.labels, t.created_at, t.updated_at
FROM tasks t JOIN workspaces w ON w.id = t.workspace_id
WHERE w.organization_id = $1 AND t.workspace_id = $2 AND t.id = $3;

-- name: InsertTask :one
INSERT INTO tasks (id, workspace_id, title, description, status, assignee_agent_id, labels)
SELECT $1, w.id, $3, $4, $5, NULLIF($6, ''), $7 FROM workspaces w WHERE w.id = $2
RETURNING id, workspace_id, title, description, source_provider, COALESCE(source_id, '') AS source_id, COALESCE(source_url, '') AS source_url, status, COALESCE(assignee_agent_id, '') AS assignee_agent_id, labels, created_at, updated_at;

-- name: UpdateTask :one
UPDATE tasks t SET title = $4, description = $5, status = $6, assignee_agent_id = NULLIF($7, ''), labels = $8, updated_at = now()
FROM workspaces w
WHERE w.organization_id = $1 AND t.workspace_id = w.id AND t.workspace_id = $2 AND t.id = $3
RETURNING t.id, t.workspace_id, t.title, t.description, t.source_provider, COALESCE(t.source_id, '') AS source_id, COALESCE(t.source_url, '') AS source_url, t.status, COALESCE(t.assignee_agent_id, '') AS assignee_agent_id, t.labels, t.created_at, t.updated_at;

-- name: DeleteTaskComponents :exec
DELETE FROM task_components WHERE workspace_id = $1 AND task_id = $2;

-- name: InsertTaskComponent :exec
INSERT INTO task_components (workspace_id, task_id, component_id) VALUES ($1, $2, $3);

-- name: ListTaskComponents :many
SELECT c.id, c.workspace_id, c.identifier, c.name, c.repository_id, c.description, c.created_at, c.updated_at
FROM components c JOIN task_components tc ON tc.component_id = c.id AND tc.workspace_id = c.workspace_id
WHERE tc.workspace_id = $1 AND tc.task_id = $2
ORDER BY c.identifier, c.id;

-- name: ListTaskComments :many
SELECT c.id, c.workspace_id, c.task_id, c.author_user_id, c.body, c.created_at
FROM task_comments c JOIN tasks t ON t.id = c.task_id AND t.workspace_id = c.workspace_id JOIN workspaces w ON w.id = t.workspace_id
WHERE w.organization_id = $1 AND c.workspace_id = $2 AND c.task_id = $3
ORDER BY c.created_at, c.id;

-- name: InsertTaskComment :one
INSERT INTO task_comments (id, workspace_id, task_id, author_user_id, body)
SELECT $1, t.workspace_id, t.id, $4, $5 FROM tasks t JOIN workspaces w ON w.id = t.workspace_id
WHERE w.organization_id = $2 AND t.workspace_id = $3 AND t.id = $6
RETURNING id, workspace_id, task_id, author_user_id, body, created_at;

-- name: ListComponents :many
SELECT c.id, c.workspace_id, c.identifier, c.name, c.repository_id, c.description, c.created_at, c.updated_at
FROM components c JOIN workspaces w ON w.id = c.workspace_id
WHERE w.organization_id = $1 AND c.workspace_id = $2
ORDER BY c.identifier, c.id;

-- name: InsertComponent :one
INSERT INTO components (id, workspace_id, identifier, name, repository_id, description)
SELECT $1, w.id, $3, $4, $5, $6 FROM workspaces w WHERE w.id = $2
RETURNING id, workspace_id, identifier, name, repository_id, description, created_at, updated_at;

-- name: UpdateComponent :one
UPDATE components c SET identifier = $4, name = $5, repository_id = $6, description = $7, updated_at = now()
FROM workspaces w
WHERE w.organization_id = $1 AND c.workspace_id = w.id AND c.workspace_id = $2 AND c.id = $3
RETURNING c.id, c.workspace_id, c.identifier, c.name, c.repository_id, c.description, c.created_at, c.updated_at;
