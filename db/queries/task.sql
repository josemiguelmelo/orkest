-- name: ListTasks :many
SELECT t.id, t.workspace_id, t.workflow_id, t.title, t.description, t.source_provider, COALESCE(t.source_id, '') AS source_id, COALESCE(t.source_url, '') AS source_url, t.status, COALESCE(t.assignee_agent_id, '') AS assignee_agent_id, t.labels, t.created_at, t.updated_at, t.stage_version, t.stage_work_state, COALESCE(t.status = workflow_final_stage(t.workspace_id,t.workflow_id),false)::boolean AS closed, COALESCE((SELECT s.name FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_name, COALESCE((SELECT s.description FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_description, COALESCE((SELECT s.custom_prompt FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_custom_prompt, EXISTS(SELECT 1 FROM workflow_events e WHERE e.workspace_id=t.workspace_id AND e.task_id=t.id AND e.stage_version=t.stage_version AND e.processed_at IS NULL) AS automation_pending
FROM tasks t JOIN workspaces w ON w.id = t.workspace_id
WHERE w.organization_id = $1 AND t.workspace_id = $2
ORDER BY t.created_at DESC, t.id;

-- name: GetWorkspaceOrganization :one
SELECT organization_id FROM workspaces WHERE id = $1;

-- name: GetTask :one
SELECT t.id, t.workspace_id, t.workflow_id, t.title, t.description, t.source_provider, COALESCE(t.source_id, '') AS source_id, COALESCE(t.source_url, '') AS source_url, t.status, COALESCE(t.assignee_agent_id, '') AS assignee_agent_id, t.labels, t.created_at, t.updated_at, t.stage_version, t.stage_work_state, COALESCE(t.status = workflow_final_stage(t.workspace_id,t.workflow_id),false)::boolean AS closed, COALESCE((SELECT s.name FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_name, COALESCE((SELECT s.description FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_description, COALESCE((SELECT s.custom_prompt FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_custom_prompt, EXISTS(SELECT 1 FROM workflow_events e WHERE e.workspace_id=t.workspace_id AND e.task_id=t.id AND e.stage_version=t.stage_version AND e.processed_at IS NULL) AS automation_pending
FROM tasks t JOIN workspaces w ON w.id = t.workspace_id
WHERE w.organization_id = $1 AND t.workspace_id = $2 AND t.id = $3;

-- name: InsertTask :one
INSERT INTO tasks AS t (id, workspace_id, workflow_id, title, description, status, assignee_agent_id, labels)
SELECT $1, w.id, $3, $4, $5, $6, NULLIF($7, ''), $8 FROM workspaces w WHERE w.id = $2
RETURNING id, workspace_id, workflow_id, title, description, source_provider, COALESCE(source_id, '') AS source_id, COALESCE(source_url, '') AS source_url, status, COALESCE(assignee_agent_id, '') AS assignee_agent_id, labels, created_at, updated_at, stage_version, stage_work_state, COALESCE(status = workflow_final_stage(workspace_id,workflow_id),false)::boolean AS closed, COALESCE((SELECT s.name FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_name, COALESCE((SELECT s.description FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_description, COALESCE((SELECT s.custom_prompt FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_custom_prompt, EXISTS(SELECT 1 FROM workflow_events e WHERE e.workspace_id=t.workspace_id AND e.task_id=t.id AND e.stage_version=t.stage_version AND e.processed_at IS NULL) AS automation_pending;

-- name: UpdateTask :one
UPDATE tasks t SET title = $4, description = $5, status = $6, assignee_agent_id = NULLIF($7, ''), labels = $8, updated_at = now()
FROM workspaces w
WHERE w.organization_id = $1 AND t.workspace_id = w.id AND t.workspace_id = $2 AND t.id = $3
RETURNING t.id, t.workspace_id, t.workflow_id, t.title, t.description, t.source_provider, COALESCE(t.source_id, '') AS source_id, COALESCE(t.source_url, '') AS source_url, t.status, COALESCE(t.assignee_agent_id, '') AS assignee_agent_id, t.labels, t.created_at, t.updated_at, t.stage_version, t.stage_work_state, COALESCE(t.status = workflow_final_stage(t.workspace_id,t.workflow_id),false)::boolean AS closed, COALESCE((SELECT s.name FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_name, COALESCE((SELECT s.description FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_description, COALESCE((SELECT s.custom_prompt FROM workflow_stages s WHERE s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status),'')::text AS stage_custom_prompt, EXISTS(SELECT 1 FROM workflow_events e WHERE e.workspace_id=t.workspace_id AND e.task_id=t.id AND e.stage_version=t.stage_version AND e.processed_at IS NULL) AS automation_pending;

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

-- name: ListWorkspaceTaskRepositories :many
SELECT tr.task_id,r.id,r.provider_name,COALESCE(NULLIF(r.display_name,''),r.url) AS name,r.url
FROM task_repositories tr
JOIN code_repositories r ON r.workspace_id=tr.workspace_id AND r.id=tr.repository_id
JOIN workspaces w ON w.id=tr.workspace_id
WHERE w.organization_id=$1 AND tr.workspace_id=$2 AND tr.task_id=ANY(sqlc.arg(task_ids)::text[])
ORDER BY tr.task_id,r.provider_name,name,r.id;
