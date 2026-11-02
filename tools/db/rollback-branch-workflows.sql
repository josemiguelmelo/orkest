-- One-time local reset for the superseded branch migrations 0035-0039.
-- Back up first and stop the platform. Run with psql -v ON_ERROR_STOP=1.
BEGIN;
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM goose_db_version WHERE version_id>39 AND is_applied) THEN
        RAISE EXCEPTION 'refusing reset: migrations newer than 39 have been applied';
    END IF;
    IF NOT EXISTS(SELECT 1 FROM goose_db_version WHERE version_id=35 AND is_applied) THEN
        RAISE EXCEPTION 'workflow migration 35 is not applied';
    END IF;
END $$;
-- Stop platform command delivery and reconcile active runs before rollback.
-- Deletes workflow configuration/history/commands; preserves tasks, integrations,
-- runtime sessions and working files. Custom stage keys become todo.
DROP TRIGGER IF EXISTS task_stage_history ON tasks;
DROP TRIGGER IF EXISTS task_stage_entry ON tasks;
DROP TRIGGER IF EXISTS workspace_workflow_init ON workspaces;
DROP FUNCTION IF EXISTS record_task_stage_history();
DROP FUNCTION IF EXISTS record_task_stage_entry();
DROP FUNCTION IF EXISTS initialize_workspace_workflow();
DROP FUNCTION IF EXISTS workflow_final_stage(text,text);
DROP FUNCTION IF EXISTS default_workspace_workflow(text);
DROP TABLE IF EXISTS workflow_commands;
DROP TABLE IF EXISTS task_stage_history;
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_source_workflow_integration_fkey;
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_workflow_stage_fkey;
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_workflow_fkey;
ALTER TABLE task_integrations DROP CONSTRAINT IF EXISTS task_integrations_identity_workflow_key;
ALTER TABLE task_integrations DROP CONSTRAINT IF EXISTS task_integrations_workflow_fkey;
ALTER TABLE task_integrations DROP COLUMN IF EXISTS workflow_id;
ALTER TABLE tasks DROP COLUMN IF EXISTS workflow_id,
    DROP COLUMN IF EXISTS workflow_snapshot, DROP COLUMN IF EXISTS workflow_run_id,
    DROP COLUMN IF EXISTS stage_work_state, DROP COLUMN IF EXISTS stage_version;
ALTER TABLE agent_launches DROP COLUMN IF EXISTS stage_version;
UPDATE tasks SET status='todo' WHERE status NOT IN ('todo','in_progress','in_review','done');
ALTER TABLE tasks ADD CONSTRAINT tasks_status_check CHECK(status IN ('todo','in_progress','in_review','done'));
DROP TABLE IF EXISTS workflow_stages;
DROP TABLE IF EXISTS workspace_workflows;
ALTER TABLE task_repositories DROP CONSTRAINT task_repositories_workspace_id_repository_id_fkey;
ALTER TABLE task_repositories ADD CONSTRAINT task_repositories_workspace_id_repository_id_fkey
    FOREIGN KEY(workspace_id,repository_id) REFERENCES code_repositories(workspace_id,id) ON DELETE RESTRICT;

-- Schema and Goose history return to v34 together. Never edit only the version table.
DELETE FROM goose_db_version WHERE version_id BETWEEN 35 AND 39;
COMMIT;
