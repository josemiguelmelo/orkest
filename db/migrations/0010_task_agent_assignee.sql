-- +goose Up
ALTER TABLE tasks
    DROP CONSTRAINT tasks_workspace_id_assignee_user_id_fkey,
    DROP COLUMN assignee_user_id,
    ADD COLUMN assignee_agent_id text,
    ADD CONSTRAINT tasks_workspace_id_assignee_agent_id_fkey
        FOREIGN KEY (workspace_id, assignee_agent_id)
        REFERENCES agents(workspace_id, id)
        ON DELETE SET NULL (assignee_agent_id);

-- +goose Down
ALTER TABLE tasks
    DROP CONSTRAINT tasks_workspace_id_assignee_agent_id_fkey,
    DROP COLUMN assignee_agent_id,
    ADD COLUMN assignee_user_id text,
    ADD CONSTRAINT tasks_workspace_id_assignee_user_id_fkey
        FOREIGN KEY (workspace_id, assignee_user_id)
        REFERENCES workspace_memberships(workspace_id, user_id)
        ON DELETE SET NULL (assignee_user_id);
