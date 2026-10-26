-- name: LockTaskSessionScope :exec
SELECT id FROM tasks WHERE id = $1 AND workspace_id = $2 FOR UPDATE;

-- name: CurrentTaskAgentSession :one
SELECT id, configuration FROM task_agent_sessions
WHERE organization_id = $1 AND workspace_id = $2 AND task_id = $3 AND is_current;

-- name: RetireTaskAgentSession :exec
UPDATE task_agent_sessions SET is_current = false
WHERE organization_id = $1 AND workspace_id = $2 AND task_id = $3 AND is_current;

-- name: InsertTaskAgentSession :exec
INSERT INTO task_agent_sessions(id, organization_id, workspace_id, task_id, configuration)
VALUES ($1, $2, $3, $4, $5);

-- name: SetRuntimeSessionID :exec
UPDATE task_agent_sessions SET runtime_session_id = $2 WHERE id = $1;

-- name: InsertPromptOutbox :exec
INSERT INTO agent_prompt_outbox(run_id, command_id, prompt) VALUES ($1, $2, $3)
ON CONFLICT (run_id, command_id) DO NOTHING;

-- name: GetPromptOutbox :one
SELECT prompt, delivered FROM agent_prompt_outbox WHERE run_id = $1 AND command_id = $2;

-- name: PendingPromptOutbox :many
SELECT command_id, prompt FROM agent_prompt_outbox WHERE run_id = $1 AND NOT delivered ORDER BY created_at, command_id;

-- name: MarkPromptDelivered :exec
UPDATE agent_prompt_outbox SET delivered = true WHERE run_id = $1 AND command_id = $2;

-- name: ActiveTaskLaunch :one
SELECT run_id FROM agent_launches WHERE workspace_id = $1 AND task_id = $2
AND status IN ('starting', 'process_started', 'ready', 'completed_pending_close', 'failed_pending_close') LIMIT 1;

-- name: CompleteTaskForRun :exec
UPDATE tasks SET status = 'in_review', updated_at = now()
FROM agent_launches AS launch
WHERE launch.run_id = $1 AND tasks.id = launch.task_id
AND tasks.workspace_id = launch.workspace_id AND tasks.status = 'in_progress';

-- name: FindTaskCommand :one
SELECT prompt FROM agent_prompt_outbox AS command
JOIN agent_launches AS launch USING (run_id)
WHERE launch.organization_id = $1 AND launch.workspace_id = $2 AND launch.task_id = $3
AND command.command_id = $4
ORDER BY command.created_at LIMIT 1;
