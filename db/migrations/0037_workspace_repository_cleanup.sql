-- +goose Up
-- NO ACTION checks the reference at transaction commit. It still prevents
-- deleting a repository used by a task, but permits workspace deletion to
-- cascade through tasks, stage definitions and repositories in either order. RESTRICT can
-- reject that valid cascade on PostgreSQL 18 before task links are removed.
ALTER TABLE task_repositories DROP CONSTRAINT task_repositories_workspace_id_repository_id_fkey;
ALTER TABLE task_repositories ADD CONSTRAINT task_repositories_workspace_id_repository_id_fkey
FOREIGN KEY(workspace_id,repository_id) REFERENCES code_repositories(workspace_id,id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE tasks ALTER CONSTRAINT tasks_workflow_stage_fkey DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE workflow_stages ALTER CONSTRAINT workflow_stage_agent_fkey DEFERRABLE INITIALLY DEFERRED;

-- +goose Down
ALTER TABLE tasks ALTER CONSTRAINT tasks_workflow_stage_fkey NOT DEFERRABLE;
ALTER TABLE workflow_stages ALTER CONSTRAINT workflow_stage_agent_fkey NOT DEFERRABLE;
ALTER TABLE task_repositories DROP CONSTRAINT task_repositories_workspace_id_repository_id_fkey;
ALTER TABLE task_repositories ADD CONSTRAINT task_repositories_workspace_id_repository_id_fkey
FOREIGN KEY(workspace_id,repository_id) REFERENCES code_repositories(workspace_id,id) ON DELETE RESTRICT;
