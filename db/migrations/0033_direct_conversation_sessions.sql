-- +goose Up
-- Direct conversation IDs share the task session and launch identity columns,
-- but do not reference workflow tasks.
ALTER TABLE task_agent_sessions
    DROP CONSTRAINT IF EXISTS task_agent_sessions_task_id_fkey;
ALTER TABLE task_agent_sessions
    DROP CONSTRAINT IF EXISTS task_agent_sessions_workspace_id_task_id_fkey;

-- +goose Down
DELETE FROM agent_launches
WHERE task_id IN (SELECT id FROM direct_agent_conversations);
DELETE FROM task_agent_sessions
WHERE task_id IN (SELECT id FROM direct_agent_conversations);

ALTER TABLE task_agent_sessions
    ADD CONSTRAINT task_agent_sessions_workspace_id_task_id_fkey
    FOREIGN KEY (workspace_id, task_id) REFERENCES tasks(workspace_id, id) ON DELETE CASCADE;
