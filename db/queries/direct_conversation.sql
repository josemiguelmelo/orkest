-- name: CreateDirectConversation :one
INSERT INTO direct_agent_conversations(id, organization_id, workspace_id, agent_id)
VALUES ($1, $2, $3, $4)
RETURNING id, agent_id, created_at;

-- name: GetDirectConversation :one
SELECT id, agent_id, created_at FROM direct_agent_conversations
WHERE organization_id = $1 AND workspace_id = $2 AND id = $3;

-- name: ListDirectConversations :many
SELECT id, agent_id, created_at FROM direct_agent_conversations
WHERE organization_id = $1 AND workspace_id = $2
ORDER BY created_at DESC;

-- name: LockDirectConversation :one
SELECT id FROM direct_agent_conversations
WHERE organization_id = $1 AND workspace_id = $2 AND id = $3 FOR UPDATE;

-- name: ActiveDirectConversationLaunch :one
SELECT run_id FROM agent_launches
WHERE workspace_id = $1 AND task_id = $2
AND status IN ('starting', 'process_started', 'ready', 'completed_pending_close', 'failed_pending_close')
LIMIT 1;

-- name: DeleteDirectConversationLaunches :exec
DELETE FROM agent_launches
WHERE organization_id = $1 AND workspace_id = $2 AND task_id = $3;

-- name: DeleteDirectConversationSessions :exec
DELETE FROM task_agent_sessions
WHERE organization_id = $1 AND workspace_id = $2 AND task_id = $3;

-- name: DeleteDirectConversation :execrows
DELETE FROM direct_agent_conversations
WHERE organization_id = $1 AND workspace_id = $2 AND id = $3;
