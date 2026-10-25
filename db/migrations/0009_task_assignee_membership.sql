-- +goose Up
ALTER TABLE tasks
    DROP CONSTRAINT tasks_workspace_id_assignee_user_id_fkey,
    ADD CONSTRAINT tasks_workspace_id_assignee_user_id_fkey
        FOREIGN KEY (workspace_id, assignee_user_id)
        REFERENCES workspace_memberships(workspace_id, user_id)
        ON DELETE SET NULL (assignee_user_id);

-- +goose Down
ALTER TABLE tasks
    DROP CONSTRAINT tasks_workspace_id_assignee_user_id_fkey,
    ADD CONSTRAINT tasks_workspace_id_assignee_user_id_fkey
        FOREIGN KEY (workspace_id, assignee_user_id)
        REFERENCES workspace_memberships(workspace_id, user_id);
