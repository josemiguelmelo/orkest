-- name: ListTasks :many
SELECT t.id, t.workspace_id, t.workflow_id, t.title, t.description, t.source_provider, COALESCE(t.source_id, '') AS source_id, COALESCE(t.source_url, '') AS source_url, t.status, COALESCE(t.assignee_agent_id, '') AS assignee_agent_id, t.labels, t.created_at, t.updated_at, t.stage_version, t.stage_work_state, t.workflow_snapshot, COALESCE(t.status = t.workflow_snapshot->'stages'->-1->>'id',false)::boolean AS closed
FROM tasks t JOIN workspaces w ON w.id = t.workspace_id
WHERE w.organization_id = $1 AND t.workspace_id = $2
ORDER BY t.created_at DESC, t.id;

-- name: GetWorkspaceOrganization :one
SELECT organization_id FROM workspaces WHERE id = $1;

-- name: GetTask :one
SELECT t.id, t.workspace_id, t.workflow_id, t.title, t.description, t.source_provider, COALESCE(t.source_id, '') AS source_id, COALESCE(t.source_url, '') AS source_url, t.status, COALESCE(t.assignee_agent_id, '') AS assignee_agent_id, t.labels, t.created_at, t.updated_at, t.stage_version, t.stage_work_state, t.workflow_snapshot, COALESCE(t.status = t.workflow_snapshot->'stages'->-1->>'id',false)::boolean AS closed
FROM tasks t JOIN workspaces w ON w.id = t.workspace_id
WHERE w.organization_id = $1 AND t.workspace_id = $2 AND t.id = $3;

-- name: InsertTask :one
INSERT INTO tasks (id, workspace_id, workflow_id, title, description, status, assignee_agent_id, labels)
SELECT $1, w.id, $3, $4, $5, $6, NULLIF($7, ''), $8 FROM workspaces w WHERE w.id = $2
RETURNING id, workspace_id, workflow_id, title, description, source_provider, COALESCE(source_id, '') AS source_id, COALESCE(source_url, '') AS source_url, status, COALESCE(assignee_agent_id, '') AS assignee_agent_id, labels, created_at, updated_at, stage_version, stage_work_state, workflow_snapshot, COALESCE(status = workflow_snapshot->'stages'->-1->>'id',false)::boolean AS closed;

-- name: UpdateTask :one
UPDATE tasks t SET title = $4, description = $5, status = $6, assignee_agent_id = NULLIF($7, ''), labels = $8, updated_at = now()
FROM workspaces w
WHERE w.organization_id = $1 AND t.workspace_id = w.id AND t.workspace_id = $2 AND t.id = $3
RETURNING t.id, t.workspace_id, t.workflow_id, t.title, t.description, t.source_provider, COALESCE(t.source_id, '') AS source_id, COALESCE(t.source_url, '') AS source_url, t.status, COALESCE(t.assignee_agent_id, '') AS assignee_agent_id, t.labels, t.created_at, t.updated_at, t.stage_version, t.stage_work_state, t.workflow_snapshot, COALESCE(t.status = t.workflow_snapshot->'stages'->-1->>'id',false)::boolean AS closed;

-- name: UpdateTaskIfStatus :one
UPDATE tasks t SET title = $5, description = $6, status = $7, assignee_agent_id = NULLIF($8, ''), labels = $9, updated_at = now()
FROM workspaces w
WHERE w.organization_id = $1 AND t.workspace_id = w.id AND t.workspace_id = $2 AND t.id = $3 AND t.status = $4
RETURNING t.id, t.workspace_id, t.workflow_id, t.title, t.description, t.source_provider, COALESCE(t.source_id, '') AS source_id, COALESCE(t.source_url, '') AS source_url, t.status, COALESCE(t.assignee_agent_id, '') AS assignee_agent_id, t.labels, t.created_at, t.updated_at, t.stage_version, t.stage_work_state, t.workflow_snapshot, COALESCE(t.status = t.workflow_snapshot->'stages'->-1->>'id',false)::boolean AS closed;

-- name: DeleteTaskRepositories :exec
DELETE FROM task_repositories WHERE workspace_id = $1 AND task_id = $2;

-- name: InsertTaskRepository :exec
INSERT INTO task_repositories (workspace_id, task_id, repository_id) VALUES ($1, $2, $3);

-- name: ListTaskRepositories :many
SELECT r.id, r.provider_name, COALESCE(NULLIF(r.display_name, ''), r.url) AS name, r.url
FROM code_repositories r JOIN task_repositories tr ON tr.repository_id = r.id AND tr.workspace_id = r.workspace_id
WHERE tr.workspace_id = $1 AND tr.task_id = $2
ORDER BY r.provider_name, name, r.id;

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
