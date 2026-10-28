-- +goose Up
ALTER TABLE agent_launches DROP CONSTRAINT IF EXISTS agent_launches_task_session_id_fkey;
ALTER TABLE agent_launches DROP CONSTRAINT IF EXISTS agent_launch_session_scope_fk;
ALTER TABLE agent_launches
    ADD CONSTRAINT agent_launch_session_scope_fk
    FOREIGN KEY (workspace_id, task_id, task_session_id)
    REFERENCES task_agent_sessions(workspace_id, task_id, id)
    ON DELETE SET NULL (task_session_id);

-- +goose Down
ALTER TABLE agent_launches DROP CONSTRAINT agent_launch_session_scope_fk;
ALTER TABLE agent_launches
    ADD CONSTRAINT agent_launch_session_scope_fk
    FOREIGN KEY (workspace_id, task_id, task_session_id)
    REFERENCES task_agent_sessions(workspace_id, task_id, id);
